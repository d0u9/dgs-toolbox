# Doc Design

## Scope

`dgs doc` keeps important personal documents: passports, driver licences,
identity cards, insurance policies, certificates, payslips, utility bills, rent
receipts, bank statements, tax papers. It is not [`dgs box`](../box/index.md),
which holds low-importance scanned paper; the two stay separate commands.

### Several trees

One `dgs doc` can keep more than one tree — papers, and books — each a tree of
its own, in its own folder, with its own Templates, Views and Cases. Only PDFs
are kept, in every tree. `doc.trees` names them; the page switches from a menu
in its top bar, and the tree is in the page's address (`?tree=books`), so two
tabs can hold two trees and a reload keeps its tree. `verify` and `export` take
`--tree <name>`. Nothing crosses between trees: an Item, a View and a Case
belong to one. Each tree has its own Targets, and a Target's folder may not lie
inside any tree.

It follows `dgs box`'s shape: a TUI plus a local web page served by the same
process, where the interaction lives. Read [`../../tui.md`](../../tui.md) and
[`../../web.md`](../../web.md) before changing either. The parts, in order, are
in [`roadmap.md`](roadmap.md).

## Pages

Separate pages, linked from the top bar, as box's intake and browse are:

| Page | What it is for |
| --- | --- |
| Browse `/browse/` | The Items as cards or a table, filtered; preview, edit fields, revisions and HEAD, delete. Nothing is imported here. |
| Templates `/templates/` | Each type's Template file, edited as written; new and delete. |
| Views `/views/` | Building layouts, previewing the tree they make, and exporting (M4–M6). |
| Targets `/targets/` | Where Views are exported to: add, rename and delete Targets, set their folders, move Views to them, export. |
| Cases `/cases/` | A matter being dealt with and the Items it has needed; its export; archiving it. |
| Merge `/merge/` | Bringing a sub-tree or an export back in (M7). |
| Import `/import/` | Taking PDFs in. |
| Log `/log/` | What was done to the tree's Items, newest first, filtered. Last in the bar. |

### Browse

Browse opens on the Items as cards, as box's Browse does, or as a table,
chosen with two icon buttons, Cards (a grid of squares) and List (bulleted lines). In the list, each column is as wide
as its header's right edge is dragged, remembered per column, and a click on
a header sorts by that column, again to turn the order round; Sort offers the
same columns. Each card or row shows the first page of HEAD, the fields that tell the document apart
(owner, country), the type, and where its expiry stands. A row of filters
across the top narrows them: search (fields, tags and the text on the page), type,
a tag filter that matches every tag chosen,
a select for every distinguishing key any Template has, expiry and kind, and
a **★ Frequent** chip that shows only the frequent Items. They
sort by date added, expiry, name or type; frequent Items always come first
and the sort orders each part. The layout, sort, order and the Frequent chip
are remembered in the browser. A thumbnail too tall or wide for its card shows
its middle. A card's name wraps to a second line before it is cut short.
Picking one opens a detail panel beside the cards. At its top,
fixed while the rest scrolls, is the picked revision one page at a time (‹ ›
turn it), as tall as the reader drags it. Below scroll the fields,
revisions, notes, similar Items, and apart at the foot, delete. The panel is
as wide as the reader drags it, never narrower than 320px. Double-clicking a
card, or the picture, opens the revision in place of the cards, the panel
still beside it without its picture. Opening it is a step in the browser's
history: the browser's Back, Esc or ← Back return to the cards; Esc again,
or ×, closes the panel.

### Frequent

A few Items are opened far more often than the rest. The star on a card or
row, or the Frequent button in the detail panel, marks an Item frequent or
unmarks it. It is kept in the sidecar as `frequent: true`, so it travels with
the tree and a merge carries it in; marking and unmarking are history events.
"Frequent" rather than "favourite" or "highlight": it says why the Item is
first, and a highlight is something done to a PDF's text.

### Log

The Log page lists every history event of every Item in the tree, newest
first, as the server orders them (`tree.Log`). It filters by action, type,
Item, text, and frequent Items; picking an entry opens its Item on Browse.
An Item's own History on Browse is the same list for that one Item, and links
to the Log filtered to it.

Expiry is read from the first of `expires`, `expiry`, `expires_at`,
`expiry_date` and `valid_until` that HEAD has, by the server
(`internal/doc/expiry`): **expired** after its day, **expires soon** within
[`doc.expiring_within_days`](../../configuration/doc.md) days (90 by default), **valid**, **no end date** for `长期`, `永久`,
`permanent` or `indefinite`, and nothing when there is no such field or it is
not a date.

### Deleting

Nothing in a tree is erased. Deleting an Item on Browse, after the owner
confirms, moves its folder — sidecar and every revision's PDF — to
`trash/<item-id>-<time>/`. Deleting a Template moves its file to
`trash/templates/<type>-<time>.yaml`, and is refused while any Item has its
type. A revision's context menu offers Make HEAD and Delete revision. Deleting
a revision moves its PDF to `trash/<item-id>-<digest>-<time>.pdf`; when it was
HEAD, the latest remaining revision becomes HEAD. The final revision is
removed by deleting the Item. Something deleted by mistake is moved back by hand.

### Templates

The Templates page edits a Template's file as text and saves it exactly as
written, comments included, after it parses and validates. A type Items use
cannot be renamed and its kind cannot change, since every sidecar names both;
its fields can. Renaming an unused Template moves the old file to the trash.
Duplicate starts a new, unsaved Template from the one shown, under the type
`<type>_copy` (`_copy2`, … when taken); it is written only on Save.

The file is edited in the shared code editor: line numbers, YAML colours and
folding. An unsaved edit is marked, Ctrl/Cmd+S saves, and leaving the Template
asks first. Beside it a panel says what a Template can say. Deleting is in the
same panel, apart from Save, and waits until the Template's type is typed out;
a Template Items use cannot be deleted.

### Import

1. **Open folder…** opens the shared file dialog in folder mode, at the folder
   opened last time.
2. The page shows that folder's PDFs as a tree, subfolders included and kept
   as folders rather than flattened. Folders with no PDF in them are left out.
   PDFs already in the tree are marked.
3. Picking a PDF previews it; the form beside it takes a Template and its
   fields. Into shows existing Items of that type as cards, alongside New item;
   choosing an existing Item asks only for its revision fields, while New item
   asks for the distinguishing fields. After
   an import the next PDF not yet imported is picked.
4. The opened folder is only read. The server serves and imports only PDFs
   under it.

### Preview

A scanned PDF is shown as pictures of its pages, drawn from each page's own
embedded scan as box draws them, 1600 px on the long side, loaded as they
scroll into view. The browser's PDF viewer redraws a full-resolution scan on
every scroll and stutters. A PDF whose first page is not a scan — one made on
a computer — is shown in the viewer.

Beside the pages is a strip of thumbnails, the page in view marked; one
clicked is scrolled to. With no saved zoom choice, a PDF opens fit to page.
The pages zoom — the bar's buttons (fit width, fit
page, in, out, 100%), Ctrl or ⌘ with the wheel or a trackpad pinch, and
`+` `−` `0` — about the pointer, and the zoom is kept for the next PDF. A page
zoomed past its picture's size is drawn again from the scan at up to 3600 px.
Dragging a page pans it; a drag that starts on text selects the text.

### Text and suggestions

The PDF's text is laid over the pictures of its pages, each line where it was
read, invisible until it is hovered or selected, so it is copied straight off
the page. It is the PDF's own text layer when it has one, else recognised from
the rendered page. There is no separate text panel: most PDFs are never
copied from. Recognition is PDFKit and
the Vision framework on macOS at its accurate level, compiled in through cgo;
elsewhere, and without cgo, the page says it is unavailable. The first four
pages are read.

Recognising a page takes about a second, so pages are read through a queue:

- page by page, two at once, and each is shown as soon as it is read;
- the page being looked at first, then the rest of its PDF, ahead of anything
  read in advance;
- while a PDF is looked at on Import, the next three not yet in the tree are
  read in advance;
- every page read is kept in the local cache (`doc.cache_dir`) by the PDF's
  SHA-256, so it is read once per machine, and a PDF met again in another
  folder or tree is not read again.

On import, each empty field whose Template `pattern` matches the text is filled
and marked as a suggestion; hovering or focusing it outlines the line on the
page it was read from. A field the reader typed in is never replaced, and
nothing is kept until the reader imports. Searching Items by their text, from
the same cache, comes later.

Loose PDFs inside the tree are allowed; they are imported by opening the
tree's own folder. Images are not imported.

## The four layers

```text
Stored object  →  Item  →  View  →  Target
```

- **Stored object** — the bytes of one PDF, identified by its SHA-256.
- **Item** — what the owner says the document is.
- **View** — which Items are selected, which of their revisions, and the
  relative path each one gets.
- **Target** — a local directory the View's tree is written under. Targets
  are per-machine configuration. `dgs doc` writes only to the directory; getting
  it into iCloud Drive, Google Drive or a NAS is a sync the owner does by hand.

The repository's own layout carries none of the View's meaning. A View names
the one Target it is exported to, and a Target takes as many Views as it needs:
Google Drive can hold one View for identity documents and another for bills,
each with its own layout, while iCloud Drive holds a third that lays out the
same identity documents another way. The repository does not change.

## Items

Every document is an Item with a stable, globally unique ID (UUIDv7 or ULID),
separate from the SHA-256 of its file: the ID is logical identity, the digest
is content identity. There are two kinds.

### Documents: revisions and HEAD

A document is one thing that is reissued over time: a driver licence is
renewed, and the new card is a new **revision** of the same Item. Each revision
is exactly one PDF; front and back are scanned into that one PDF, never as two
files.

Each document has a **HEAD** that points at one revision, like git's HEAD.
Adding a revision moves HEAD to it. HEAD can be moved back by hand, for a
later import that turns out to be a rescan of an older card. What is current is
HEAD, not the highest revision number and not a computation over dates.

Browse links to a Change type page for an Item. The left column shows its
current fields and each revision's values; the right column chooses another
Template of the same kind and asks for its Item and revision fields. The
change is checked and saved in one sidecar write. The Item ID, PDFs, revisions,
HEAD, notes and tags remain. Fields absent from the target Template leave the
current fields and remain in the previous-values snapshot in history.

### Records

A record is one independent historical paper: a payslip, a bill, a statement.
It has no revisions and no HEAD.

### Telling documents of one type apart

The owner fills in fields that tell an Item apart from others of the same type,
such as `country`: Jane may have a Chinese and an Australian driver licence,
which are two Items. For documents, `owner` + `type` + these fields name one
Item and must be unique; import refuses a second Item with the same ones.

## Templates and Views live in the repository

Templates and Views describe the documents, so they are kept in the repository
and travel with it when trees are merged. Only Targets, which are paths on one
machine, are configuration.

## Templates

A Template describes one type for import: the fields it fills in by default
and the fields it asks for. Adding a PDF means picking a Template and answering
only what it asks. Nothing is filed automatically: the page may suggest a
Template (see [Suggesting a type](#suggesting-a-type)), and the owner picks.

A field has a `type`, which checks its value and chooses the control the page
shows for it:

- `text` — the default.
- `date` — `YYYY-MM-DD`. A date field with no `pattern` is suggested from any
  date the text holds (see [Dates in the text](#dates-in-the-text)).
- `select` — one of the field's `options`.
- `item` — the ID of another Item, such as a passport's previous passport. The
  page picks it from the Items; export never follows it.
- `country` — a country however it is typed: its ISO code (`cn`, `CHN`), its
  English or Chinese name (`China`, `中国`) or a common alias
  (`中华人民共和国`, `PRC`), any case. It is kept in the field's `format`:
  `zh` (`中国`, the default), `en` (`China`), `alpha2` (`CN`) or `alpha3`
  (`CHN`). A name dgs does not know is refused. Two documents whose country
  is written differently, `AU` in an older sidecar and `澳大利亚` now, are
  the same document. The table is ISO 3166-1, in `internal/doc/country`.

A field of a document may be `per_revision: true`: its value belongs to each
revision rather than to the Item. A renewed card has its own number and expiry
and the old card keeps its own. Import asks for every field; adding a revision
asks only for the per_revision ones, suggested from the new PDF's text. On
the same form, the Item's current tags and notes can be edited and are saved
with the new revision; they belong to the Item, not to an individual revision.
On Browse, picking a revision shows and edits the fields as that revision has
them. Everything else — the page's list, a View's query — uses HEAD's. A
layout key is the exported revision's own value, so exporting all revisions
can name the old card and the new one differently. A distinguishing field
names the document, so it cannot be per_revision, and a record has one PDF,
so its fields cannot be either. A value a sidecar keeps at the Item for a key
made per_revision later stands for every revision without its own, until that
revision's fields are saved.

A sidecar may also hold `notes`, free text the owner writes. Notes are never a
key and never exported. The Item sidecar also keeps timestamped history events
for imports, field and metadata edits, HEAD changes, type changes, revision
deletion, PDFs actually written by exports, merges, and marking an Item
frequent or not. Older sidecars remain readable;
their revision `added` times appear as import history on Browse and Log
(`tree.Timeline`). Each event
has an operation time. Type changes keep the previous type and fields in the
history event.

### Suggesting a type

After a PDF's first page is read, the page ranks the Templates by how much
that page looks like the first pages of Items already filed under each: word
counts compared by cosine similarity, the best Item of a type standing for it.
A word is a run of letters or digits, lowercased, except digits alone, which
differ between copies of one form; Chinese and Japanese text counts as pairs
of adjacent characters. Only Items whose text has been read, on Import or
Browse, take part. The top Template is preselected, unless the owner already picked one for this
PDF, when its score passes a threshold (named, with a default, in the algorithm's package) and the
type has at least one Item; the owner can always pick another. It learns
nothing and stores nothing: the texts come from the rebuildable text cache.

### Dates in the text

Dates are found in the text in the forms scans carry: `2026-09-25`,
`2026.09.25`, `2026年9月25日`, and `25/09/2026`. A form with day and month in
digits both ways is read in the order `doc.date_order` names (`DMY` by
default). A date field with a `pattern` keeps only a match that is a date, written
`YYYY-MM-DD`; one with none takes the next date in the text that no earlier
date field took. A date the Template lists under `ignore_dates` — a birthday on every
page of a person's documents — is never suggested.

A key added to Items of a type (see [Missing keys](#missing-keys)) is added to
the type's Template, so later imports of that type ask for it.

## Views

A View has a query, a selection, and a layout.

- **Query** — which Items, by their fields, such as type and owner.
- **Selection** — `head` (a document's HEAD revision only) or `all`.
- **Layout** — fixed text and keys, such as
  `{country}/{owner}/important/{type}.pdf`. The Target provides the root, so
  a layout never names iCloud or a NAS. On the web page the layout is built by
  picking keys, with a preview of the resulting tree.

Keys are the Item's own fields (`owner`, `type`, `country`, and whatever its
Template defines, such as `employer`), values derived from them (`year`,
`month` and `date` from `issued_at`), `revision` (the revision's number,
counting from 1, in order added) and `ext`.

A key may name a country format: `{country:alpha3}` writes the Item's country
as `CHN` however its sidecar keeps it, and likewise `:zh` (`中国`), `:en`
(`China`) and `:alpha2` (`CN`). A value that names no country is written as it
is. Any other format after the colon is refused.

A View is one YAML file under `views/`, named after its `name`:

```yaml
name: important
query: {type: [id_card, passport], owner: jane}
selection: head
layout: '{country}/{owner}/important/{type}.{ext}'
default: none              # optional: stands in for a missing key
target: icloud             # optional: the Target it is exported to
```

`target` is a name from the tree's `targets.yaml`, never a path. The tree
says where its Views go; the folder each goes to is chosen when exporting:

```yaml
icloud:
  about: read on the phone
  folder: ~/Library/Mobile Documents/com~apple~CloudDocs/Documents
kindle:
  about: books for the flight     # no folder: one is chosen every time
```

- A Target's `folder` is only its default. A leading `~` is the home folder
  of the machine exporting, so one `targets.yaml` usually fits every Mac.
- On the Views page each Target shows, under its name, the folder it goes to
  this time; clicking it chooses another through the file dialog. The choice
  is remembered in that browser, marked "this machine", and the page offers to
  make it the Target's own folder in `targets.yaml` instead.
- A Target with no folder of its own, and none chosen, stops the run until one
  is chosen.
- The Targets page manages them: adding one, what it is for, its own folder
  and this browser's, the Views exporting there (moving one in, taking one
  out, starting a new one for it), exporting it or every Target, renaming —
  every View naming it follows — and deleting one no View names. The Target
  select in a View's form lists them and adds a new one too.
- `dgs doc export --to kindle=/Volumes/Kindle/documents` chooses a folder on
  the command line; comma separate several.

A Target belongs to its tree, so two trees each have their own `kindle`.
Where a machine keeps a Target's folder is never configuration: the tree
names the default and the export may choose another.

A layout is rendered by three rules:

- **Missing keys.** Without `default`, a missing key is refused (below). With
  it, the key is written as that text instead.
- **Clean segments.** Each key's value has `/`, `\`, control characters and
  the characters Windows forbids replaced by `_`, and leading or trailing dots
  and spaces trimmed, so a value never adds a folder or escapes the Target.
- **Duplicates.** Two PDFs landing on one path is refused, unless the View
  sets `dedupe: number`: then, in the
  Items' ID order, the second and later get `_01`, `_02` before the extension,
  so the same state always numbers the same way.

### The Views page

The page lists the Views on the left. The form picks the types, adds
conditions on other keys (`owner is jane, tom`), HEAD or all revisions, and
the layout, typed or built by clicking key chips that insert at the caret.
The chips are grouped: the Templates' fields, the keys every PDF has, and each
country field's formats. Typing `{` in the layout lists the keys with what
each writes; after a country key's `:` it lists the formats with an example.
Beside it the preview redraws, as the form changes, the folder tree an
export would write, each file linked to its Item on Browse, with the PDFs
lacking a key and the paths wanted twice listed above it. Saving writes
`views/<name>.yaml`; renaming a View removes the old file.

### Missing keys

Export refuses to write anything while a PDF the View selects lacks a key the
layout uses, and lists each PDF with what it is missing. The page lets the
owner fill the key in, on one PDF or on several at once. Only PDFs the View
selects have to have it; the rest of the type is not checked. `year`, `month`
and `date` are filled through the date field they come from; a key no field of
the Item's Template supplies cannot be filled, only named — the layout or the
Template has to change.

The same refusal applies when two PDFs would land on one path.

## Export

Exports are deterministic: the same repository state and View produce the same
tree. Before writing, an export computes the whole plan — add, replace,
remove — and can show it without writing (dry run).

A file is published under the same contract as
[`../photo/import.md`](../photo/import.md): written under a `.dgs-part` name,
read back independently, and renamed to its final name only when the readback
matches its SHA-256.

Export removes only files it wrote itself, recorded in a manifest at the
Target root; anything else under the Target is never touched.

A Target is exported as a whole: every View naming it, planned together. A
run is one Target, several, or every Target a View names, and it is checked
entirely before anything is written. Any of these stops the whole run, and no
Target is written:

- a key a selected PDF lacks, in any View;
- two PDFs wanting one path, ignoring case, within a View or between two
  Views of a Target, and a PDF wanting a path that another needs as a folder;
- a wanted path held by a file the export does not own (below), including one
  another View exported into the same folder;
- a Target inside the tree or holding it, two Targets of the run overlapping,
  a View naming a Target `targets.yaml` does not have, a Target with no
  folder, a folder inside another tree, and a manifest that
  cannot be read.

The check lists every one of them, by View and path; the export itself plans
and checks again from the state at that moment.

The Views page lists the Views grouped by Target, each group with **Export…**,
and **Export all Targets…** above them; a View's own form picks its Target.
A View naming no Target is exported alone to a folder chosen in the shared file
dialog, which can make a new folder there; the last one chosen is remembered.
`dgs doc export [<target>...]` does the same from the command line, every
Target when none is named; `-n` checks and writes nothing. It fails, writing
nothing, when the check finds anything.

- A wanted path held by a file the manifest does not record blocks the export:
  nothing is written until the owner moves it. A file already there with the
  wanted content is adopted instead.
- A recorded file changed since it was exported is never replaced or removed.
  Wanted, it blocks the export; no longer wanted, it is left where it is and
  the manifest forgets it.
- Folders a removal leaves empty are removed.
- The manifest is rewritten at the end, and also when an export stops early,
  so the next export only does what is left.

Export is incremental: a file whose path and SHA-256 the manifest already
records, and which still reads back to that digest, is not written again.

The manifest, `dgs-export.json`, records for each file its path, digest, the
View that placed it, the Item's ID and revision, and the Item's fields at
export time. An export of some of a Target's Views replaces and removes only
those Views' files. A version 1 manifest, from before a Target took several
Views, is read with its files belonging to its one View. That is enough
to import a Target back into a repository: `dgs doc` reads the manifest and
recreates the Items, matching existing ones as a merge does. It also
records each Item's kind and which revision was HEAD.

## Cases

A Case is a matter being dealt with — renewing a passport, a lease
application — and the Items it has needed so far. What such a matter wants is
seldom known at the start: a photo ID first, a bill later, a payslip after
that. So a Case grows as it goes, then is archived when it is done.

- A Case refers to Items; it copies nothing. It lives in the tree as
  `cases/<name>.yaml`, so it moves, merges and is backed up with the tree.
  The name is the file's; the title is what the page shows.
- An Item goes in from the Cases page, or from the Item on Browse ("Add to a
  Case"), with a note on why it was wanted. Browse shows the Cases an Item is
  in.
- **Still needed** lists what was asked for that the tree does not have yet —
  "the last three electricity bills". Once the Item is filed, it meets the
  need and goes into the Case with the need's text as its note.
- The Case shows each Item's expiry, as Browse does, so an ID that will not
  last the matter out is seen early.
- **Export** copies the Case's Items into a folder to hand over, named by the
  Case's own layout (`{type}.{ext}` unless set, two of a type numbered). It
  is planned and checked as a View's export is and writes `dgs-export.json`,
  so a second export only brings the folder up to date. This takes the place
  of the Bundle once planned.
- **Archive** closes the Case: for each Item it records the revision that was
  current, and nothing changes until it is reopened. An archived Case's
  export takes those revisions, so after a passport is renewed it still
  shows, and exports, the one handed over. Nothing is copied to do this; if a
  recorded revision leaves the tree, the Case says so.
- Deleting a Case moves its file into `trash/cases/`. Its Items stay.

A Case belongs to one tree. In a tree of books, the same thing is a reading
list — for a flight, for a course — which is simply never archived.

## Searching

Browse's filter matches an Item by its fields and by the text read off its
current PDF: every word typed must appear, ignoring case, and a match shows the
line around it. A word matches inside a longer one, so Chinese, written
without spaces, matches as it is typed. "More like this" lists the Items whose
text is most like the one picked, by the same similarity as
[Suggesting a type](#suggesting-a-type).

Only text already in the cache is searched; nothing is read to answer a
search. Browse says how many Items have no text yet and can queue them all to
be read.

## Verify

`dgs doc export [<target>...]` is described under [Export](#export).

`dgs doc verify [<tree>]` (`-q` for the result only) checks the repository against its sidecars and
changes nothing. It reports:

- a revision whose PDF is missing, or whose content no longer hashes to its
  name;
- a PDF under `items/` no sidecar names;
- a sidecar that does not parse, names an unknown Template, or whose HEAD is
  not one of its revisions.

It exits non-zero when it reports anything.

## Metadata: sidecars are the truth

As in [`dgs box`](../box/index.md#metadata-the-sidecar-is-the-truth), each PDF
has a YAML sidecar beside it, and the sidecars are the only metadata truth.
Everything else — the index the page queries, thumbnails — is a cache: it lives
on the local machine, outside the repository, and can be deleted and rebuilt
from the sidecars at any time. The owner edits on the page, never in YAML.

A cache is not a database: [`AGENTS.md`](../../../AGENTS.md) allows none, so
the index is held in memory and saved as a file, as `box`'s is.

## Repositories and merging

A **full tree** lives at home on the NAS. A **sub-tree** is a temporary
repository, usually on a laptop. Either is edited the same way; which one is
home matters only when merging.

A sub-tree is created with `init` on its own. Cloning one from the full tree
is deferred (see [Later](#later)). A merge brings the sub-tree's changes into
the full tree:

- The same PDF is recognised by its SHA-256 and stored once.
- There is no common state to compare against. Items are matched to the
  full tree's by `owner` + `type` + the distinguishing fields. A matched
  document's new revision is added and HEAD moves to it; the merge lists these
  for confirmation first. A matched Item whose fields differ is a conflict. An
  unmatched Item is added as new. Records are matched by digest alone.
- HEAD moved on both sides is a conflict.

Conflicts are resolved by hand on the page before the merge completes.

The Merge page opens the folder to merge in and only reads it. It shows what
the merge would do — new Items, revisions added, HEADs moved, new Templates
and Views — and each conflict with both sides, to keep this tree's or take
theirs. The merge itself plans again from what both sides hold then, and is
refused while a conflict has no choice.

- An Item is matched first by its ID, so merging the same sub-tree again finds
  what it brought last time and changes nothing.
- HEAD: a side whose HEAD the other has never seen moved it. When one side
  did, HEAD goes there. When both did and they share a revision, it is a
  conflict; a sub-tree made on its own shares none, and HEAD moves to its new
  revision.
- Notes are never a conflict: notes the tree lacks are added below its own.
- Tags and Frequent are never a conflict: the tree gains the Source's tags,
  and an Item frequent on either side is frequent after.
- A new Item keeps the history it had in the other tree. Every Item the merge
  adds or changes gets a `merge` history event, listing what it changed.
- A Template or View both sides have, different, is a conflict. Only the
  fields that differ are shown.
- What no choice resolves stops the merge: an Item whose type has no Template
  on either side, two Items matching one, a record holding another PDF here.
- PDFs are copied in with the same readback as import, before the sidecar
  naming them is written.

A Target imports back the same way: the Merge page opens a folder with a
`dgs-export.json` as it opens a sub-tree. It brings the revisions that were
exported and the Items' fields; a Target carries no Templates or Views.

## Layout on disk

```text
<tree>/
  dgs-doctree.yaml          # the marker: dgs doc init writes it, nothing else does
  templates/
    id_card.yaml            # one Template per type
  views/                    # one View per file (M4)
  cases/                    # one Case per file
  targets.yaml              # where Views are exported to, by name
  trash/                    # deleted Items and Templates, never erased
  items/
    <item-id>/
      item.dgs-item.yaml    # the sidecar: everything known about the Item
      <sha256>.pdf          # one PDF per revision, named by its content
```

- The marker is `dgs-doctree.yaml`, not `dgs-doc.yaml`, which reads too much
  like box's `*.dgs-doc.yaml` sidecars. It is visible, as box's is.
- An Item's folder is named by its ID, so changing `owner` or `country` moves
  nothing. Readable paths are what a View exports.
- A PDF is named by its SHA-256, so the same file is stored once and merging
  matches by digest.
- An export writes `dgs-export.json` in the Target (M6).
- PDFs anywhere else in the tree are loose, and allowed. Import copies a PDF in
  from wherever it is and leaves the original where it was.

## Template format

```yaml
type: id_card
description: Identity cards and household registers
kind: document            # document: revisions and HEAD; record: one PDF
fields:
  - key: owner
    required: true
    distinguishing: true  # type + the distinguishing fields are unique
  - key: country
    type: country
    format: zh              # zh 中国, en China, alpha2 CN, alpha3 CHN
    required: true
    distinguishing: true
  - key: number
    per_revision: true      # each revision keeps its own
    pattern: '(\d{17}[\dXx])'
  - key: expires
    per_revision: true
    pattern: '[-－—–一~～至]\s*(\d{4}[.\-/]\d{2}[.\-/]\d{2}|长期)'
  - key: issued
    type: date
  - key: previous
    type: item
  - key: status
    type: select
    options: [valid, expired]
ignore_dates: [1990-01-01]  # never suggested as a date
defaults:
  country: AU
```

The file is named after its `type`. `description` is a short explanation shown
on the Import card and in the Templates list; older Templates may omit it.
A `select` field lists its allowed values under `options`. A field's `pattern` is a regular
expression that suggests its value from the document's text (below): the
first capture group, else the whole match. A value written more than one way
takes `patterns`, a list tried after `pattern`, in order: the first to find a
value that fits the field suggests it, so a date field passes over a match
that is not a date and tries the next.

```yaml
  - key: expires
    type: date
    patterns:
      - '有效期限.*[-－]\s*(\d{4}\.\d{2}\.\d{2})'   # 2016.01.01-2036.01.01
      - '(?i)expiry\W*(\d{2}/\d{2}/\d{4})'            # Expiry: 03/04/2031
``` The sidecar:

```yaml
id: 01J8...               # ULID
type: id_card
kind: document
fields: {owner: jane, country: AU}
head: <sha256>            # documents only
revisions:
  - {digest: <sha256>, added: 2026-09-25T10:00:00+10:00, source: scan.pdf}
```

A record has exactly one revision and no `head`. `notes` and `tags`, when the
owner has written any, are Item keys in the sidecar. Tags are lower case, with
spaces joined by hyphens, and duplicates removed. Import and Browse offer
existing tags as the owner types; Browse filters by tags in the current tree.

## Later

- **Archive serial numbers and barcode separator pages**, if box wants them:
  they concern paper, not filed documents.

- **Snapshot** — an export into a new dated directory that is never updated,
  recording what was submitted for a visa or a claim.
- **Clone** — a sub-tree cloned from the full tree would remember the state it
  was cloned from, so its merge could compare three sides, as git does.

Its settings — `doc.root`, `doc.web.port` — are in
[`configuration/doc.md`](../../configuration/doc.md).
