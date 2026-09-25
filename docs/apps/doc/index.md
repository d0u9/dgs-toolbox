# Doc Design

## Scope

`dgs doc` keeps important personal documents: passports, driver licences,
identity cards, insurance policies, certificates, payslips, utility bills, rent
receipts, bank statements, tax papers. It is not [`dgs box`](../box/index.md),
which holds low-importance scanned paper; the two stay separate commands.

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
| Merge `/merge/` | Bringing a sub-tree or an export back in (M7). |
| Import `/import/` | Taking PDFs in. Last in the bar. |

### Browse

Browse opens on the Items as cards, as box's Browse does, or as a table:
each shows the first page of HEAD, the fields that tell the document apart
(owner, country), the type, and where its expiry stands. A row of filters
across the top narrows them: search (fields and the text on the page), type,
a select for every distinguishing key any Template has, expiry and kind. They
sort by date added, expiry, name or type. The layout, sort and order are
remembered in the browser. A thumbnail too tall or wide for its card shows
its middle. Picking one opens a detail panel beside the cards. At its top,
fixed while the rest scrolls, is the picked revision one page at a time (‹ ›
turn it), as tall as the reader drags it. Below scroll the fields,
revisions, notes, similar Items, and apart at the foot, delete. The panel is
as wide as the reader drags it, never narrower than 320px. Double-clicking a
card, or the picture, opens the revision in place of the cards, the panel
still beside it without its picture. Opening it is a step in the browser's
history: the browser's Back, Esc or ← Back return to the cards; Esc again,
or ×, closes the panel.

Expiry is read from the first of `expires`, `expiry`, `expires_at`,
`expiry_date` and `valid_until` that HEAD has, by the server
(`internal/doc/expiry`): **expired** after its day, **expires soon** within 90
days (`DefaultSoon`), **valid**, **no end date** for `长期`, `永久`,
`permanent` or `indefinite`, and nothing when there is no such field or it is
not a date.

### Deleting

Nothing in a tree is erased. Deleting an Item on Browse, after the owner
confirms, moves its folder — sidecar and every revision's PDF — to
`trash/<item-id>-<time>/`. Deleting a Template moves its file to
`trash/templates/<type>-<time>.yaml`, and is refused while any Item has its
type. Something deleted by mistake is moved back by hand.

### Templates

The Templates page edits a Template's file as text and saves it exactly as
written, comments included, after it parses and validates. A type Items use
cannot be renamed and its kind cannot change, since every sidecar names both;
its fields can. Renaming an unused Template moves the old file to the trash.

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
   fields, and whether it is a new Item or a new revision of a document. After
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
clicked is scrolled to. The pages zoom — the bar's buttons (fit width, fit
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

The repository's own layout carries none of the View's meaning. The same View
can be written to several Targets — iCloud Drive, a synced Google Drive
folder, a NAS — and the repository does not change.

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
Browse, picking a revision shows and edits the fields as that revision has
them. Everything else — the page's list, a View's query — uses HEAD's. A
layout key is the exported revision's own value, so exporting all revisions
can name the old card and the new one differently. A distinguishing field
names the document, so it cannot be per_revision, and a record has one PDF,
so its fields cannot be either. A value a sidecar keeps at the Item for a key
made per_revision later stands for every revision without its own, until that
revision's fields are saved.

A sidecar may also hold `notes`, free text the owner writes. Notes are never a
key and never exported. An Item's history is its revisions; no other log is
kept.

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
```

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

The Views page exports the saved View to a folder chosen in the shared file
dialog, which can make a new folder there; the last one chosen is remembered.
Targets, the named folders in [`doc.targets`](../../configuration/doc.md), are
offered beside it as shortcuts. A dry run lists what would be
added, replaced and removed, and the export itself plans again from the state
at that moment. Nothing is written while the View is incomplete. A Target,
chosen or named, may not overlap the tree.

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
Item's ID and revision, and the Item's fields at export time. That is enough
to import a Target back into a repository: `dgs doc` reads the manifest and
recreates the Items, matching existing ones as a merge does. It also
records each Item's kind and which revision was HEAD.

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

The file is named after its `type`. A `select` field lists its values under
`options`. A field's `pattern` is a regular
expression that suggests its value from the document's text (below): the
first capture group, else the whole match. The sidecar:

```yaml
id: 01J8...               # ULID
type: id_card
kind: document
fields: {owner: jane, country: AU}
head: <sha256>            # documents only
revisions:
  - {digest: <sha256>, added: 2026-09-25T10:00:00+10:00, source: scan.pdf}
```

A record has exactly one revision and no `head`. `notes`, when the owner has
written any, is one more key of the sidecar.

## Open questions

- **Whether box keeps `identity` and `insurance`** once doc exists — decided
  after doc is built.

## Later

- **Archive serial numbers and barcode separator pages**, if box wants them:
  they concern paper, not filed documents.

- **Snapshot** — an export into a new dated directory that is never updated,
  recording what was submitted for a visa or a claim.
- **Clone** — a sub-tree cloned from the full tree would remember the state it
  was cloned from, so its merge could compare three sides, as git does.
- **Bundle** — a hand-picked set of Items for one purpose, exported once.

Its settings — `doc.root`, `doc.web.port` — are in
[`configuration/doc.md`](../../configuration/doc.md).
