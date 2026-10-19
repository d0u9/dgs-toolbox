package photo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"dgs-toolbox/internal/apps/photo/importer"
	"dgs-toolbox/internal/apps/photo/postprocess"
	"dgs-toolbox/internal/config"
	"dgs-toolbox/internal/tui"
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
	if flags["profile"] == listProfiles {
		// "--profile card" reads card as an argument; alone it lists them.
		if len(args) != 1 {
			return printProfiles(out, global)
		}
		flags = maps.Clone(flags)
		flags["profile"], args = args[0], nil
	}
	// A profile names the extensions a camera may write; a card without one
	// of them is not a misspelling, unlike a typed --ext.
	var givenNames []string
	_ = json.Unmarshal([]byte(flags[tui.GivenFlags]), &givenNames)
	extFromProfile := flags["profile"] != "" && !slices.Contains(givenNames, "ext")
	args, flags, err := withImportProfile(args, flags, global)
	if err != nil {
		return err
	}
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
		if !matched[extension] && extFromProfile {
			fmt.Fprintf(out, "No %s files in %s\n", extension, source)
		} else if !matched[extension] {
			return fmt.Errorf("no file in %s has extension %s", source, extension)
		}
	}
	if within, err := parseDateRange(flags["range"]); err != nil {
		return err
	} else if within.set() {
		before := len(files)
		var undated []string
		files, undated = withinRange(files, captureDays(source, files), within)
		for _, path := range undated {
			fmt.Fprintf(out, "No capture date, not imported: %s\n", path)
		}
		fmt.Fprintf(out, "Range %s: %d of %d files\n", flags["range"], len(files), before)
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
	if flags["allow-check-rename"] == "true" {
		release, err := allowCheckRename(out, targets)
		if err != nil {
			return err
		}
		defer release()
	}
	if err := checkPublication(targets); err != nil {
		return err
	}
	fmt.Fprintf(out, "Importing %d files into %d destinations (%s)\n", len(files), len(targets), operation)
	display := newImportProgress(out, len(targets), len(files), workers)
	display.destinations = targets
	display.summaries = make([]string, len(targets))
	defer display.clear()
	out = display
	unfinished := 0
	// Destinations run at once, so redirected logs name each up front and tag
	// every file line with its destination number.
	if !display.enabled {
		for i, target := range targets {
			fmt.Fprintf(out, "Destination %d/%d: %s\n", i+1, len(targets), target)
		}
	}
	result, err := importer.RunMultiple(ctx, plans, operation, importer.MultipleHooks{
		// Terminals update worker rows; redirected output keeps per-file logs.
		Event: func(target int, event importer.Event) {
			defer display.update(target, event)
			if display.enabled || !finalPhase(event.Phase) {
				return
			}
			fmt.Fprintf(out, "  %d  %s  %s", target+1, event.Phase, relative(targets[target], event.Destination))
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
		// The last line must not read as success: say it failed, and how much.
		verdict := fmt.Sprintf("IMPORT INCOMPLETE: %d of %d files are not in every destination", len(files)-present, len(files))
		if present == len(files) {
			// Every copy is there; organizing, a Move or the state file failed.
			verdict = "IMPORT INCOMPLETE: every file was copied, but a later step failed"
		} else if present == 0 {
			verdict = fmt.Sprintf("IMPORT FAILED: none of %d files was imported", len(files))
		}
		return fmt.Errorf("%s; see the failed lines above. Copies that did finish are kept, so rerunning imports only the rest", verdict)
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
		if file.Final != file.Original {
			moved[file.Original] = file.Final
		}
		if file.Status == postprocess.Failed && !file.NoDate {
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

// allowCheckRename opts the destinations into check-then-rename publication,
// for filesystems such as SMB that have neither exclusive rename nor hard
// links, and holds each one's lock until release.
func allowCheckRename(out io.Writer, targets []string) (func(), error) {
	var releases []func()
	release := func() {
		for _, one := range releases {
			one()
		}
	}
	for _, target := range targets {
		one, err := verifiedcopy.AllowCheckRename(target)
		if err != nil {
			release()
			return nil, err
		}
		releases = append(releases, one)
	}
	fmt.Fprintln(out, "Warning: where a destination cannot refuse an existing name, a file another program writes to the same name at the same moment can be replaced.")
	return release, nil
}

// checkPublication refuses at once a destination that would refuse every
// file after it was copied, naming the opt-in that imports there.
func checkPublication(targets []string) error {
	for _, target := range targets {
		err := verifiedcopy.CheckPublication(target)
		if errors.Is(err, verifiedcopy.ErrUnsafePublication) {
			return fmt.Errorf("%w; nothing was copied. An SMB mount is like this: add --allow-check-rename (allow_check_rename in a profile, Check-rename in the workspace) to import there", err)
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

// listProfiles is --profile given without a name.
const listProfiles = "\x00list"

// printProfiles lists the configured profiles, one per line.
func printProfiles(out io.Writer, global config.Config) error {
	names := global.PhotoImportProfileNames()
	if len(names) == 0 {
		return errors.New("no import profile is configured; add import.profiles to photo/config.json")
	}
	fmt.Fprintln(out, "Import profiles (dgs photo import --profile <name>):")
	table := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	for _, name := range names {
		profile := global.Photo.Import.Profiles[name]
		fmt.Fprintf(table, "  %s\t%s → %s\n", name, profile.Source, strings.Join(profile.Dest, ", "))
	}
	return table.Flush()
}

// withImportProfile fills the flags of a --profile run from the configured
// profile. A flag given on the command line keeps its value; --dest given
// replaces the profile's list. A profile never moves.
func withImportProfile(args []string, flags map[string]string, global config.Config) ([]string, map[string]string, error) {
	name := flags["profile"]
	if name == "" {
		return args, flags, nil
	}
	profile, ok := global.Photo.Import.Profiles[name]
	if !ok {
		names := global.PhotoImportProfileNames()
		if len(names) == 0 {
			return nil, nil, fmt.Errorf("no import profile %q: none is configured in photo/config.json", name)
		}
		return nil, nil, fmt.Errorf("no import profile %q; configured: %s", name, strings.Join(names, ", "))
	}
	if len(args) > 0 {
		return nil, nil, errors.New("a source argument is not given with --profile; the profile names it")
	}
	var givenNames []string
	_ = json.Unmarshal([]byte(flags[tui.GivenFlags]), &givenNames)
	given := func(flag string) bool { return slices.Contains(givenNames, flag) }
	merged := maps.Clone(flags)
	set := func(flag, value string) {
		if !given(flag) {
			merged[flag] = value
		}
	}
	list := func(values []string) string {
		encoded, _ := json.Marshal(append([]string{}, values...))
		return string(encoded)
	}
	set("dest", list(profile.Dest))
	set("ext", list(profile.Ext))
	if profile.Duplicates != "" {
		set("duplicates", profile.Duplicates)
	}
	if profile.Workers > 0 {
		set("workers", strconv.Itoa(profile.Workers))
	}
	if profile.Incremental != nil {
		set("incremental", strconv.FormatBool(*profile.Incremental))
	}
	set("organize", strconv.FormatBool(profile.Organize))
	set("allow-check-rename", strconv.FormatBool(profile.AllowCheckRename))
	return []string{profile.Source}, merged, nil
}
