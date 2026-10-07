# Doc Roadmap

Doc 2.0, in the order its parts build on each other. The design is in
[`v2.md`](v2.md); the parts of doc 1 (M1–M13) are done and recorded in Git
history.

Every part ends with something to try on the page.

| Part | What there is to try | State |
| --- | --- | --- |
| V1 | An in-place tree: `init` writes the layout; Browse lists Items whose sidecars are in `items/` and whose files are anywhere under the root | Done |
| V2 | Import in place: a file under the root is recorded where it is; one from outside is copied to a chosen folder; content already stored is reused | Done |
| V3 | Moved and changed files: reported, then the new path confirmed, or a change recorded as a revision or an error | Done |
| V4 | Complex Items in a folder of their own; replacement by a new Item; deletion counting references | Done |
| V5 | Files other than PDF: thumbnails, show in Finder | Done |
| V6 | Outlines as a folder skeleton: `take`, `each`, first match, unplaced Items | Done |
| V7 | Numbering within a folder; pinned Items and numbers | Done |
| V8 | Revisions and validity per folder: `revision`, `valid` | Done |

## Parts

Each part keeps every by-Item tree working unchanged: existing tests pass,
and its own tests build trees in memory or in a temporary folder with
invented data. Rules from [AGENTS.md](../../../AGENTS.md) hold throughout:
verified copy and readback before publication, nothing deleted, caches
outside the tree and rebuilt rather than repaired.

### V1 — an in-place tree (done)

- The marker's `layout` accepts `in-place` beside the by-fields patterns,
  which stay refused. `tree.Open` selects the layout; `Init` takes it as an
  option, and `dgs doc init` gains a flag for it.
- `layout.InPlace()`: `Sidecar` is `items/<id>.dgs-item.yaml`; `Sidecars`
  lists `items/`; `Owned` is every file a sidecar names. It is not
  `Folders`.
- A revision records its file's path relative to the root (`path`) in an
  in-place tree; `PDF` and `Image` answer the recorded path, not a computed
  one. By-Item sidecars carry no `path` and read as today.
- Browse, the Item page and export read such a tree. No import yet: tests
  write sidecars directly. Import, attachment changes, Item/revision deletion
  and merge are refused before writes.
- Reads and writes enforce one path per digest within an Item; field
  corrections preserve paths. Paths use `/` and NFC, exclude reserved
  locations, and resolve symlinks only within the root. Missing files are
  reported at preview/export; unreferenced files are normal, not loose files.
- Done when an in-place tree made by hand opens, lists, previews and exports
  like a by-Item one.

### V2 — import in place (done)

- A file under the root is recorded where it is: hashed, nothing written but
  the sidecar.
- A file from elsewhere is copied with `internal/verifiedcopy` to a folder the
  owner chooses on the page, under its own name; a name already taken is
  refused, never overwritten.
- Same content stored once: before anything is written, the digest is looked
  up; when an object with it exists, the page offers to name that object.
- A photo import writes its PDF to the chosen folder. Conversion stays in
  memory before duplicate confirmation; retained internal originals stay
  put, and external originals are verified copies beside the PDF.
- Attachment changes, attachment-free Items, replacement imports, deletion
  and merge remain deferred.
- Done when each of the three imports works on the page and the sidecar
  names the right path and digest.

### V3 — moved and changed files (done)

- A check compares each recorded path with the disk. A missing file is
  looked for by digest across the tree; found, the new path is reported.
- The page lists these and records a new path only when the owner confirms.
- A file whose digest changed at its path is handled by lock: an error for a
  locked Item, a new revision for any other, its earlier revision marked as
  having lost its file.
- `locked` is a Template default and an Item setting; Browse changes it
  through the existing metadata endpoint.
- Browse's explicit Check files action records unlocked changes; CLI check
  remains read-only. Each confirmation rechecks the recorded path and candidate
  digest and updates all references within that Item. Old snapshots use `lost`
  to prevent preview/export from reading newer content under the old digest.
- Searches also find moved retained originals. A changed original is an error;
  it cannot silently become a different PDF revision.
- Done when moving, renaming and editing files in Finder each show up and
  are resolved on the page.

### V4 — complex Items, replacement, deletion (done)

- An Item with photo originals or a second revision made through doc keeps
  new files written by doc in a folder of its own, at a place the owner chooses, files
  named by digest; its sidecar stays in `items/`.
- Recorded files never move when an Item becomes complex. A plain file edited
  in place stays a file.
- Replacing an Item with a new one (Replaces at import) works as in doc 1.
- Deleting moves the sidecar to the trash, and each file once no other Item
  names it.
- Done when a renewed card supersedes the old one with neither file moved,
  and deleting one of two Items sharing a file keeps the file.

### V5 — files other than PDF (done)

- A revision's file may be of any type; the PDF-only paths (pages, text,
  search, preview) check the type and say so for others.
- Thumbnails through QuickLook on macOS behind a build tag, none elsewhere,
  in the local cache.
- Show in Finder for any file.
- Done when a video and an audio file import, show a thumbnail on macOS and
  open in Finder.

### V6 — Outlines as an assembly of folder skeletons (done)

- Rules in the new format: a list of folders with `name`, `take`, `if`,
  `each`, `file`, `order`, `defaults`.
- Outlines with `defaults` and `mount` of rules and Snapshots at folders.
- First match through the mounts in order; unplaced Items listed on the page.
- The export plan and Why here work on the new format. Doc 1 formats are not
  read by it; converting is open.
- Done when an Outline built on the page places invented Items as the
  examples in [`v2.md`](v2.md#format) show.

### V7 — numbering and pins (done)

- Numbers within one folder, by one key or the owner's order, from 01.
- Revisions of one Item in one folder share its number and differ by date.
- Pins in the Outline by folder path: Items put there, numbers fixed there,
  both edited on the page.
- Done when dragging an Item and fixing a number on the page survive a
  reload and show in the plan.

### V8 — revisions and validity (done)

- `revision: head | all` and `valid: today | expired` per folder, inherited
  from defaults.
- Validity from `date.end` and supersession, on the day of the export.
- Done when a "current" and an "expired" Outline mounting the same rule
  split invented cards correctly.

## Deferred

- **Convergence** — types that inherit, conditions as one `if` expression, a
  rule's paths nesting: built, and in [`index.md`](index.md). What is left is
  the one-off migration of the real tree. [`convergence.md`](convergence.md)
  keeps the reasoning.

- **Accounts on a server** — not decided. The same web pages served to several
  people, each seeing and editing only part of the Items.
  - Sign-in with accounts in a YAML file, password hashes only; HTTPS.
  - Access is per Item, not per folder: a generated tree is a view, and one
    Item appears in several. What a person sees is the tree built from the
    Items they may read.
  - An `acl` field on `record`, inherited by every type, holds groups, not
    people. A Template's `defaults` fills it when a PDF is added. Only an
    owner or an admin may change it.
  - `users.yaml` holds groups and a `policy` of `if` expressions per group,
    for access without touching sidecars.
  - Readers are the owner, the members of the groups in `acl`, and the
    groups a policy matches. Grants only add; there is no deny. Write access
    comes from policy only.
  - Every endpoint filters: search, Browse, an Item, its PDF, `/api/values`,
    `about` links (a link to an unreadable Item shows no content), export,
    Outlines, Snapshots. Templates and Rules are admin only.
  - Concurrent writers need a lock or a version check per sidecar; history
    events record who made them.
  - Shipped as the same `dgs` binary in a Docker image, run as a server mode
    of `dgs doc`, not as a second program.
  - Groundwork worth doing before: every Item read goes through one function;
    history events carry an `actor`; `internal/doc/expr` knows nothing of
    rules.

- **Integrity** — not decided. The PDFs are the data to protect, so the
  answer is at the file level, not a database: content addressing, atomic
  writes, `verify` and backups.
  - An operation touching several files (a move on a changed date, a merge)
    writes each file as a temporary, reads it back, then renames it, as Photo
    Import does. Steps are ordered new before old and are idempotent, so an
    interrupted run leaves at worst an extra file, which `verify` reports and
    a rerun finishes.
  - `verify` also checks fields: a required one missing, a value its type
    rejects, an `about` naming an Item the tree lacks. Run before and after
    the convergence migration.
  - `verify --deep` hashes every PDF again to catch bit rot. A server runs
    `verify` daily and `--deep` weekly, and reports what it finds.
  - A deleted or damaged PDF comes back only from a backup (NAS snapshots, an
    off-site copy); `verify` finds it, and its digest checks the restore.

- **Clone**, and the three-way merge a cloned sub-tree allows.
