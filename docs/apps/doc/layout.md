# Storage Layouts

A doc tree stores its originals in a layout chosen when the tree is created
and kept for its life. Two are described here; a third, [In place](#in-place),
leaves imported files where they are:

- **By Item** — the layout every tree has today: one folder per Item, named by
  its ID, holding its sidecar and its PDFs. It remains the default.
- **By fields** — the PDFs are placed in readable directories built from their
  fields, such as event year and month for scans or category and language for
  e-books, and the sidecars are kept together in one folder.

The second layout exists so the owner can find files by hand in Finder in
trees where that matters: scans imported into doc keep their
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

The marker can name a layout. A marker without one is a by-Item tree, so every
existing tree reads as it does now. At step 1b, `Open` refuses any supplied
layout (including an empty one); `Init` writes no layout. The examples below
are the deferred step 2 design, not a layout supported by this build.

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

Editing a field used by the layout moves the PDFs it places:

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

`tree.Open(root)` is the only public entry point for an existing tree. It reads
and checks the marker before returning a Store. A marker without `layout`
selects `layout.ByItem()`; `layout: in-place` selects the in-place layout.
By-fields patterns, empty values and unknown names remain unsupported.
`Init` continues to write the original marker with no layout.

Domain operations and CLI actions pass the opened `*tree.Store` through every
layer. Store methods own sidecar reads, writes, imports, metadata, trash and
history; definition codecs also receive that Store. They do not reopen an
unchecked by-Item store from a root. Root strings remain for configuration,
initialization and display, and for plain path comparisons.

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
	Paths  map[string]string // PDF or original digest to recorded path, in place only
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
	Owned(rel string, items ...Item) bool
}

// A Move renames one file, both paths relative to the root.
type Move struct{ From, To string }

// Folders is a Layout that gives every Item a folder of its own, holding all
// its files and nothing else's. Deleting such an Item moves the folder whole.
type Folders interface {
	Folder(id string) string
}

// ByItem supplies the existing, plain-value layout.
func ByItem() Layout
```

```go
package tree

// Marker retains the legacy bytes when Layout is absent.
type Marker struct {
	Version int `yaml:"version"`
	CreatedAt string `yaml:"created_at"`
	Layout *string `yaml:"layout,omitempty"`
}

// Open checks the marker and selects ByItem or InPlace; other layouts are refused.
func Open(root string) (*Store, error)

// Init creates the marker and initial Template. Optional observers receive
// the same report paths the CLI previously computed, after successful init.
func Init(root string, now time.Time, report ...func(string)) error

// InitWithLayout selects in-place with the name "in-place".
func InitWithLayout(root string, now time.Time, name string, report ...func(string)) error

func (s *Store) Root() string
func (s *Store) Sidecar(id string) string
func (s *Store) PDF(item Item, digest string) string
func (s *Store) OriginalPath(item Item, pdf string, original Original) string
func (s *Store) Sidecars() ([]string, error)
func (s *Store) Folder(id string) (string, bool)
func (s *Store) Items() ([]Item, error)
func (s *Store) WriteItem(item Item) error
```

- **One implementation today; by fields deferred.** `byItem` is today's: `Sidecar` is
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
  with no root-taking path helper; check lists sidecars
  through `Sidecars` and reports what `Owned` refuses.

## Plan

1. **Done — refactor, no layout change.** Add `layout` with `byItem` only and
   route all 25 places through the Store, the web handlers included (see
   [Web API](api.md#plan)), and Templates, Outlines, rules, Snapshots and Cases
   through one store for definitions (see [Definitions](definitions.md#plan)). Existing tree layouts and file formats stay as they are. Storage regression
   tests pass; definition name checks and the web routes follow the completed
   Definitions and Web API steps.
1b. **Done — one entry point.** `tree.Open` is the only public way to a
   Store. Domain packages and the CLI take a Store; the marker may name a
   layout, and Open refuses one it does not know. Store methods own the
   implementations, and no outside caller uses an unchecked constructor or
   root-taking tree operation. Existing paths, data and web contracts stay
   unchanged.
2. **Deferred — add by fields.** `byFields`, the marker's `layout`, `{digest:N}`, moves,
   status and put-back, each with tests built on constructed trees.

## What does not change

- Import, export, Outlines, rules, Cases, Snapshots and history work on Items
  and digests, not paths, and behave the same in both layouts.
- Deleting moves an Item's sidecar and its files to the trash, as today.
- **No merging across layouts.** Two trees are merged only when they share a
  layout. Trees with different layouts are not merged.
- Existing trees stay by Item. Nothing converts them.

## In place

Doc 2.0: a large change, developed on its own branch.

Its aim is in [Doc 2.0](v2.md#aim).

V1 supports creating and reading this layout; V2 adds new-Item import:

```sh
dgs doc init --layout in-place ./documents
```

Omitting the flag creates a by-Item tree with its original marker bytes.
By-fields patterns, empty layouts and unknown names remain unsupported.
Browse, the Item page, PDF preview and export read the recorded paths.
Import records internal PDFs, copies external PDFs into a chosen folder and
converts images into PDFs there. Items without a PDF, such as a trip an
anchor stands for, are created as in any tree: only the sidecar is written.
Attachment changes, replacement imports, deletion and merge remain refused
before writes.

A third layout, for a tree made over files that are already organised: an
import leaves the file where it is, under its own name, and the Item records
where that is. Nothing is computed from fields, so editing a field never moves
a file.

```text
<root>/
  _dgs/                          everything doc keeps; the rest is the owner's
    tree.yaml                    the marker, layout: in-place
    changed                      the change token
    items/
      <item-id>.dgs-item.yaml    every sidecar, naming each file's digest and path
    templates/ outlines/ rules/ snapshots/ cases/ trash/
  Books/
    中文/
      a-book.pdf                 where it was before the import
  2026/
    03/
      scan-0012.pdf
      registration.pdf          generated PDF, named at import
      registration/             retained originals, original filenames
        1.jpg
        2.jpg
        3.jpg
```

- **Inside the root stays put.** A file inside the tree's root is recorded
  where it is. A file from outside is copied, with the verified copy every
  import uses, to a folder chosen at import; the source is left alone.
- **The path is stored.** The sidecar names each PDF by digest and by its path
  relative to the root. This is the one layout where a path is truth, not
  computed.
- **New PDFs go where the owner says.** A PDF the import makes, such as one
  from photos, is written to a folder chosen at import.
- **Moved by hand: reported, then confirmed.** A PDF missing from its recorded
  path is looked for by digest; where it is found, the new path is reported,
  and the sidecar is updated only after the owner confirms.
- **Changed in place: by lock.** A file at its recorded path whose digest no
  longer matches is an error for a [locked Item](v2.md#locked-items), and a
  new revision for any other.
- **Generated PDFs are named at import.** The PDF goes directly in the
  selected folder. Retained images go in a sibling directory named after the
  PDF without `.pdf`, keeping their original filenames. An empty PDF name uses
  the first image's stem plus `.pdf`. Existing PDFs or original-image folders,
  and duplicate original filenames, are refused without overwriting.
  Ordinary file revisions keep their names in the selected destination;
  a suffix places a copy beside the current file. Recorded files stay put.
- **One folder for doc's own files.** The marker, the change token, the
  sidecars, the definition folders and the trash are all under `_dgs/`, so
  the root holds only the owner's files and folders. The folder is visible,
  not `.dgs`, so a copy made by hand keeps it; the leading `_` sorts it first.
  An in-place tree is recognised by `_dgs/tree.yaml`; a by-Item tree keeps
  `dgs-doctree.yaml` and its folders at the root, unchanged.
- **`_dgs/items/` holds only descriptions.** Every sidecar, plain or complex,
  is `_dgs/items/<item-id>.dgs-item.yaml`, and nothing else is there, so the
  owner can keep `_dgs/` (without `trash/`) under Git while the large files
  stay out of it. Finding an Item by ID is reading one file.
- **A plain file edited in place stays a file.** If the owner records the
  changed file as a new revision, the earlier revision keeps its fields and
  digest and is marked as having lost its PDF.
- **Not only PDFs.** An object may be any file. Images may be made into a
  PDF, as photo imports are. Other files, such as video and audio, are stored
  as they are, with only basic handling: a thumbnail where the platform can
  make one natively (QuickLook on macOS; none elsewhere), kept in the local
  cache outside the tree, and a button that shows the file in Finder. Page
  text, search within the text and page views remain PDF-only.
- **Shared files allowed.** Several Items may name the same file.
- **Deletion counts references.** Deleting an Item moves its sidecar to the
  trash, and each of its files too once no other Item names it.

### Import in V2

The Import page opens the owner's existing folders. A PDF already under the
root stays where it is: only its new sidecar is written. An external PDF
keeps its original basename in the selected folder inside the root; an
occupied name is refused. The source stays untouched. The same path
validation as V1 applies to sources and destinations.

If a sidecar already names the input's digest, Import offers to reuse that
object. Confirmation creates a new Item naming the existing path; it copies
no files. The object is checked again when confirmation arrives. A missing
or changed object is an error, and an unconfirmed offer writes nothing.

Images use the existing converter, in memory until the digest lookup is done.
Generated PDFs use `<chosen folder>/<PDF filename>`; retained originals use
`<chosen folder>/<PDF stem>/<original filename>`, including internal inputs.
The import form and API accept a PDF filename, defaulting to the first image's
stem plus `.pdf`. Name conflicts are refused. Recorded files always keep their
location and digest; existing imports are not moved.
Reuse keeps the existing object's paths and archived originals.

Copies use source digest verification and independent destination readback
before publication. Temporary files stay beside the destination. A failed
import removes only its own published files. `items/` continues to contain only sidecars.

### Replacement and deletion in V4

Replaces accepts a new Item of the same type and owner/country; it marks its
predecessor superseded without moving either plain file. Deletion reads live
sidecars to count references. It moves the sidecar and only objects with no
remaining reference into a new trash folder. Revision deletion keeps other
snapshots and restores HEAD when necessary. Changed content is refused before
moves; failed writes reverse completed moves. No file bytes are deleted.

### Other files in V5

In-place imports accept regular files of any type, preserving bytes and the
extension. PDF pages, text and search reject or skip non-PDF files. Browse uses
macOS QuickLook thumbnails from the local cache and Show in Finder for any
file. Other platforms and builds without cgo report thumbnail unavailability.
Attachment changes and merge remain deferred.

### Recorded paths in V1

A revision names its PDF with `digest` and `path`; each retained original
image also has a `path`. By-Item sidecars omit paths and keep their existing
meaning. For example, a manually written `items/alice.dgs-item.yaml`:

```yaml
id: alice
type: id_card
kind: document
fields: { owner: alice }
revisions:
  - digest: '<SHA-256 of Books/card.pdf>'
    path: Books/card.pdf
    added: '2026-10-07T00:00:00Z'
```

- Within one Item, every snapshot naming the same digest must name the same
  path. Reads and writes reject conflicting paths. A field correction keeps
  the PDF path and original-image paths on the new snapshot.
- Paths use `/` and NFC Unicode normalization. Resolution accepts the actual
  NFC or NFD filename on disk without renaming it; a normalized-name collision
  is an error. Paths must be relative, nonempty and stay inside the root.
- Paths cannot enter `_dgs/`. Resolved symlink targets
  must also stay inside the root and outside these reserved locations.
- A file missing from its recorded location stays missing; V1 does not search
  or update its path. Preview and export report "file is not at its recorded
  location" with the Item and recorded path. Export checks selected sources
  before changing the target and uses verified copy before publication.
- `check` checks referenced files. Unreferenced files are normal in an
  in-place tree and are not reported as loose files or errors.

## Open

- Where a PDF goes when a key its `path` needs is empty and not in brackets:
  refused at import, or a fallback such as the import date for a scan with no
  event date.

### Independent Outline format and file sizes

At init, `--layout=in-place` selects recorded paths; omitting it selects by
Item. `--outlines=paths|folders` selects Doc 1 rules or folder skeletons independently,
with paths as the default. The page's Initialize form offers both settings. The
marker omits default choices, preserving the original marker bytes.

Revisions and retained originals may record `size` in bytes beside `digest`.
Imports write it; field corrections preserve it; recording changed content
writes its new size. Moved-file lookup hashes only matching-size candidates.
Old sidecars without a size retain the complete-search fallback.
