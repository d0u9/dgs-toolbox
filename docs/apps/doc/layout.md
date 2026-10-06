# Storage Layouts

A doc tree stores its originals in one of two layouts, chosen when the tree is
created and kept for its life:

- **By Item** — the layout every tree has today: one folder per Item, named by
  its ID, holding its sidecar and its PDFs. It remains the default.
- **By fields** — the PDFs are placed in readable directories built from their
  fields, such as event year and month for scans or category and language for
  e-books, and the sidecars are kept together in one folder.

The second layout exists so the owner can find files by hand in Finder in
trees where that matters: Box's scans, moved into doc, keep their
`<YYYY>/<MM>` directories; a library of e-books is browsed by category and
language. It is how the originals themselves are stored, not a copy. An export
is what is handed to someone else; a second, browsable copy of the same files
is not a goal.

## What is the truth

As in Git, the design separates what is the truth from what is computed from
it. The truth is backed up and never rebuilt; everything else may be thrown
away and rebuilt from it.

| Truth | Computed |
|---|---|
| each sidecar | where each PDF belongs |
| each PDF, named by its digest | which files changed since the last look |
| each original page image | where a file moved by hand now is |
| the marker, naming the layout | |

The model already resembles Git's:

| Git | doc |
|---|---|
| a blob, named by its content's hash, never changed | a PDF, named by its SHA-256, never changed |
| a commit, its hash covering content and parent | a snapshot, its ID covering fields and PDF digest |
| a ref and HEAD | an Item's `head` |
| the reflog, appended | an Item's `history`, appended |
| `git fsck` | `check.Tree` |
| the index, a stat cache rebuilt at will | the caches outside the tree, rebuilt at will |

What a by-fields layout adds is Git's checkout: a file's place follows from
the data, so it is computed rather than stored.

## Layout on disk

```text
<root>/
  dgs-doctree.yaml               the marker, naming the layout
  templates/ outlines/ rules/ snapshots/ cases/ trash/   as in every tree
  items/
    <item-id>.dgs-item.yaml      every sidecar, one file per Item
  2016/
    05/
      passport-a1b2c3d4e5f6.pdf
  2026/
    03/
      passport-e5f6a7b8c9d0.pdf
      scan-0012-77f0e1b9a2c3.pdf
      77f0e1b9a2c3…d4.jpg        an original page image, beside its PDF
```

- **Sidecars together, in a visible folder.** Every sidecar is
  `items/<item-id>.dgs-item.yaml`. The folder is visible, not `.dgs`, so a
  copy of the tree made by hand never leaves it behind. Without the sidecars
  the PDFs are files nobody can name; they are as much the truth as the PDFs.
- **PDFs flat.** A PDF lies directly in the directory the layout names, with
  no folder per Item. An Item with several PDFs — a passport and its renewal —
  may have them in different directories.
- **Page images beside their PDF.** The original images a photo import turned
  into a PDF lie in the same directory as that PDF, named by their own digest
  as they are today.
- **Everything else as in every tree.** Templates, Outlines, rules,
  Snapshots, Cases and the trash are where [Layout on disk](index.md#layout-on-disk)
  puts them.

## The marker

The marker gains the layout. A marker without one is a by-Item tree, so every
existing tree reads as it does now.

```yaml
version: 1
created_at: "2026-10-06T21:00:00+11:00"
layout:
  path: '{date:YYYY}/{date:MM}'
  file: '{name}-{digest:12}.{ext}'
```

```yaml
# an e-book library
layout:
  path: '{category}/{language}'
  file: '{title} - {author}-{digest:12}.{ext}'
```

- **Chosen once.** The layout is written by `init` and never changed.
  Changing it would move every file, so it is not offered; a tree that wants
  another layout is a new tree.
- **Rule syntax.** `path` and `file` are written as a rule's are, with the
  same keys, formats and bracketed optional parts: `{date:YYYY}`,
  `{country:alpha2}`, `[-{issuer}]`. Linked Items' keys such as
  `{about.address}` are not offered: a file would move whenever another Item
  changed.
- **`{digest:N}`** is new: the first N hex digits of the PDF's SHA-256, at
  least 8; `{digest}` is all 64. The file name must include it, so two PDFs
  whose fields agree — two alike Items, or a rescan correcting its own
  snapshot — never share a path. A layout without it is refused by `init`.

## Where a PDF belongs

A PDF's place is computed from the layout, the fields of the snapshot that
holds it, and its digest. A sidecar names its PDFs by digest only, as it does
today; no path is stored.

- **Finding an Item by ID** is reading `items/<item-id>.dgs-item.yaml`.
- **Finding a snapshot's PDF** is computing its path from the sidecar. No index
  is needed for either.
- **A PDF held by several snapshots** — a field edit that kept the PDF — is
  placed by the newest snapshot that holds it.
- **Text is made safe for a path** as an export makes it: separators and
  characters a filesystem refuses are replaced, and a name too long for the
  filesystem is shortened before the digest, which is always kept whole.

## Moves

Editing a field the layout uses moves the PDFs it places, as Box's event-date
correction does:

1. The sidecar is written first, so a failure afterwards leaves a correct
   description.
2. Each PDF, and its page images, is renamed into its new directory on the
   same volume: atomic, no copy, no readback, and its digest stays valid
   because its bytes are untouched.
3. A file that cannot move is put back, and the next edit tries again.
4. The move is recorded in the Item's history with both paths.
5. A directory left empty is removed.

Fields the layout does not use — tags, notes, most fields — never move
anything.

## Files moved by hand

The owner may move or rename files in Finder; they are found, not lost.

- **Status.** A PDF missing from where it belongs is reported, as
  `git status` reports a change. Only then is the tree walked to find it by
  digest. A PDF no sidecar names is reported as loose.
- **Put back.** The owner may put a file found elsewhere back where it
  belongs. Nothing is moved without being asked.
- **Check.** `check.Tree` verifies digests at their computed places and
  reports the same missing and loose files.

## Cache

The only cache is a record of each file's size and modification time, so a
start or a check rehashes only the files that changed. It lives outside the
tree with the other caches ([cache.md](cache.md)), and a missing, corrupt or
older one is discarded and rebuilt, never repaired. Rebuilding it yields the
same answers as using it.

## Interface

A layout is one interface behind which both layouts sit, so nothing above it
joins a path. Today 25 places outside it do — `tree.PDFPath(root, id, digest)`
and `filepath.Join(root, tree.ItemsDir, …)` in `tree`, `check`, `export`,
`merge` and the web handlers — and each would otherwise need to ask which
layout it is in.

```text
web, export, merge, check      know Items, IDs and digests; never join a path
        │  tree.Open(root) gives a Store
tree.Store                     reads and writes sidecars, imports, moves, trashes
        │  asks a Layout where
layout.Layout                  computes paths from plain values; touches no disk
```

```go
// Package layout places a doc tree's files. It only computes paths: reading,
// writing and renaming stay in package tree.
package layout

// An Item is what a layout needs of one: its ID and, for each PDF digest,
// the fields of the newest snapshot holding it. tree.Placing builds it.
type Item struct {
	ID     string
	Fields map[string]map[string]string
}

// A Layout places an Item's files under a tree's root. Every path is
// relative to the root.
type Layout interface {
	// Sidecar is where the Item's sidecar is.
	Sidecar(id string) string
	// PDF is where the Item's PDF of this digest belongs.
	PDF(item Item, digest string) string
	// Image is where an original page image of that PDF belongs.
	Image(item Item, pdf, image, ext string) string
	// Sidecars lists every sidecar in the tree.
	Sidecars(tree fs.FS) ([]string, error)
	// Moves are the renames that take before's files to where after places
	// them. By Item, there are never any.
	Moves(before, after Item) []Move
	// Owned tells whether a path is one the layout places, so check can
	// report the rest as loose.
	Owned(rel string) bool
}

// A Move renames one file, both paths relative to the root.
type Move struct{ From, To string }

// Folders is a Layout that gives every Item a folder of its own, holding all
// its files and nothing else's. Deleting such an Item moves the folder whole.
type Folders interface {
	Folder(id string) string
}

// Open reads the layout from a tree's marker: by Item when it names none.
func Open(root string, m Marker) (Layout, error)
```

- **Two implementations.** `byItem` is today's: `Sidecar` is
  `items/<id>/item.dgs-item.yaml` and `Moves` is always empty. `byFields`
  computes `PDF` from the marker's pattern in rule syntax and `Moves` by
  comparing the places of before and after.
- **`layout` is plain computation.** It takes values and returns paths, so it
  is tested with constructed Items and no filesystem, as AGENTS.md asks of a
  domain package. Listing sidecars reads an `fs.FS`, which a test builds in
  memory.
- **An Item folder is optional.** By Item is also `Folders`; a layout built
  from fields is not. Deleting an Item moves its folder whole only where there
  is one; a tree without Item folders refuses deletion until step 2 moves an
  Item's files one by one. `check` walks the tree for loose PDFs through
  `Owned`, and reports an Item folder without its sidecar only where the
  layout has folders.
- **`tree` is the only package that touches files.** It writes sidecars,
  imports, applies `Moves` one rename at a time with the put-back of
  [Moves](#moves), and moves to the trash.
- **Callers ask the Store.** An export's source becomes `store.PDF(item, digest)`
  instead of `tree.PDFPath(root, a.Item, a.Digest)`; check lists sidecars
  through `Sidecars` and reports what `Owned` refuses.

## Plan

1. **Done — refactor, no layout change.** Add `layout` with `byItem` only and
   route all 25 places through the Store, the web handlers included (see
   [Web API](api.md#plan)), and Templates, Outlines, rules, Snapshots and Cases
   through one store for definitions (see [Definitions](definitions.md#plan)). Existing tree layouts and file formats stay as they are. Storage regression
   tests pass; definition name checks and the web routes follow the completed
   Definitions and Web API steps.
2. **Add by fields.** `byFields`, the marker's `layout`, `{digest:N}`, moves,
   status and put-back, each with tests built on constructed trees.

## What does not change

- Import, export, Outlines, rules, Cases, Snapshots and history work on Items
  and digests, not paths, and behave the same in both layouts.
- Deleting moves an Item's sidecar and its files to the trash, as today.
- **No merging across layouts.** Two trees are merged only when they share a
  layout. Trees with different layouts are not merged.
- Existing trees stay by Item. Nothing converts them.

## Open

- Where a PDF goes when a key its `path` needs is empty and not in brackets:
  refused at import, or a fallback such as the import date, as Box places a
  scan with no event date.
