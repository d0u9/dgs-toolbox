package photo

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"dgs-toolbox/internal/apps/photo/postprocess"
)

// organizeFolder moves the photos inside a folder into capture-date folders.
// It reads every capture date first and asks before moving anything into a
// date folder that already exists, unless flags say yes or dry-run.
func organizeFolder(in io.Reader, out io.Writer, args []string, flags map[string]string) error {
	root, err := filepath.Abs(expandHome(args[0]))
	if err != nil {
		return err
	}
	layout := flags["format"]
	if layout == "" {
		layout = postprocess.DefaultLayout
	}
	if err := postprocess.ValidateLayout(layout); err != nil {
		return err
	}
	dest := root
	if flags["dest"] != "" {
		if dest, err = filepath.Abs(expandHome(flags["dest"])); err != nil {
			return err
		}
	}
	list := postprocess.FolderFiles
	if flags["recursive"] == "true" {
		list = postprocess.TreeFiles
	}
	files, err := list(root)
	if err != nil {
		return err
	}
	plan := postprocess.NewLayoutPlan(dest, files, layout)
	dates := plan.Dates()
	if len(dates) == 0 {
		fmt.Fprintf(out, "No JPG or RAW files with a capture date in %s\n", root)
		printOutcomes(out, root, plan.Files)
		return nil
	}
	fmt.Fprintf(out, "Capture dates: %s\n", strings.Join(dates, ", "))
	existing := plan.ExistingDates()
	if len(existing) > 0 {
		fmt.Fprintf(out, "These folders already exist in %s:\n", dest)
		for _, date := range existing {
			fmt.Fprintf(out, "  %s/\n", date)
		}
	}
	if flags["dry-run"] == "true" {
		printPlan(out, root, plan.Files)
		return nil
	}
	if len(existing) > 0 && flags["yes"] != "true" {
		fmt.Fprint(out, "Continue and move photos into them? [y/N] ")
		answer, _ := bufio.NewReader(in).ReadString('\n')
		if answer = strings.ToLower(strings.TrimSpace(answer)); answer != "y" && answer != "yes" {
			fmt.Fprintln(out, "Cancelled. No files were moved.")
			return nil
		}
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	printOutcomes(out, root, postprocess.Apply(plan).Files)
	return nil
}

// printPlan lists where each photo would go, for --dry-run.
func printPlan(out io.Writer, root string, outcomes []postprocess.Outcome) {
	move, skip, fail := 0, 0, 0
	for _, outcome := range outcomes {
		switch {
		case outcome.Status == postprocess.Failed:
			fail++
			fmt.Fprintf(out, "! %s · %s\n", relative(root, outcome.Original), outcome.Error)
		case outcome.Status == postprocess.Moved && !alreadyPlaced(outcome):
			move++
			fmt.Fprintf(out, "→ %s → %s/\n", relative(root, outcome.Original), outcome.Date)
		default:
			skip++
		}
	}
	fmt.Fprintf(out, "Dry run: would organize %d   Skip %d   Fail %d. No files were moved.\n", move, skip, fail)
}

func printOutcomes(out io.Writer, root string, outcomes []postprocess.Outcome) {
	moved, skipped, failed := 0, 0, 0
	for _, outcome := range outcomes {
		switch {
		case outcome.Status == postprocess.Failed:
			failed++
			fmt.Fprintf(out, "! %s · %s\n", relative(root, outcome.Original), outcome.Error)
		case outcome.Status == postprocess.Moved && !alreadyPlaced(outcome):
			moved++
			fmt.Fprintf(out, "✓ %s → %s/\n", relative(root, outcome.Original), outcome.Date)
		default:
			skipped++
		}
	}
	fmt.Fprintf(out, "Organized %d   Skipped %d   Failed %d\n", moved, skipped, failed)
}

// alreadyPlaced reports a photo that is already in its date folder, which a
// recursive run finds again inside folders it organized before.
func alreadyPlaced(outcome postprocess.Outcome) bool {
	return filepath.Clean(outcome.Original) == filepath.Clean(outcome.Final)
}

func relative(root, path string) string {
	if rel, err := filepath.Rel(root, path); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return path
}
