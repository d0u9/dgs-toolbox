package photo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"dgs-toolbox/internal/apps/photo/importer"
	"dgs-toolbox/internal/apps/photo/postprocess"
	"dgs-toolbox/internal/config"
	"dgs-toolbox/internal/verifiedcopy"
)

func importDirectories(_ io.Reader, out io.Writer, args []string, flags map[string]string, global config.Config) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// The first signal cancels the import cleanly. Restoring the default
	// handling then lets a second Ctrl+C end a process stuck in a read that
	// cannot be interrupted, such as a hung network mount.
	go func() {
		<-ctx.Done()
		stop()
	}()
	return importDirectoriesContext(ctx, out, args, flags, global)
}

func importDirectoriesContext(ctx context.Context, out io.Writer, args []string, flags map[string]string, global config.Config) error {
	if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
		return errors.New("one source directory is required")
	}
	source, err := filepath.Abs(expandHome(args[0]))
	if err != nil {
		return err
	}
	var targets, extensions []string
	if err := json.Unmarshal([]byte(flags["dest"]), &targets); err != nil {
		return errors.New("repeat --dest for each destination directory")
	}
	for i, target := range targets {
		if strings.TrimSpace(target) == "" {
			return errors.New("destination must not be empty")
		}
		targets[i], err = filepath.Abs(expandHome(target))
		if err != nil {
			return err
		}
	}
	if err := importer.ValidateTargets(source, targets); err != nil {
		return err
	}
	workers, err := strconv.Atoi(flags["workers"])
	if err != nil || workers < 1 || workers > importer.MaxWorkers {
		return fmt.Errorf("workers must be 1–%d", importer.MaxWorkers)
	}
	conflict := importer.ConflictPolicy(flags["duplicates"])
	if conflict != importer.Skip && conflict != importer.KeepBoth {
		return errors.New("duplicates must be skip or keep-both; replace is unavailable")
	}
	if err := json.Unmarshal([]byte(flags["ext"]), &extensions); err != nil {
		return errors.New("repeat --ext for each extension")
	}
	selected := make(map[string]bool)
	var order []string
	for _, extension := range extensions {
		extension = strings.ToUpper(strings.TrimPrefix(strings.TrimSpace(extension), "."))
		if extension == "" || strings.ContainsAny(extension, "/\\") {
			return errors.New("ext must name a file extension")
		}
		if strings.ContainsAny(extension, ", ") {
			return fmt.Errorf("ext %q names several extensions; repeat --ext for each", extension)
		}
		if !selected[extension] {
			order = append(order, extension)
		}
		selected[extension] = true
	}
	for _, pair := range importer.SharedFilesystems(targets) {
		fmt.Fprintf(out, "Warning: %s and %s are on the same filesystem; their copies are not independent of each other\n", targets[pair[0]], targets[pair[1]])
	}
	stateName := global.PhotoImportStateFile()
	scan := scanSource(source)
	if len(scan.sourceErrors) > 0 {
		return fmt.Errorf("source scan incomplete: %s", scan.sourceErrors[0])
	}
	for _, excluded := range scan.sourceSkipped {
		fmt.Fprintf(out, "Not scanned: %s\n", excluded)
	}
	var files []scannedFile
	matched := make(map[string]bool)
	for _, file := range scan.source {
		extension := strings.ToUpper(strings.TrimPrefix(filepath.Ext(file.path), "."))
		if len(selected) == 0 || selected[extension] {
			files = append(files, file)
			matched[extension] = true
		}
	}
	// A misspelt extension must not read as a successful, empty import.
	for _, extension := range order {
		if !matched[extension] {
			return fmt.Errorf("no file in %s has extension %s", source, extension)
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(files) == 0 {
		fmt.Fprintln(out, "No regular files in Source. Nothing imported.")
		return nil
	}
	if flags["dry-run"] == "true" {
		fmt.Fprintln(out, "Dry run: inventory only; incremental SHA-256 checks and capture-date planning happen during execution.")
		for _, file := range files {
			fmt.Fprintf(out, "Source: %s (%d bytes)\n", file.path, file.size)
		}
		for _, target := range targets {
			var inventory []scannedFile
			var scanErrors, skipped []string
			scanTree("DST", target, stateName, &inventory, &scanErrors, &skipped)
			fmt.Fprintf(out, "Destination: %s (%d regular files)\n", target, len(inventory))
			for _, file := range inventory {
				fmt.Fprintf(out, "  %s (%d bytes)\n", file.path, file.size)
			}
			for _, entry := range skipped {
				fmt.Fprintf(out, "Not scanned: %s\n", entry)
			}
			if len(scanErrors) > 0 {
				return fmt.Errorf("destination scan incomplete: %s", scanErrors[0])
			}
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		fmt.Fprintf(out, "Selected: %d files; move=%t, incremental=%t, duplicates=%s, workers=%d, organize=%t\n", len(files), flags["move"] == "true", flags["incremental"] != "false", conflict, workers, flags["organize"] == "true")
		return nil
	}
	identities := make([]os.FileInfo, len(targets))
	for i, target := range targets {
		identities[i], err = os.Stat(target)
		if err != nil {
			return err
		}
	}
	plans := make([]importer.Plan, len(targets))
	for i, target := range targets {
		plans[i] = importer.Plan{Jobs: buildJobs(source, target, files), SourceRoot: source, DestinationRoot: target,
			StatePath: filepath.Join(target, stateName), Conflict: conflict, Workers: workers, Incremental: flags["incremental"] != "false"}
	}
	operation := importer.Copy
	if flags["move"] == "true" {
		operation = importer.Move
	}
	fmt.Fprintf(out, "Importing %d files into %d destinations (%s)\n", len(files), len(targets), operation)
	display := newImportProgress(out, len(targets), len(files), workers)
	display.destinations = targets
	display.summaries = make([]string, len(targets))
	defer display.clear()
	out = display
	unfinished := 0
	current := -1
	result, err := importer.RunMultiple(ctx, plans, operation, importer.MultipleHooks{
		// Terminals update worker rows; redirected output keeps per-file logs.
		Event: func(target int, event importer.Event) {
			defer display.update(target, event)
			if target != current {
				current = target
				if !display.enabled {
					fmt.Fprintf(out, "Destination %d/%d: %s\n", target+1, len(targets), targets[target])
				}
			}
			if display.enabled || !finalPhase(event.Phase) {
				return
			}
			fmt.Fprintf(out, "  %s  %s", event.Phase, relative(targets[target], event.Destination))
			if event.Err != nil {
				fmt.Fprintf(out, "  %s", event.Err)
			}
			fmt.Fprintln(out)
		},
		Target: func(target int, result importer.Result) {
			if display.enabled {
				importedCount, attention := 0, 0
				for _, file := range result.Files {
					if imported(file.Phase) {
						importedCount++
					} else {
						attention++
					}
				}
				display.summaries[target] = fmt.Sprintf("Destination %d/%d: %d imported, %d need attention", target+1, len(targets), importedCount, attention)
				display.lastDraw = time.Time{}
				display.update(target, importer.Event{Index: -1})
			}
			for _, file := range result.Files {
				if display.enabled && (!imported(file.Phase) || file.Error != "") {
					fmt.Fprintf(out, "  %s  %s  %s\n", file.Phase, relative(targets[target], file.Destination), file.Error)
				}
				if file.Warning != "" {
					fmt.Fprintf(out, "  ! %s  %s\n", relative(targets[target], file.Destination), file.Warning)
				}
			}
			if result.StateError != nil {
				fmt.Fprintf(out, "  State error: %s\n", result.StateError)
				unfinished++
			}
		},
		File: func(file importer.FileResult) {
			if file.Phase != importer.PhaseComplete {
				fmt.Fprintf(out, "%s  %s  %s\n", file.Phase, relative(source, file.Source), file.Error)
			}
		},
	})
	if err != nil {
		return err
	}
	if display.enabled {
		for _, summary := range display.summaries {
			if summary != "" {
				fmt.Fprintln(out, summary)
			}
		}
	}
	for i := result.Started; i < len(targets); i++ {
		fmt.Fprintf(out, "Destination %d/%d: %s not started (cancelled)\n", i+1, len(targets), targets[i])
	}
	for _, file := range result.Files {
		if file.Phase != importer.PhaseComplete {
			unfinished++
		}
	}
	if flags["organize"] == "true" && ctx.Err() == nil {
		for i, target := range result.Targets {
			unfinished += organizeImportTarget(ctx, out, source, targets[i], plans[i].StatePath, target)
		}
	}
	present, kept := countMultipleImported(result.Files)
	fmt.Fprintf(out, "Imported into every destination: %d/%d files", present, len(files))
	if kept > 0 {
		fmt.Fprintf(out, " (%d sources kept)", kept)
	}
	fmt.Fprintln(out)
	if err := ctx.Err(); err != nil {
		return err
	}
	if unfinished > 0 {
		return errors.New("import incomplete; successful copies and destination state files were retained")
	}
	if flags["delete-state"] == "true" {
		// Inspect every target before removing any resume bookkeeping.
		for i, plan := range plans {
			if err := verifiedcopy.CheckRoot(targets[i], identities[i]); err != nil {
				return err
			}
			state, err := importer.LoadState(plan.StatePath)
			if err != nil {
				return err
			}
			if importer.EarlierSourceKept(state) > 0 {
				fmt.Fprintln(out, "State retained: an earlier Move still has sources to delete.")
				return nil
			}
		}
		for i, plan := range plans {
			if err := verifiedcopy.CheckRoot(targets[i], identities[i]); err != nil {
				return err
			}
			if err := os.Remove(plan.StatePath); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("delete state: %w", err)
			}
			fmt.Fprintf(out, "State deleted: %s\n", plan.StatePath)
		}
	}
	return nil
}

// finalPhase reports an event that ends a file's transfer in one target.
func finalPhase(phase importer.Phase) bool {
	switch phase {
	case importer.PhaseComplete, importer.PhaseAlreadyPresent, importer.PhaseSourceKept, importer.PhaseSkipped, importer.PhaseFailed, importer.PhaseUndetermined:
		return true
	}
	return false
}

// countMultipleImported counts sources with a verified copy in every
// destination, and among them the Moves whose source could not be deleted:
// those are imported too, only their Move is unfinished.
func countMultipleImported(files []importer.FileResult) (present, kept int) {
	for _, file := range files {
		switch file.Phase {
		case importer.PhaseComplete:
			present++
		case importer.PhaseSourceKept:
			present++
			kept++
		}
	}
	return present, kept
}

// organizeImportTarget organizes one target and returns how many files need
// attention. A file whose capture date cannot be read stays where it is and
// is reported, but is not a failure: nothing is wrong with the copy.
func organizeImportTarget(ctx context.Context, out io.Writer, source, root, statePath string, target importer.Result) int {
	var files []postprocess.File
	var existing []postprocess.ExistingFile
	for _, result := range target.Files {
		if !imported(result.Phase) {
			continue
		}
		file := postprocess.File{Source: result.Source, Path: result.Destination}
		if result.Existing || result.Phase == importer.PhaseAlreadyPresent {
			existing = append(existing, postprocess.ExistingFile{File: file, Digest: result.DestinationHash, Size: result.Size})
			if !unorganized(source, root, result) {
				continue
			}
		}
		files = append(files, file)
	}
	plan := postprocess.NewPlanWithExisting(ctx, root, files, existing)
	organized := postprocess.ApplyContext(ctx, plan)
	moved := make(map[string]string)
	failed := 0
	for _, file := range organized.Files {
		if file.Status == postprocess.Moved {
			moved[file.Original] = file.Final
		} else if file.Status == postprocess.Failed && !file.NoDate {
			failed++
		}
	}
	fmt.Fprintf(out, "Organize destination: %s\n", root)
	detailed := true
	if display, ok := out.(*importProgress); ok && display.enabled {
		detailed = false
	}
	printOutcomesDetail(out, root, organized.Files, detailed)
	if err := importer.UpdateDestinations(statePath, moved); err != nil {
		fmt.Fprintf(out, "State error: %s\n", err)
		failed++
	}
	return failed
}
