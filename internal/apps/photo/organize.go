package photo

import (
	"bufio"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"dgs-toolbox/internal/apps/photo/postprocess"
)

// organizeFolder moves the photos directly inside a folder into YYYYMMDD
// folders beside them. It reads every capture date first and asks before
// moving anything into a date folder that already exists.
func organizeFolder(in io.Reader, out io.Writer, args []string) error {
	root, err := filepath.Abs(expandHome(args[0]))
	if err != nil {
		return err
	}
	files, err := postprocess.FolderFiles(root)
	if err != nil {
		return err
	}
	plan := postprocess.NewPlan(root, files)
	dates := plan.Dates()
	if len(dates) == 0 {
		fmt.Fprintf(out, "No JPG or RAW files with a capture date in %s\n", root)
		printOutcomes(out, plan.Files)
		return nil
	}
	fmt.Fprintf(out, "Capture dates: %s\n", strings.Join(dates, ", "))
	if existing := plan.ExistingDates(); len(existing) > 0 {
		fmt.Fprintf(out, "These folders already exist in %s:\n", root)
		for _, date := range existing {
			fmt.Fprintf(out, "  %s/\n", date)
		}
		fmt.Fprint(out, "Continue and move photos into them? [y/N] ")
		answer, _ := bufio.NewReader(in).ReadString('\n')
		if answer = strings.ToLower(strings.TrimSpace(answer)); answer != "y" && answer != "yes" {
			fmt.Fprintln(out, "Cancelled. No files were moved.")
			return nil
		}
	}
	printOutcomes(out, postprocess.Apply(plan).Files)
	return nil
}

func printOutcomes(out io.Writer, outcomes []postprocess.Outcome) {
	moved, skipped, failed := 0, 0, 0
	for _, outcome := range outcomes {
		switch outcome.Status {
		case postprocess.Moved:
			moved++
			fmt.Fprintf(out, "✓ %s → %s/\n", filepath.Base(outcome.Original), outcome.Date)
		case postprocess.Failed:
			failed++
			fmt.Fprintf(out, "! %s · %s\n", filepath.Base(outcome.Original), outcome.Error)
		default:
			skipped++
		}
	}
	fmt.Fprintf(out, "Organized %d   Skipped %d   Failed %d\n", moved, skipped, failed)
}
