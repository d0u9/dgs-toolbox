package tree

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"

	"dgs-toolbox/internal/verifiedcopy"
)

// ReuseFile offers an existing object. Import writes nothing until its Item
// is explicitly named by ImportRequest.Reuse.
type ReuseFile struct {
	Item   string `json:"item"`
	Digest string `json:"digest"`
	Path   string `json:"path"`
}

func (r *ReuseFile) Error() string {
	return fmt.Sprintf("%s: %s; confirm reuse of %s", ErrDuplicate, r.Item, r.Path)
}
func (r *ReuseFile) Unwrap() error { return ErrDuplicate }

// relativePath classifies an existing source or output folder. Empty means
// outside the tree. Paths lexically inside are checked before following links.
func (st *Store) relativePath(full string) (string, error) {
	root, err := filepath.Abs(st.root)
	if err != nil {
		return "", err
	}
	full, err = filepath.Abs(full)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, full)
	if err != nil {
		return "", err
	}
	if !filepath.IsLocal(rel) {
		root, err = filepath.EvalSymlinks(root)
		if err != nil {
			return "", err
		}
		full, err = filepath.EvalSymlinks(full)
		if err != nil {
			return "", err
		}
		rel, err = filepath.Rel(root, full)
		if err != nil {
			return "", err
		}
		if !filepath.IsLocal(rel) {
			return "", nil
		}
	}
	if rel == "." {
		return rel, nil
	}
	rel, err = cleanRecorded(filepath.ToSlash(rel))
	if err != nil {
		return "", err
	}
	if _, err = st.recordedPath(rel); err != nil {
		return "", err
	}
	return rel, nil
}

func (st *Store) importInPlace(ctx context.Context, request ImportRequest, input imageInput, item Item, digest string, reused *Revision) (saved Item, err error) {
	if err := ctx.Err(); err != nil {
		return Item{}, err
	}
	var r *Revision
	for i := range item.Revisions {
		if item.Revisions[i].Ref() == item.Current() {
			r = &item.Revisions[i]
			break
		}
	}
	if r == nil {
		return Item{}, fmt.Errorf("no HEAD revision")
	}

	if reused != nil {
		r.Path = reused.Path
		r.Originals = slices.Clone(reused.Originals)
		r.SourceDigests = slices.Clone(reused.SourceDigests)
		input.originals = r.Originals
		input.annotate(&item, r.Ref(), request.KeepOriginal)
		return item, st.WriteItem(item)
	}
	rel, err := st.relativePath(request.Source)
	if err != nil {
		return Item{}, err
	}
	complex := input.generated != nil || len(item.Revisions) > 1
	copyFile := rel == "" || input.generated != nil
	complex = complex && copyFile
	folder := ""
	if request.Path != "" {
		if !copyFile || input.generated != nil {
			return Item{}, fmt.Errorf("a suffix names a copy of a file from outside the tree; this file is already inside it and stays where it is")
		}
		if rel, err = cleanRecorded(request.Path); err != nil {
			return Item{}, err
		}
		if _, err := os.Lstat(filepath.Join(st.root, filepath.FromSlash(rel))); !os.IsNotExist(err) {
			return Item{}, fmt.Errorf("%s already exists", rel)
		}
		complex = false
	}
	if copyFile && request.Path == "" {
		if !filepath.IsAbs(request.Folder) {
			return Item{}, fmt.Errorf("choose an output folder inside the tree")
		}
		folder, err = st.relativePath(request.Folder)
		if err != nil {
			return Item{}, err
		}
		if folder == "" {
			return Item{}, fmt.Errorf("output folder is outside the tree")
		}
		info, err := os.Stat(request.Folder)
		if err != nil {
			return Item{}, err
		}
		if !info.IsDir() {
			return Item{}, fmt.Errorf("output folder is not a directory")
		}
		name := input.name
		if complex {
			name = digest + filepath.Ext(input.name)
			if input.generated != nil {
				name = digest + ".pdf"
			}
		}

		rel, err = cleanRecorded(path.Join(folder, name))
		if err != nil {
			return Item{}, err
		}
	}
	root, err := os.OpenRoot(st.root)
	if err != nil {
		return Item{}, err
	}
	defer root.Close()
	if complex {
		folder, err = cleanRecorded(path.Join(folder, item.ID))
		if err != nil {
			return Item{}, err
		}
		created := false
		if info, statErr := root.Stat(filepath.FromSlash(folder)); os.IsNotExist(statErr) {
			if err = root.Mkdir(filepath.FromSlash(folder), 0755); err != nil {
				return Item{}, err
			}
			created = true
		} else if statErr != nil {
			return Item{}, statErr
		} else if !info.IsDir() {
			return Item{}, fmt.Errorf("Item directory is not a folder")
		}
		defer func() {
			if err != nil && created {
				err = errors.Join(err, root.Remove(filepath.FromSlash(folder)))
			}
		}()
		name := digest + filepath.Ext(input.name)
		if input.generated != nil {
			name = digest + ".pdf"
		}
		rel = path.Join(folder, name)
	}
	r.Path = rel
	if request.KeepOriginal {
		for i := range input.originals {
			p, err := st.relativePath(input.sources[i])
			if err != nil {
				return Item{}, err
			}
			if p == "" || complex {
				p = path.Join(folder, input.originals[i].Digest+input.originals[i].Extension)
			}
			input.originals[i].Path = p
		}
	}
	input.annotate(&item, r.Ref(), request.KeepOriginal)
	if err := st.ValidateItemPaths(&item); err != nil {
		return Item{}, err
	}
	expectedRoot, err := root.Stat(".")
	if err != nil {
		return Item{}, err
	}
	copied := ""
	undoOriginal := func() {}
	defer func() {
		if err != nil {
			undoOriginal()
			if copied != "" {
				_ = root.Remove(copied)
			}
		}
	}()
	if copyFile {
		destination, err := st.recordedPath(rel)
		if err != nil {
			return Item{}, err
		}
		actualRel, err := filepath.Rel(st.root, destination)
		if err != nil {
			return Item{}, err
		}
		source := input.pdf
		if input.generated != nil {
			part := filepath.Join(filepath.Dir(actualRel), "."+item.ID+".dgs-part")
			file, err := root.OpenFile(part, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
			if err != nil {
				return Item{}, err
			}
			defer root.Remove(part)
			_, err = file.Write(input.generated)
			if err == nil {
				err = file.Sync()
			}
			closeErr := file.Close()
			if err != nil {
				return Item{}, err
			}
			if closeErr != nil {
				return Item{}, closeErr
			}
			source = filepath.Join(st.root, part)
		}
		result, err := verifiedcopy.Copy(ctx, verifiedcopy.Request{Source: source, Destination: destination, ExpectedDigest: digest, Root: st.root, ExpectedRoot: expectedRoot})
		if err != nil {
			return Item{}, err
		}
		copied = actualRel
		r.Size = new(result.Size)
	} else {
		actual, size, err := st.recordedDigest(ctx, rel)
		if err != nil {
			return Item{}, err
		}
		r.Size = new(size)
		if actual != digest {
			return Item{}, fmt.Errorf("source changed while it was being imported")
		}
	}
	undoOriginal, err = input.archive(ctx, st, item, request.KeepOriginal, expectedRoot)
	if err != nil {
		return Item{}, err
	}
	if err := ctx.Err(); err != nil {
		return Item{}, err
	}
	return item, st.WriteItem(item)
}
