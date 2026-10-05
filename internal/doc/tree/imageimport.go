package tree

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"dgs-toolbox/internal/doc/docfile"
	"dgs-toolbox/internal/doc/imagepdf"
	"dgs-toolbox/internal/verifiedcopy"
)

// Original identifies one archived input image by content, beside its PDF.
type Original struct {
	Digest    string `yaml:"digest" json:"digest"`
	Extension string `yaml:"extension" json:"extension"`
}

// OriginalPath resolves a validated archive reference. Empty rejects malformed
// sidecar paths rather than allowing a reference outside the Item directory.
func OriginalPath(root, id string, o Original) string {
	if id == "" || id == "." || id == ".." || filepath.Base(id) != id {
		return ""
	}
	digest, err := hex.DecodeString(o.Digest)
	if err != nil || len(digest) != sha256.Size || strings.ToLower(o.Digest) != o.Digest || !docfile.IsImage("image"+o.Extension) || strings.ToLower(o.Extension) != o.Extension || filepath.Ext("image"+o.Extension) != o.Extension {
		return ""
	}
	return filepath.Join(Dir(root, id), o.Digest+o.Extension)
}

// sameOriginals reports whether two lists archive the same bytes in the same
// order; an extension alias such as .jpg and .jpeg names the same bytes.
func sameOriginals(a, b []Original) bool {
	return slices.EqualFunc(a, b, func(x, y Original) bool { return x.Digest == y.Digest })
}

// imageInput is what an import takes in: a PDF as it is, or images turned
// into one PDF, a page each.
type imageInput struct {
	pdf, name   string
	expectedPDF string
	// sources are the images, in page order, and originals their digests and
	// extensions; nil for a PDF.
	sources      []string
	originals    []Original
	inputDigests []string
	cleanup      func()
}

// prepareInput reads the images once and converts them into a part file in a
// hidden folder of the tree's items, on the tree's own volume; nothing about
// them is written elsewhere. A single PDF is taken as it is. Archival later
// copies each image from its source and refuses one whose bytes changed.
func prepareInput(ctx context.Context, root string, sources []string) (imageInput, error) {
	in := imageInput{cleanup: func() {}}
	if len(sources) == 0 {
		return in, errors.New("no file to import")
	}
	in.name = filepath.Base(sources[0])
	if len(sources) == 1 && !docfile.IsImage(sources[0]) {
		in.pdf = sources[0]
		return in, nil
	}
	inputs := make([]imagepdf.Input, len(sources))
	for i, source := range sources {
		if !docfile.IsImage(source) {
			return in, fmt.Errorf("%s: only images are put together as pages", filepath.Base(source))
		}
		data, err := imagepdf.Read(source)
		if err != nil {
			return in, err
		}
		sum := sha256.Sum256(data)
		digest := hex.EncodeToString(sum[:])
		inputs[i] = imagepdf.Input{Name: source, Data: data}
		in.inputDigests = append(in.inputDigests, digest)
		in.originals = append(in.originals, Original{Digest: digest, Extension: strings.ToLower(filepath.Ext(source))})
	}
	in.sources = sources
	output, err := imagepdf.ConvertAll(ctx, inputs)
	if err != nil {
		return in, err
	}
	sumPDF := sha256.Sum256(output)
	in.expectedPDF = hex.EncodeToString(sumPDF[:])
	if err := os.MkdirAll(filepath.Join(root, ItemsDir), 0755); err != nil {
		return in, err
	}
	dir, err := os.MkdirTemp(filepath.Join(root, ItemsDir), ".dgs-import-*")
	if err != nil {
		return in, err
	}
	in.cleanup = func() { os.RemoveAll(dir) }
	in.pdf = filepath.Join(dir, "converted.pdf.dgs-part")
	if err := os.WriteFile(in.pdf, output, 0600); err != nil {
		in.cleanup()
		return in, err
	}
	return in, nil
}

// archive copies each original image beside the PDF, each read back before
// it is published. One already there is checked, not copied again. The
// function returned removes what this call published.
func (in imageInput) archive(ctx context.Context, root, id string, keep bool) (func(), error) {
	var published []string
	undo := func() {
		for _, path := range published {
			os.Remove(path)
		}
	}
	if !keep {
		return func() {}, nil
	}
	for i, original := range in.originals {
		path := OriginalPath(root, id, original)
		if path == "" {
			undo()
			return func() {}, fmt.Errorf("invalid original image reference")
		}
		if _, err := os.Lstat(path); err == nil {
			digest, err := FileDigest(path)
			if err == nil && digest != original.Digest {
				err = fmt.Errorf("stored original digest mismatch")
			}
			if err != nil {
				undo()
				return func() {}, err
			}
			continue
		} else if !os.IsNotExist(err) {
			undo()
			return func() {}, err
		}
		result, err := verifiedcopy.Copy(ctx, verifiedcopy.Request{Source: in.sources[i], Destination: path})
		if err != nil {
			undo()
			return func() {}, err
		}
		published = append(published, path)
		if result.Digest != original.Digest {
			undo()
			return func() {}, fmt.Errorf("%s changed while it was being imported", filepath.Base(in.sources[i]))
		}
	}
	return undo, nil
}

// annotate records on revision ref where its PDF came from: the first
// input's name, every image's digest, so the images are known again, and,
// when kept, the originals in page order.
func (in imageInput) annotate(item *Item, ref string, keep bool) {
	if len(in.inputDigests) == 0 {
		return
	}
	for n := range item.Revisions {
		r := &item.Revisions[n]
		if r.Ref() != ref {
			continue
		}
		r.Source = in.name
		for _, d := range in.inputDigests {
			if !slices.Contains(r.SourceDigests, d) {
				r.SourceDigests = append(r.SourceDigests, d)
			}
		}
		if keep && len(in.originals) > 0 {
			r.Originals = in.originals
		}
	}
}

// needsSource reports whether revision r does not yet know every input image.
func (in imageInput) needsSource(r Revision) bool {
	for _, d := range in.inputDigests {
		if !slices.Contains(r.SourceDigests, d) {
			return true
		}
	}
	return false
}

// preserveProvenance keeps a reused snapshot's archived originals. Asking to
// keep different ones is a conflict, never a silent success; not asking
// leaves them as they are.
func (in *imageInput) preserveProvenance(item Item, ref string, keep bool) error {
	r, ok := item.Revision(ref)
	if !ok || len(r.Originals) == 0 || len(in.originals) == 0 {
		return nil
	}
	if sameOriginals(r.Originals, in.originals) {
		in.originals = r.Originals
		return nil
	}
	if keep {
		return fmt.Errorf("this snapshot already has different archived originals; keep the existing ones or import different document content")
	}
	in.originals = nil
	return nil
}

// heldBy is the Item whose revisions already know one of the input images,
// so the same images are not taken in twice even where their PDF came out
// different, on another system or converter.
func (in imageInput) heldBy(items []Item) (Item, bool) {
	for _, item := range items {
		for _, r := range item.Revisions {
			for _, d := range in.inputDigests {
				if slices.Contains(r.SourceDigests, d) {
					return item, true
				}
			}
		}
	}
	return Item{}, false
}

// digest verifies generated bytes independently after their temporary write.
func (in imageInput) digest() (string, error) {
	actual, err := FileDigest(in.pdf)
	if err != nil {
		return "", err
	}
	if in.expectedPDF != "" && actual != in.expectedPDF {
		return "", fmt.Errorf("converted PDF SHA-256 readback mismatch")
	}
	return actual, nil
}
