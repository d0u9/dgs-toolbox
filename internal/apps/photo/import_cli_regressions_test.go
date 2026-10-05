package photo

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dgs-toolbox/internal/apps/photo/importer"
	"dgs-toolbox/internal/config"
)

func runImportCLI(t *testing.T, source string, flags map[string]string) (string, error) {
	t.Helper()
	var output bytes.Buffer
	err := importDirectoriesContext(context.Background(), &output, []string{source}, flags, config.Default())
	return output.String(), err
}

func TestCLIImportRefusesMissingDestination(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "card")
	writeTree(t, source, "DCIM/IMG_0001.JPG")
	mountpoint := filepath.Join(root, "nas")
	if err := os.Mkdir(mountpoint, 0o755); err != nil {
		t.Fatal(err)
	}
	flags := importCLIFlags(nil)
	flags["dest"] = `["` + filepath.Join(mountpoint, "photos") + `"]`
	if _, err := runImportCLI(t, source, flags); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("missing destination accepted: %v", err)
	}
	if entries, _ := os.ReadDir(mountpoint); len(entries) != 0 {
		t.Fatal("import wrote under an empty mount point", entries)
	}
}

func TestCLIImportRefusesMisspeltExtensions(t *testing.T) {
	for _, ext := range []string{`["JPG,DNG"]`, `["JPG DNG"]`, `["JPEG"]`, `["JPG","CR3"]`} {
		t.Run(ext, func(t *testing.T) {
			root := t.TempDir()
			source, target := filepath.Join(root, "card"), filepath.Join(root, "library")
			writeTree(t, source, "DCIM/IMG_0001.JPG", "DCIM/IMG_0001.DNG")
			flags := importCLIFlags([]string{target})
			flags["ext"] = ext
			if output, err := runImportCLI(t, source, flags); err == nil {
				t.Fatalf("misspelt --ext reported success: %s", output)
			}
			if entries, _ := os.ReadDir(target); len(entries) != 0 {
				t.Fatal("refused command wrote destination", entries)
			}
		})
	}
}

func TestCLIImportWarnsAboutSharedFilesystemAndStreamsFiles(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "card")
	targets := []string{filepath.Join(root, "library"), filepath.Join(root, "backup")}
	writeTree(t, source, "DCIM/IMG_0001.JPG", "DCIM/IMG_0002.JPG")
	output, err := runImportCLI(t, source, importCLIFlags(targets))
	if err != nil {
		t.Fatal(err, output)
	}
	for _, want := range []string{"same filesystem", "Destination 1/2: " + targets[0], "Destination 2/2: " + targets[1], "1  complete  DCIM/IMG_0002.JPG", "2  complete  DCIM/IMG_0002.JPG", "Imported into every destination: 2/2 files"} {
		if !strings.Contains(output, want) {
			t.Fatalf("missing %q in:\n%s", want, output)
		}
	}
	// Destinations are named before any file, and files are printed as they
	// finish, not after the batch.
	if names, first, summary := strings.Index(output, "Destination 2/2"), strings.Index(output, "complete  DCIM/IMG_0001.JPG"), strings.Index(output, "Imported into every"); names < 0 || first < names || summary < first {
		t.Fatalf("destinations not named first or files not streamed:\n%s", output)
	}
}

// A RAW container the EXIF reader cannot parse, shot without a JPG, has no
// capture date to organize by. It stays where it is without failing.
func TestCLIOrganizeLeavesUndatedFilesWithoutFailing(t *testing.T) {
	root := t.TempDir()
	source, target := filepath.Join(root, "card"), filepath.Join(root, "library")
	writeTree(t, source, "DCIM/IMG_0001.CR3")
	flags := importCLIFlags([]string{target})
	flags["organize"] = "true"
	output, err := runImportCLI(t, source, flags)
	if err != nil || !strings.Contains(output, "No capture date 1") || !strings.Contains(output, "left in place") {
		t.Fatalf("err=%v\n%s", err, output)
	}
	if _, err := os.Stat(filepath.Join(target, "DCIM", "IMG_0001.CR3")); err != nil {
		t.Fatal(err)
	}
}

func TestCLIOrganizeReportsMissingPublishedFileAsFailure(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(t.TempDir(), "card")
	path := filepath.Join(root, "IMG_0001.JPG")
	target := importer.Result{Files: []importer.FileResult{{Source: filepath.Join(source, "IMG_0001.JPG"), Destination: path, Phase: importer.PhaseComplete}}}
	var output bytes.Buffer
	if failures := organizeImportTarget(context.Background(), &output, source, root, filepath.Join(root, ".dgs-state"), target); failures != 1 {
		t.Fatalf("missing published file was treated as undated: failures=%d\n%s", failures, output.String())
	}
	if !strings.Contains(output.String(), "Failed 1") || strings.Contains(output.String(), "left in place") {
		t.Fatal(output.String())
	}
}

func TestCLIScanReadsSourceOnly(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, "DCIM/IMG_0001.JPG")
	summary := scanSource(root)
	if scannedPaths(summary.source) != "DCIM/IMG_0001.JPG" || len(summary.destination) != 0 || len(summary.destinationErrors) != 0 {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestCLISummaryCountsSourceKeptAsImported(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	root := t.TempDir()
	source := filepath.Join(root, "card")
	targets := []string{filepath.Join(root, "library-a"), filepath.Join(root, "library-b")}
	writeTree(t, source, "DCIM/IMG_0001.JPG")
	folder := filepath.Join(source, "DCIM")
	// A read-only card: every copy is made, but the source cannot be deleted.
	if err := os.Chmod(folder, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(folder, 0o755) })
	flags := importCLIFlags(targets)
	flags["move"] = "true"
	var output bytes.Buffer
	if err := importDirectoriesContext(context.Background(), &output, []string{source}, flags, config.Default()); err == nil {
		t.Fatal("unfinished Move reported success")
	}
	if !strings.Contains(output.String(), "Imported into every destination: 1/1 files (1 sources kept)") {
		t.Fatal(output.String())
	}
}
