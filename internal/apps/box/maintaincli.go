package box

import (
	"context"
	"fmt"
	"io"
	"strings"

	boxweb "dgs-toolbox/internal/apps/box/web"
	"dgs-toolbox/internal/box/index"
	"dgs-toolbox/internal/box/maintain"
	"dgs-toolbox/internal/config"
)

// The three housekeeping commands are CLI actions rather than commands with a
// workspace. None of them writes anything into the Box — Index saves the
// discardable cache, which lives outside it, and Verify and Dedupe write
// nothing at all — and none of them asks anything once it starts: each reports
// what it found and repairs none of it. A keystroke in front of a read-only
// report gates nothing and keeps the slow one out of a scheduled run.
//
// They stay three actions rather than one because their costs differ by three
// orders of magnitude: Index and Dedupe read sidecars, Verify reads every byte
// in the Box. A name that hid that would mean either never dares be run.

// boxRoot resolves the Box to work in: the positional argument if given, the
// configured root otherwise.
func boxRoot(args []string, global config.Config) (boxweb.Settings, error) {
	settings := boxweb.SettingsFrom(global)
	if len(args) > 0 {
		settings.Root = args[0]
	}
	if settings.Root == "" {
		return settings, fmt.Errorf("no directory given and box.root is not configured")
	}
	return settings, nil
}

// indexAction rebuilds the discardable cache from the sidecars.
func indexAction(_ io.Reader, out io.Writer, args []string, _ map[string]string, global config.Config) error {
	settings, err := boxRoot(args, global)
	if err != nil {
		return err
	}
	result, err := maintain.Reindex(settings.Root, settings.MarkerName, settings.CacheDir)
	if err != nil {
		return err
	}
	built := "refreshed"
	if result.Rebuilt {
		built = "rebuilt from nothing"
	}
	fmt.Fprintf(out, "%d scans indexed, %d of them in the trash (%s).\n", result.Scans, result.Trashed, built)
	fmt.Fprintln(out, "Sidecars only: no scan's bytes were read. That is dgs box verify.")
	for _, line := range orphanLines(result.Orphans) {
		fmt.Fprintln(out, line)
	}
	return nil
}

// verifyAction recomputes every digest from the bytes.
//
// It reports each file as it reads it, because this is tens of gigabytes and a
// run that printed nothing for an hour would be indistinguishable from a hung
// one. Ctrl+C cancels through the context, and what was checked up to that
// point is still true.
func verifyAction(_ io.Reader, out io.Writer, args []string, flags map[string]string, global config.Config) error {
	settings, err := boxRoot(args, global)
	if err != nil {
		return err
	}
	// maintain.Verify signals completion with a last call carrying no path;
	// printing that one would report a file past the end of the run.
	progress := func(path string, done, total int) {
		if path == "" {
			return
		}
		fmt.Fprintf(out, "reading  %*d/%d  %s\n", digits(total), done+1, total, path)
	}
	if flags["quiet"] == "true" {
		progress = nil
	}
	result, err := maintain.Verify(context.Background(), settings.Root, settings.MarkerName, progress)
	if err != nil {
		return err
	}
	if progress != nil && result.Checked > 0 {
		fmt.Fprintln(out)
	}
	fmt.Fprintf(out, "%d files read, %s.\n", result.Checked, byteCount(result.Bytes))
	if len(result.Mismatches) == 0 {
		fmt.Fprintln(out, "Every file still matches the digest recorded for it.")
		return nil
	}
	fmt.Fprintln(out, "Nothing was repaired. Both digests are kept, because rewriting either destroys the evidence.")
	for _, mismatch := range result.Mismatches {
		fmt.Fprintln(out, "  "+mismatchLine(mismatch))
	}
	// A Box that no longer holds what it says it holds is the one finding a
	// scheduled run must not report as success.
	return fmt.Errorf("%d of %d files do not match", len(result.Mismatches), result.Checked)
}

// dedupeAction groups the scans that are the same piece of paper.
func dedupeAction(_ io.Reader, out io.Writer, args []string, _ map[string]string, global config.Config) error {
	settings, err := boxRoot(args, global)
	if err != nil {
		return err
	}
	result, err := maintain.Dedupe(settings.Root, settings.MarkerName)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "%d documents are in the Box more than once, %d copies in all.\n",
		len(result.Groups), result.Copies)
	fmt.Fprintln(out, "Nothing was discarded: which copy goes is a person's decision.")
	for _, group := range result.Groups {
		note := ""
		if group.Trashed {
			note = "  (one copy is already in the trash)"
		}
		fmt.Fprintln(out, "  "+strings.Join(group.Paths, "  =  ")+note)
	}
	return nil
}

func mismatchLine(mismatch maintain.Mismatch) string {
	if mismatch.Missing {
		return mismatch.Path + " — the file is not there"
	}
	return fmt.Sprintf("%s — recorded %s, bytes give %s", mismatch.Path, mismatch.Recorded, mismatch.Actual)
}

// orphanLines reports both directions and repairs neither. A sidecar with no
// scan may be the remains of a move; a scan with no sidecar has lost its only
// metadata. Guessing which is which is how a tool destroys what it looks after.
func orphanLines(orphans []index.Orphan) []string {
	if len(orphans) == 0 {
		return nil
	}
	lines := []string{fmt.Sprintf("%d things do not pair up. Nothing was repaired:", len(orphans))}
	for _, orphan := range orphans {
		lines = append(lines, fmt.Sprintf("  %s — %s", orphan.Path, orphan.Reason))
	}
	return lines
}

func digits(n int) int {
	if n < 10 {
		return 1
	}
	return len(fmt.Sprint(n))
}

func byteCount(bytes int64) string {
	const unit = 1000
	if bytes < unit {
		return fmt.Sprintf("%d bytes", bytes)
	}
	value, exponent := float64(bytes)/unit, 0
	for value >= unit && exponent < 4 {
		value /= unit
		exponent++
	}
	return fmt.Sprintf("%.1f %sB", value, []string{"k", "M", "G", "T", "P"}[exponent])
}
