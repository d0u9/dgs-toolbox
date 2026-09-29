# Doc Design

## Scope

`dgs doc` keeps important personal documents: passports, driver licences,
identity cards, insurance policies, certificates, payslips, utility bills, rent
receipts, bank statements, tax papers. It is not [`dgs box`](../box/index.md),
which holds low-importance scanned paper; the two stay separate commands.

### Several trees

One `dgs doc` can keep more than one tree — papers, and books — each a tree of
its own, in its own folder, with its own Templates, Outlines and Cases. Only PDFs
are kept, in every tree. `doc.trees` names them; the page switches from a menu
in its top bar, and the tree is in the page's address (`?tree=books`), so two
tabs can hold two trees and a reload keeps its tree. `verify` and `export` take
`--tree <name>`. Nothing crosses between trees: an Item, an Outline and a Case
belong to one, and a folder an Outline is exported to may not lie inside any
tree.

It follows `dgs box`'s shape: a TUI plus a local web page served by the same
process, where the interaction lives. Read [`../../tui.md`](../../tui.md) and
[`../../web.md`](../../web.md) before changing either. The parts, in order, are
in [`roadmap.md`](roadmap.md).

## Pages

Separate pages, linked from the top bar, as box's intake and browse are:

| Page | What it is for |
| --- | --- |
| Browse `/browse/` | The Items as cards or a table, filtered; preview, edit fields, revisions and HEAD, delete and attach or replace a PDF on a selected revision. |
| Explore `/explore/` | An Outline's result: its PDFs in their tree, its Snapshots as folders, each folder counting the PDFs beneath it; picking a folder lists it, a PDF opens beside it; exporting the tree. |
| Templates `/templates/` | Each type's Template file, edited as written, listed as the tree of what extends what; new and delete. |
| Rules `/rules/` | Making a rule: which PDFs it picks and the path each has, with the tree it makes redrawn as it changes. |
| Snapshots `/snapshots/` | Taking a Snapshot from a rule, or beginning one empty, and editing its PDFs by hand. |
| Outlines `/outlines/` | Making an Outline from rules and Snapshots, each Snapshot at a folder given, and the folder it is exported to, with its tree redrawn as they change. |
| Cases `/cases/` | A matter being dealt with and the Items it has needed; its export; archiving it. |
| Merge `/merge/` | Bringing a sub-tree or an export back in (M7). |
| Import `/import/` | Taking PDFs in. |
| Log `/log/` | What was done to the tree's Items, newest first, filtered. Last in the bar. |

### Issuer suggestions

Every text `issuer` field offers an editable list of previously saved issuers.
The scope is **current tree + document type + country**: both type and country
must match. Issuers from another type or country are not useful candidates,
even when their names happen to match. The owner is not part of the scope: one authority
issues a type to everyone in its region, so another owner's issuer is a
candidate too. This applies to every Template with a
text `issuer` field, not just visas; no Template change is needed.

- Import (including creation without a PDF), field editing and change-type
  forms use the same control. Changing the selected Template or country updates
  the candidates. Change-type forms use the target type; an old revision uses
  its own type and country.
- Country names and codes are normalized for matching: `AU`, `AUS` and
  `澳大利亚` identify the same country. The form uses the entered country, or
  the Template's country default when applicable. Without a recognized country,
  there are no candidates; it never falls back to a cross-country list.
- Choose a saved name or type a new one. Saving the Item makes that name
  available in the same type-and-country scope next time. Suggestions are
  optional, not validation constraints. Updating candidates does not replace
  an issuer already entered.
- Candidates come from Item sidecars, including retired Items and retained
  revisions. A revision's own type and country determine where its issuer is
  suggested; a later change of type or country must not relabel that history.
  Values are sorted and deduplicated after trimming surrounding whitespace;
  empty values are omitted.

There is no separate issuer catalogue, database or browser-only history.
Deleting all saved occurrences removes a name from the candidates. This keeps
the list reproducible from the tree, including after moving it to another
machine, and avoids a second source of metadata truth.

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
a Use filter for Items in use or retired, and a **★ Frequent** chip that
shows only the frequent Items. The type and key selects offer only the
values the other filters leave — choosing a type narrows the owners to that
type's — and a key with no value left is hidden. They
sort by date added, expiry, name or type; frequent Items always come first
and retired ones last, and the sort orders each part. The layout, sort, order and the Frequent chip
are remembered in the browser. A thumbnail too tall or wide for its card shows
its middle. A card's name wraps to a second line before it is cut short.
Picking one opens a detail panel beside the cards. At its top,
fixed while the rest scrolls, is the picked revision one page at a time (‹ ›
turn it), as tall as the reader drags it. Below scroll its sections in four named groups: Details (fields, tags,
notes), Revisions and history, Related (Cases, sharing, similar Items),
and Retire or delete, with delete apart at the foot. Beside the page
buttons, Attach PDF… adds a PDF to a revision without one and Replace PDF…
saves another PDF as a new revision of one that has it. The panel is
as wide as the reader drags it, never narrower than 320px. Double-clicking a
card, or the picture, opens the revision in place of the cards, the panel
still beside it with every section and Replace PDF…, only without its picture and page buttons. Opening it is a step in the browser's
history: the browser's Back, Esc or ← Back return to the cards; Esc again,
or ×, closes the panel.
Browser viewer, beside the title here and at the foot of the preview on
every other page that shows a PDF (Import among them), shows it in the browser's own viewer
instead of the pages dgs draws from each page's embedded image, to tell a
PDF dgs draws wrongly — a layered scan whose text is a masked second image —
from one that is itself wrong. The choice is kept in that browser and holds
on every such page.

### Frequent

A few Items are opened far more often than the rest. The star on a card or
row, or the Frequent button in the detail panel, marks an Item frequent or
unmarks it. It is kept in the sidecar as `frequent: true`, so it travels with
the tree and a merge carries it in; marking and unmarking are history events.
"Frequent" rather than "favourite" or "highlight": it says why the Item is
first, and a highlight is something done to a PDF's text.

### Retired

Some documents stop being used without expiring: a staff card or a local
residence card once the owner has left the organisation or the place. Retire,
in the detail panel, sets such an Item aside, with an optional reason ("left
the company"). It is kept in the sidecar as `retired: true` and
`retired_reason`, and retiring, changing the reason and putting it back in use
are history events. Browse still shows a retired Item, faded, in grey, with a
"retired" badge whose hover gives the reason, and after every Item in use,
whatever the sort. The Use filter shows only Items in use or only retired
ones. Outlines and exports are not affected. A merge retires a matched Item the
Source has retired.

"Retired" rather than "expired", which is about a date, or "archived", which
Cases already use.

### Shared

A letter addressed to two people belongs to both, but an Item has one
owner: the owner field is one value everywhere, in a distinguishing key, a
layout's `{owner}` and a rule's `if`. So the letter is imported once, as the
owner's (the person named first), and shared with the other: Shared with, in
the detail panel, lists the people it is shared with, from the owner field's
options, or Import's Shared with for a new Item. It is kept in the sidecar as `shared_with`, sorted, never naming the
owner, and changing it is a history event. A merge adds the Source's people
to the matched Item's.

On Browse, filtering by owner shows an Item shared with that person too,
marked "shared by" its owner. An Outline is not changed by sharing: a rule
selecting `owner: emma` does not select an Item Emma shares, unless it sets
`shared: true`. Its layout still writes the Item's own owner for `{owner}`.

"Shared" rather than several owners: a second owner would make every key,
path and `if` that reads `owner` take a list, and a PDF would have more
than one place in an export.

### Visa replacement: separate Items, explicit relationship

Each visa grant is a separate record: it has its own grant number, dates and
supporting letter. A later grant does not become a revision of the earlier
grant. Revisions describe changes to the same record, such as corrected
fields or an attached scan; replacement describes a relationship between two
records. Keeping both Items preserves their documents and any Cases that
already refer to them.

The same owner and country are not enough to infer replacement. The owner
may need to keep multiple visas in use, so importing a visa never retires
another automatically. **Replaces** is an explicit, optional choice,
defaulting to none, on both PDF imports and creation without a PDF. Candidates
are in-use visas for the same owner and country. The detail panel also lets
the reader apply the relationship after both Items already exist. This
feature currently applies to the `visa` type.

For example, when visa B replaces visa A:

- B remains a separate Item in use; A is retired and shown with a
  **superseded** badge. Other visas stay as they were.
- A's detail panel, in **Retire**, shows **Superseded by: B**, linking to B.
- B's detail panel shows **Replaces: A**, linking back to A. The old record
  must explain why it was set aside, without requiring the reader to find
  the new record first.
- **Undo replacement** removes the relationship and restores A to use. It
  does not delete B or change either Item's snapshots, PDFs or Case entries.
  Replacement and undo are recorded in history.

Replacement is usage metadata, not an expiry date or a content revision.
It therefore reuses retirement's filtering and ordering without rewriting
`expires` or changing a revision ID. This also distinguishes an intentional
replacement from an ordinary retirement with a free-text reason.

The predecessor stores `superseded_by` (the successor's Item ID) together
with `retired: true` in one sidecar write. The reverse link is derived from
that field rather than stored a second time: there is only one relationship
to update or undo. Saving a new Item succeeds before its predecessor is
changed. If linking then fails, the UI reports that the new visa was saved
but replacement failed; the relationship can be applied from details.

A successor can replace only one predecessor. Self-links, already-retired
predecessors and retired successors are refused. A normal retirement edit
cannot bypass an active replacement; use **Undo replacement** first. A
missing related Item is shown by ID, for example in a partial exported tree.
Merge remaps successor IDs when source Items match different destination
IDs and refuses conflicting replacement targets.

### Log

The Log page lists every history event of every Item in the tree, newest
first, as the server orders them (`tree.Log`). It filters by action, type,
Item, text, and frequent Items; picking an entry opens its Item on Browse.
An Item's own History on Browse is the same list for that one Item, and links
to the Log filtered to it.

Expiry is the end of HEAD's `date` when it is a span, read by the server
(`internal/doc/expiry`): **expired** after that day, **expires soon** within
[`doc.expiring_within_days`](../../configuration/doc.md) days (90 by default), **valid**, **no end date** for a span
left open at its end, and nothing for a one-day date or none.

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
The list is a file tree of what extends what: a type others extend is a
folder, opened by picking its row, and one none extends is a file; each row
shows its Item count, or `abstract`. A type's description and what it
extends show beside its name above the text, and on its row's hover.
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
   fields. The Templates are a list, one line each with the type and its
   description, the suggested one marked; past six a filter narrows it, and
   the chosen one always stays in view. Into shows existing Items of that type as cards, alongside New item;
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

## The three layers

```text
Stored object  →  Item  →  Outline
```

- **Stored object** — the bytes of one PDF, identified by its SHA-256.
- **Item** — what the owner says the document is.
- **Outline** — a tree of PDFs made by rules. Each rule selects Items, which
  of their revisions, and the relative path each PDF gets; the rules' paths
  together are the tree. It is browsed as it is, and exported into a folder:
  `dgs doc` writes only to the folder; getting it into iCloud Drive, Google
  Drive or a NAS is a sync the owner does by hand.

The repository's own layout carries none of an Outline's meaning. One
Outline can hold a rule for identity documents and another for bills, each
with its own path, while a second Outline lays out the same identity
documents another way. The repository does not change.

Before Outlines, a View held one rule and named the Target, a folder, it was
exported to. Opening a tree that still has `views/` or `targets.yaml` turns
each Target into an Outline whose rules are the Views naming it, and each
View naming none into an Outline of that one rule; the old files are moved
into `migrated/`. An Outline already saved under one of those names stops
it, with nothing written.

## Items

Every document is an Item with a stable, globally unique ID (UUIDv7 or ULID),
separate from the SHA-256 of its file: the ID is logical identity, the digest
is content identity. There are two kinds.

### Documents: revisions and HEAD

A document is one thing that is reissued over time: a driver licence is
renewed, and the new card is a new **revision** of the same Item. Each revision
keeps its own fields and may have one PDF; front and back are scanned into
that one PDF, never as two files.

Each document has a **HEAD** that points at one revision, like git's HEAD.
Adding a revision moves HEAD to it. HEAD can be moved back by hand, for a
later import that turns out to be a rescan of an older card. What is current is
HEAD, not the highest revision number and not a computation over dates.

Browse links to a Change type page. It maps the current revision into a
Template of the same kind and saves a new snapshot. Previous snapshots keep
their original type and fields. The Item ID, notes and tags remain.

### Records

A record is one independent historical paper: a payslip, a bill, a statement.
It does not accept another paper as a renewal. Editing its details or adding
its scan creates a new snapshot of that same paper, with HEAD identifying the
current snapshot, just as for a document.

### Revision snapshots

A revision is an immutable snapshot of all Template field values, the type,
and the optional PDF. Editing fields, changing type, attaching a PDF or
replacing a PDF creates a new snapshot and moves HEAD to it. Editing an older
snapshot branches from its values; other snapshots remain intact. Saving
unchanged content does nothing. Returning to content already kept reuses that
snapshot and moves HEAD instead of storing a duplicate; that snapshot keeps
its revision tags, and tags given with it are added.

Import asks for both kinds of tags: the Item's, and the revision's own. An
imported Item's first revision starts with the Item's tags, since at first
the Item is that one revision, and the revision tags given are added to
them. A later revision gets only the tags given for it.

The revision ID is SHA-256 of a versioned, canonical JSON payload containing
the type, nonempty fields (keys sorted), and the PDF's SHA-256 or an empty
string. The PDF digest identifies only the attachment. Notes, tags, filenames
and timestamps do not affect the snapshot ID. Metadata edits do not create
revisions. One PDF may be shared by several snapshots with different fields.

Old sidecars remain readable. Before a write changes shared Item fields, old
revisions' effective values are frozen; their existing references stay valid.
Case snapshots keep referencing the exact old version, including its lack of
an attachment. They do not silently follow a later scan. Displayed revision
numbers remain the positions in the revision list; the hash is the identity.

### Details before a scan

Import's **New without PDF** creates the first snapshot from a Template.
Browse shows **No PDF**; fields and expiry work normally. Import also supports
recording a replacement card without a scan.

Only the selected Item on Browse offers **Attach PDF in a new revision** or
**Replace PDF in a new revision**. These copy the chosen snapshot's field
values into a new snapshot with the selected attachment. The earlier snapshot
is kept. PDFs are independently read back before publication. A PDF already
kept by another Item is refused; an existing blob in this Item is verified
before reuse.

Verify checks hashed snapshots against their stored content. It skips
intentional missing attachments but still reports a missing or
changed PDF when a digest is recorded. Outlines and exports skip snapshots
without attachments; a HEAD without a PDF does not fall back to an older scan.
OCR and similarity reading skip these snapshots. Export manifests retain
each snapshot ID and its own field values even when snapshots share a PDF.

Tree merge carries the snapshots and their attachment blobs. A shared snapshot
reference with conflicting content is refused rather than overwritten. A
replacement link to an Item in neither tree is refused rather than dropped.
Deleting any snapshot saves the original sidecar in trash. Its PDF moves
to trash only when no remaining snapshot uses it; shared PDFs remain.

### Telling documents of one type apart

The owner fills in fields that tell an Item apart from others of the same type,
such as `country`: Emma may have a Chinese and an Australian driver licence,
which are two Items. For documents, `owner` + `type` + these fields name one
Item and must be unique; import refuses a second Item with the same ones.

## Templates and Outlines live in the repository

Templates and Outlines describe the documents, so they are kept in the
repository and travel with it when trees are merged. An Outline's folder is
only a default; the folder an export writes to is chosen then.

## Templates

A Template describes one type for import: the fields it fills in by default
and the fields it asks for. Adding a PDF means picking a Template and answering
only what it asks. Nothing is filed automatically: the page may suggest a
Template (see [Suggesting a type](#suggesting-a-type)), and the owner picks.

Every document has a type, an owner and a country. A Template must list
`owner` and `country` (of type `country`) as `required` fields, for records
as for documents; a Template without them is refused when it is loaded or
saved.

A field has a `type`, which checks its value and chooses the control the page
shows for it:

- `text` — the default.
- `date` — `YYYY-MM-DD`. A date field with no `pattern` is suggested from any
  date the text holds (see [Dates in the text](#dates-in-the-text)).
- `month` — `YYYY-MM`, entered with a month control. An expiry month remains
  valid through its last day.
- `select` — one of the field's `options`.
- `item` — the ID of another Item, such as a passport's previous passport. The
  page picks it from the Items. A rule's layout may use the linked Item's
  keys (see below); export never copies the linked PDF.
- `revision` — one revision of another Item, written `<item-id>@<revision>`,
  such as the licence a translation was made from: a later renewal does not
  change what was translated. The page picks the Item, then its revision,
  HEAD at first. A rule's layout may use that revision's keys (see below);
  export never copies the linked PDF.

An `item` or `revision` field may carry `match`: a field of the linked Item
(or `type`) mapped to a field of this form that it must equal. A
translation's `original` has `match: {type: of, owner: owner}`: once `of`
is `driver_licence`, only that owner's licences are offered, and saving an
original that disagrees is refused. A field left empty narrows nothing. A
value starting with `=` is the value itself: `match: {type: =tenancy}`
offers only tenancies. The key `anchor` matches whether the linked Item's
Template is marked `anchor: true`: `match: {anchor: =true}` offers every
Item documents hang under, whatever its type, so a new kind of anchor needs
no change to the Templates linking to it. The page asks the server which
Items to offer.

An `item` or `revision` field may also carry `within`, which suggests the
linked Item by a date:

```yaml
- key: about
  type: item
  match: {anchor: =true}
  within: {date: period, from: start, to: end}
```

`date` is a date or month field of this form; `from` and `to` are fields of
the linked Item. An Item whose span from `from` to `to` holds the date is
suggested — a month is held when it shares a day with the span, and an
empty `to` leaves the span open. The page marks the suggestion, and fills
the field in with it when the field is empty, only one Item is suggested,
and nobody has picked one. Any Item `match` allows can still be chosen.
`dgs doc link` fills the same in for every Item at once.

This is how documents are grouped by a part of life rather than by their
own fields. An anchor is an Item of a Template marked `anchor: true`,
usually without a PDF: a `tenancy` holds an address and the days lived
there, a vehicle its plate and the days it was owned. Anchors name their
span `start` and `end` and their short name `name`, so one `within` and one
`{about.name}` serve every kind. A document's `about` field links it to
one; a rule places a tenancy's leases, bills and receipts with
`rental/{about.address}/{about.start:compact}-{about.end:compact}/`, and
tells the kinds apart with `about.type`. A document is about one anchor; a
date two anchors' spans hold, such as a tenancy's and a car's, suggests
both and leaves the choice.
- `country` — a country however it is typed: its ISO code (`cn`, `CHN`), its
  English or Chinese name (`China`, `中国`) or a common alias
  (`中华人民共和国`, `PRC`), any case. It is kept in the field's `format`:
  `zh` (`中国`, the default), `en` (`China`), `alpha2` (`CN`) or `alpha3`
  (`CHN`). A name dgs does not know is refused. Two documents whose country
  is written differently, `AU` in an older sidecar and `澳大利亚` now, are
  the same document. The table is ISO 3166-1, in `internal/doc/country`.

A field of a document may be `per_revision: true`: Import asks for it again
when recording a renewal. Every snapshot stores all fields, including the
fields inherited from the preceding snapshot. A renewed card has its own number and expiry
and the old card keeps its own. Import asks for every field; adding a revision
asks only for the per_revision ones, suggested from the new PDF's text. On
the same form, the Item's current tags and notes can be edited and are saved
with the new revision; they belong to the Item, not to an individual revision.
On Browse, picking a revision shows its fields; saving edits creates a new
snapshot. Everything else — the page's list, a rule's `if` — uses HEAD's. A
layout key is the exported revision's own value, so exporting all revisions
can name the old card and the new one differently. A distinguishing field
names the document, so it cannot be per_revision, and a record has one issue,
so its fields cannot be either. A value a sidecar keeps at the Item for a key
made per_revision later stands for every revision without its own, until that
revision's fields are saved.

A sidecar may also hold `notes`, free text the owner writes. Notes are never a
key and never exported. The Item sidecar also keeps timestamped history events
for imports, field and metadata edits, HEAD changes, type changes, revision
deletion, PDFs actually written by exports, merges, and marking an Item
frequent or not, and retiring it or putting it back in use. Older sidecars remain readable;
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

## Outlines

Browse filters and lists, but has no levels and no order. An Outline puts
PDFs into a tree, the same tree whether it is looked at or exported. Its
tree is made of two kinds of part, alike in standing: rules, which place
the PDFs they pick as the Items are now, and Snapshots, fixed subtrees put
in as folders.

An Outline is one YAML file under `outlines/`, each rule one under `rules/`
and each Snapshot one under `snapshots/`, each named after its `name`,
written by the Outlines, Rules and Snapshots pages and never edited by hand.
Rules and Snapshots belong to the tree, not to one Outline: an Outline names
those it uses, and several Outlines can use one.

```yaml
# outlines/phone.yaml
name: phone
about: read on the phone          # optional
folder: ~/Library/Mobile Documents/com~apple~CloudDocs/Documents   # optional
rules: [ids, cars]
snapshots:                        # optional
  - name: visa-2026
    at: emma/visa                 # the folder it goes in; the top when empty
    as: 签证                      # optional: the folder's name; the Snapshot's when empty
```

```yaml
# rules/ids.yaml
name: ids
if: type in [id_card, passport] && owner == emma
selection: head
file: '{country}/{owner}/important/{type}.{ext}'
default: none                     # optional: stands in for a missing key
order:                            # optional: what each {#} numbers, in order
  '{owner}': [alex, emma]
  '{country}': [CN, AU]
```

```yaml
# rules/cars.yaml
name: cars
if: type == vehicle_registration
selection: all
file: '{country:alpha2} {make}/{#}-{plate}/{type:zh}.{ext}'
dedupe: number
order:
  '{plate}': [浙AF3897, 浙AT73C7]
```

- `folder` is where it is exported unless another is chosen then. A leading
  `~` is the home folder of the machine exporting, so one file usually fits
  every Mac. Without one, a folder is chosen every time.
- A rule's `name` is unique among the rules; a Snapshot or an Outline may
  share it. An export's manifest records it against every file the rule
  placed.
- Editing a rule changes it in every Outline using it. Renaming one renames
  it in each of them. Removing a rule from an Outline leaves it for the
  others; a rule no Outline uses stays until it is deleted.

### Rules

A rule is a node: an `if`, a path and a file name, and children, which
are nodes too.

```yaml
name: rental
if: owner == alex && tags in [network-5, network-6]
path: 11-Rental/{#}-{about.address}
file: '{name}[-{date:compact}].{ext}'
children:
  - if: category == utility
    path: utility/{category}
    file: '{date:compact}-{issuer}.{ext}'   # in place of the file above
  - if: type == money
    path: rental/{category}
  - if: type == translation                 # no path, file or children: left out
  - path: other                             # no if: the else
```

- **If** — which Items, one condition in package `expr`'s language:
  `&&`, `||`, `!` and brackets; `key == value`, `key != value`,
  `key in [a, b]`, `key ~ text` for a value holding the text, and
  `has(key)` for a key that is filled. `!` binds before `&&`, and `&&`
  before `||`. For a key of several values, such as `tags`, `==` asks
  whether any is the value and `!=` whether none is. A rule without one takes every Item. A value matches ignoring
  case, and a country however it is written: `country == CN` takes an Item
  that keeps `中国`. A value with a space, or one that is a word of the
  language (`in`, `has`), is quoted: `name == "Water Bill"`.
  Keys are those a layout reads (below), linked ones such as `about.type`
  and those `inherit` supplies included, and two more: `tags`, the Item's
  tags and the revision's own, and `status`, where the Item stands:
  `superseded` a visa a later one replaces, `retired` any Item no longer
  used, a superseded one included. `id != <id>` leaves one Item out.
  `type == money` takes every type below money (see Templates); a value
  matches the group its field names it in: `category == utility` takes 水
  when utility holds it.
- **Path and file** — fixed text and keys: `path` is folders, after the
  parent's; `file` is the PDF's name, and may hold folders too. A rule
  needs a file; a child's replaces the one above it. The export's folder
  provides the root, so a layout never names iCloud or a NAS.
- **Children** — an if/elif/else chain: the first whose `if` an Item meets
  takes it, a child with no `if` is the else and comes last, and an Item
  none takes is placed by the node itself. A child with no path, file or
  children leaves what it takes out. `default` passes down like `file`;
  orders serve the whole rule, so a `{#}` written alike in two nodes
  numbers from one order.
- **Selection** — `head` (a document's HEAD revision only) or `all`.
- **Shared** — `shared: true` also takes an Item shared with someone, as if
  they were its owner (see Shared): its `if` is asked with `owner` as each
  person. A rule setting it needs `owner` in its `if`.

The Rules page draws an `if` as bubbles: each comparison a key, an
operator and its values, the key and values offered from the Templates and
the Items, joined by `&&` or `||`, which a click switches, and brackets as
boxes that nest, negated with `!`. A bubble or box is dragged to move it.
A Text switch shows the condition as text for typing, the server marking
what it cannot read and where; text it can read redraws the bubbles. The path and file are typed as text; Edit… opens a dialog of
one row per folder and one for the file's name, each a line of keys, text,
`{#}` and optional parts, added from the key chips and dragged into place.
Children are drawn as nested blocks, as Scratch draws an if: `if`, `else
if` and `else` heads with their conditions, their folders and file, and the
blocks inside them; dragging a block by its head, or ↑/↓, orders them,
and + if and + else add one. A block's folders stay hidden behind + folders
until it has some, as a block that only names its PDFs differently needs a
file alone. Each PDF
the rule cannot place has a Leave out button, which adds `id != <id>`
to the `if`.

Keys are the Item's own fields (`owner`, `type`, `country`, and whatever its
Template defines, such as `employer`), values derived from them (`year` and
`month` from `date`, its start when it is a span; `.start` and `.end` of any
span, such as `{date.end}`), `revision` (the revision's number,
counting from 1, in order added) and `ext`.

A key may name a country format: `{country:alpha3}` writes the Item's country
as `CHN` however its sidecar keeps it, and likewise `:zh` (`中国`), `:en`
(`China`) and `:alpha2` (`CN`). A value that names no country is written as it
is. A date or month may be written `:compact`, `2010-01-01` as `20100101`,
or `:fy`, the Australian financial year it falls in, named by the year it
ends: `2023-08-03` is `FY2024`. A value that is no date is written as it
is. Any other format after the colon is refused.

`{type:zh}` and `{type:en}` write the type's name from its Template's
`names`, such as `驾驶证` or `Driver licence`; `{type}` stays the Template's
file name. A Template that does not name its type in that language leaves the
PDF missing the key `type:zh` or `type:en`, fixed in the Template, not the
Item.

Keys written `{a|b}` are alternatives: the first one an Item has is written,
each with its own format. `{name|type:zh}` writes an ID card's
name, such as `户口首页`, and a driver licence, which has no name, as `驾驶证`.
An Item with none of them is missing the first.

A group in square brackets is optional: it is written only when the Item
has every key in it, and otherwise leaves nothing, its text included.
`[-{degree}]` writes `-本科` for an Item whose `degree` is `本科`, and nothing,
not even the `-`, for one without. An Item lacking a key in a group is still
placed. A group holds text and keys, formats and alternatives included,
`[-{country:alpha3}]` or `[ ({name|type:zh})]`, but no other group. Text in a
layout, in a group or not, may not hold `{ } [ ]`.

A group starting with `/`, at the end of a folder or file name, makes a
folder of its own: `license[/{language}]/{of|type}[-{language}].{ext}` puts a
driver licence at `license/driver_licence.pdf` and its English translation at
`license/en/driver_licence-en.pdf`. Such a group ends a name that has
something before it. It may add several folders, all or none:
`{end:compact}[/bill/{service}]/…` puts a bill in `…/bill/水/` and anything
without a service straight in the folder before. Such a group holds no `{#}`.
`{#}` does not number a folder group: in
`{#}-{level}[/{language}]` the level is numbered and the language folder
follows. `[/{#}-{language}]` numbers the folder itself, from the order named
`{language}`: with that order `en, fr, ru` and `en` numbered 10, the folders
are `10-en`, `11-fr`, `12-ru`. `{#}` goes in no other group.

A layout written before groups, with an optional key in its braces —
`{-degree?}`, `{/language?}`, `{/#-language?}` — is rewritten as the group it
is, `[-{degree}]`, `[/{language}]`, `[/{#}-{language}]`, in its rule's file
when the tree is opened, and its orders renamed to match.

A key written `<field>.<key>` is a key of the Item that an `item` or
`revision` field links to, at the revision it names (an `item` field: its
current one). A translation whose `original` is a bachelor's degree
certificate has `{original.level}` 本科 and `{original.name}` 学位证书. A
translation with no original, or one linking to an Item not in the tree,
lacks the key and is not placed; its field to fill is the link.

A rule may name such fields to inherit from: with `inherit: [original]`, a
key an Item lacks is its original's, and a key it has stays its own. One rule
then places the certificates and their translations together:
`{#}-{level}[/{language}]/{#}-{name}.{ext}` puts the translation at
`02-本科/en/02-学位证书.pdf`, as `{level|original.level}` and
`{name|original.name}` would without it. With several fields named, the first
whose Item has the key gives it. On the Rules page it is a tick per link field
the rule's Templates have.

A folder made only of groups, all left out, would vanish from the path, so
the PDF is not placed instead.

`{#}` numbers a folder or file name: it writes the place, counting from
`01`, of the name the rest of that folder or file name makes, in the rule's
`order` named after that rest as written. The rest leaves out the text
straight after `{#}`, its separator, a folder group ending the name, and a
file's `.{ext}`. So
`{#}-{owner}/{#}-{country:alpha3}` with the order above writes
`01-alex/02-AUS`, and `{#}-{name|type:zh}[-{level}].{ext}`, with an order
named `{name|type:zh}[-{level}]` listing `[身份证, 毕业证书-本科,
毕业证书-硕士]`, writes `02-毕业证书-本科.pdf` and `03-毕业证书-硕士.pdf`:
the name and its level together get one number. A folder or file name has
one `{#}` at most, and a key must follow it.

`{/#}` stops the number: what follows it is written but not numbered, and
the order is named after the rest up to it. `{#}-{name}{/#}[-{signed:compact}].{ext}`,
with an order `{name}` listing `[合同, 物业发票]`, writes `01-合同-20180320.pdf`,
`01-合同-20190322.pdf` and `02-物业发票.pdf`: both contracts share the number
their name has. A name has one `{/#}` at most, after its `{#}`; the page adds
it with the `/#` chip.

A key is no longer numbered on its own. A layout written `{key#}`,
`{key:format#}` or `{a|b#}`, or earlier `{a|b}#`, `{key}#` or
`{key#:format}`, is rewritten as `{#}-{key}` in its rule's file when the tree
is opened, and its order renamed to match, `owner` becoming `{owner}`.

A name may be given its own number, to skip some: `numbers`, per order,
sets it, and the names after it count on from there. With the order
`[身份证, 护照, 结婚证, 户口]` and `numbers: {"{name}": {结婚证: 6}}`, they
are `01`, `02`, `06` and `07`. A number must be larger than the one before
it, and may be `0`. On the Rules page each number in a Numbering list can be typed over;
one set by hand is filled in, and clearing it counts on from the one
before again.

`unnumbered`, per order, lists names left without a number: with
`unnumbered: {"{name}": [押金]}` and the layout `{#}-{name}.{ext}`, the
deposit is `押金.pdf` — neither number nor the text straight after `{#}` —
and takes no number, so the name after it counts on from the one before. A
name is not both unnumbered and in `numbers`. On the Rules page the `#`
button of a Numbering row switches it.

Numbers have two digits, more when the largest is over 99. Names in an
order are compared as a country when both name one, so `CN`, `CHN` and `中国`
are one entry, and otherwise ignoring case. A `{#}` needs an order, and an
order may not list one name twice. A name the order does not list leaves
the PDF missing that order: the number is never guessed. Changing an order
renames every folder after the moved entry at the next export. On the Rules
page, each `{#}` in the layout gets a Numbering list, its names shown with
their numbers and reordered by dragging or with up and down; one new to the
layout starts with the names its Items make, sorted. A name the chosen
Items make that the list lacks is offered below it, to add last.

A layout is rendered by three rules:

- **Missing keys.** Without `default`, a PDF lacking a key is not placed
  (below). With it, the key is written as that text instead.
- **Clean segments.** Each key's value has `/`, `\`, control characters and
  the characters Windows forbids replaced by `_`, and leading or trailing dots
  and spaces trimmed, so a value never adds a folder or escapes the export's
  folder.
- **Duplicates.** Two PDFs of one rule landing on one path are not placed,
  unless the rule sets `dedupe: number`: then, in the Items' ID order, the
  second and later get `_01`, `_02` before the extension, so the same state
  always numbers the same way.

The rules of an Outline are planned together. A PDF two rules select is in
the tree twice, once where each puts it. Two PDFs of different rules
wanting one path, ignoring case, and a PDF wanting a path another needs as a
folder, are not placed either.

### Not placed

A PDF a rule selects but cannot place — lacking a key, or wanting a path
another PDF wants — is gathered under Not placed with the reason, never
dropped. Export refuses to write anything while any PDF is not placed. Only
PDFs a rule selects have to have its keys; the rest of the type is not
checked. `year`, `month` and `date` are filled through the date field they
come from; a key no field of the Item's Template supplies cannot be filled,
only named — the layout or the Template has to change.

### The pages

Making the rules and looking through the result are kept apart:

- **Rules** makes a rule. Its name, which names the Outlines using it —
  saving changes it there too — then a form: the types, conditions on
  other keys (`owner is emma, tom`), each ticking values the Items of the
  chosen types hold, HEAD or all revisions, and the layout, typed or built
  by clicking key chips that insert at the caret. The field chips are those
  of the chosen types, or of every type when none is, grouped: the
  Templates' fields, the keys every PDF has, and each country field's
  formats. Typing `{` in the layout lists the keys with what each writes;
  after a country key's `:` it lists the formats with an example. Beside
  the form the rule's tree is redrawn as it changes, with the PDFs not
  placed above it: a PDF lacking a field gets a form to fill it, on one PDF
  or on several at once; a numbered value the order lacks can be numbered
  last. **Snapshot…** takes a Snapshot of what it places now. Saving writes
  `rules/<name>.yaml`; renaming renames it in every Outline. A rule an
  Outline uses cannot be deleted.
- **Snapshots** takes and edits Snapshots (below).
- **Outlines** puts them together: the Outline's name, what it is for and
  its own folder, then the rules it uses, ticked, and its Snapshots, each
  with the folder it goes in. Beside the form the Outline's tree is redrawn
  as it changes, each Snapshot a folder of its name, with what it cannot
  place above it, each linked to the rule or Snapshot to fix. Saving writes
  `outlines/<name>.yaml`; renaming removes the old file.
- **Explore** shows the result, browsed as in a file manager. It lists the
  saved Outlines, a list that folds away, and draws the chosen one's tree
  down to each PDF's name, every folder counting the PDFs beneath it.
  Picking a folder lists its PDFs, and those beneath it under their folder,
  each linked to its Item on Browse; picking a PDF opens its pages beside
  the tree, with Show in Finder, which selects the tree's own file, and
  Open in Browse. Not placed is a row of the tree. **Export…** writes it
  (below).
  Nothing on it changes an Outline; Edit rules opens it on the Outlines page.

### Snapshots

A Snapshot is a fixed subtree of PDFs: what was handed in for a visa or a
claim. The rules and the Items change after; the Snapshot does not. It
stands beside the rules: an Outline puts it in its tree as it puts a
rule's PDFs.

```yaml
# snapshots/visa-2026.yaml
name: visa-2026
about: handed in for the visa     # optional
taken: 2026-09-26T10:00:00+10:00
rule: ids                         # what it was taken from; absent when begun empty
files:
  - path: CN/passport.pdf         # inside the Snapshot's folder
    item: 01M3CQZ4J29EC3KMWDG5DGB6R3
    revision: 01M3CR0A…           # the revision's ID
    rule: ids                     # the rule that placed it, for a reader
folders:                          # optional: folders with no PDF in them
  - CN/old
naming: "{owner}-{type}.{ext}"    # optional: how a PDF added by hand is named
```

- On the Snapshots page one is taken from a rule — what it places now, at
  the paths it gives them — or begun empty. A rule with PDFs it cannot
  place is refused: place them first.
- It is edited by hand after, in its tree: a folder made in the folder
  picked, PDFs added there several at once from a list narrowed by words
  matching any field, an owner, a country, a type, a tag, one field's
  value (`fy` 2023),
  whether it is in the Snapshot already and whether retired, the filters
  remembered in the browser, in a dialog listing them in columns as in
  Finder: Name and the keys chosen by a right click on the header or its
  +, each dragged by its edge to size it and by its header to move it, a
  click on a header sorting by it, all remembered (each its
  Item's latest revision, named after the Item), a PDF or folder dragged onto another
  folder or to the top, renamed, a PDF set to another revision of its Item,
  or either taken out — a folder with the PDFs in it. A folder a PDF leaves
  stays. The tree works as Finder's: a right click on a row
  offers New Folder, Add PDFs, Rename, Show in Finder and Remove, and on
  the space around the rows New Folder and Add PDFs at the top; a click
  there picks nothing; a name is typed in its row; Enter renames the row
  picked, Cmd-Delete removes it, Shift-Cmd-N makes a folder. Cmd-click
  adds a row to what is picked or takes it away, Shift-click picks every
  row from the last one picked; dragging one of them moves them all, and
  the menu and Cmd-Delete act on them all, asked first. Escape keeps only
  the last one picked. The filters
  fold away. Nothing is copied, as an archived Case copies nothing.
- A PDF added by hand is named by `naming`, a layout written as a rule's,
  relative to the folder it is added to; a `/` in it makes folders, and
  `{#}` is refused, since one PDF numbers nothing. Beside the Items, Add PDFs
  shows the ticked ones as the tree they would make, named as they would
  be, redrawn as the ticks or the layout change. Without `naming` it is
  named by its Item's label. `.pdf` is added when the name lacks it, and a
  name already there gets `_02`, `_03`. An Item lacking a key the layout
  needs is refused with the keys it lacks, and nothing is added. Changing
  `naming` renames no PDF already in the Snapshot by itself: Rename by
  Naming, on a PDF, a folder or several picked, renames those PDFs and the
  PDFs in those folders, each in the folder it is in.
- A folder with no PDF in it is kept under `folders`; one holding a PDF is
  not written. An Outline and an export see only the folders its PDFs are
  in: an empty folder is not exported.
- An Outline puts it in its tree as the folder `<at>/<name>`, or
  `<at>/<as>` when the Outline gives it a name there: one name, no `/`. One of its
  paths wanted by a rule's PDF too, or one needed as a folder, is a clash,
  and the export fails, as between two rules.
- Its name is unique among the Snapshots; a rule or an Outline may share
  it. The manifest records its files under it, so a rule and a Snapshot of
  one name share what an export wrote for them. Renaming one renames it in
  every Outline.
- A revision that leaves the tree, or an Item deleted, is listed as lost,
  never replaced by a newer one. An Outline with a lost file is not
  exported, as one with a PDF not placed is not.
- One an Outline uses cannot be deleted. Deleting moves its file into
  `trash/snapshots/`. Its Items stay.
- A Snapshot taken before Snapshots stood alone keeps the Outline and folder
  it was taken from; nothing reads them. Its old folder no longer exports
  it: an Outline does.

## Export

Exports are deterministic: the same repository state and Outline produce the same
tree. Before writing, an export computes the whole plan — add, replace,
remove — and can show it without writing (dry run).

A file is published under the same contract as
[`../photo/import.md`](../photo/import.md): written under a `.dgs-part` name,
read back independently, and renamed to its final name only when the readback
matches its SHA-256.

Export removes only files it wrote itself, recorded in a manifest at the
root of the folder; anything else under it is never touched.

An Outline is exported as a whole: every rule and Snapshot, planned
together, into the folder chosen for it — this browser's choice, else its
own folder. A run is one Outline, several, or every Outline with rules or
Snapshots and a folder, and it is
checked entirely before anything is written. Any of these stops the whole
run, and no Outline is written:

- a PDF not placed, by any rule, and a Snapshot's PDF the tree has lost;
- a wanted path held by a file the export does not own (below), including one
  another Outline exported into the same folder;
- a folder inside the tree or holding it, two folders of the run
  overlapping, an Outline with no rule and no Snapshot, or no folder, a folder inside another
  tree, and a manifest that cannot be read.

The check lists every one of them, by rule and path; the export itself plans
and checks again from the state at that moment.

On Explore, **Export…** opens the shared file dialog on the folder the
Outline last went to from that browser, else its own; the dialog can make a new folder there. Choosing one checks the plan:
what stops it is listed beside the tree and nothing is written; a plan that
replaces or removes a file asks first; otherwise it writes. `dgs doc export [<outline>...]` does the same from the
command line, every Outline with rules and a folder when none is named;
`--to phone=/Volumes/Kindle/documents` chooses a folder, comma separated for
several; `-n` checks and writes nothing. It fails, writing nothing, when the
check finds anything.

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
rule that placed it (under the key `view`, its name from before Outlines), the Item's ID and revision, and the Item's fields at
export time. An export replaces and removes only the files of the rules it
exports. A version 1 manifest, from when a folder took one View, is read
with its files belonging to that one rule. That is enough
to import an exported folder back into a repository: `dgs doc` reads the manifest and
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
  is planned and checked as an Outline's export is and writes `dgs-export.json`,
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

`dgs doc link [<tree>]` fills every empty link field whose `within`
suggests exactly one Item, such as the tenancy a bill is about, by its
billing period.
It lists each link, and those where several Items fit, and writes nothing
unless given `--apply`; each link is a new snapshot, as an edit on the page
is. A link where several fit is left to be chosen on the page.

`dgs doc verify [<tree>]` (`-q` for the result only) checks the repository against its sidecars and
changes nothing. It reports:

- a revision whose PDF is missing, or whose content no longer hashes to its
  name;
- a PDF under `items/` no sidecar names;
- a sidecar that does not parse, names an unknown Template, or whose HEAD is
  not one of its revisions.
- a file under `rules/`, `outlines/`, `snapshots/` or `cases/` that does not
  read on its own: it does not parse, is not valid, or is not named for what
  it holds; an Outline naming a rule whose file does not read is among them;
- such a file naming what the tree lacks: an Outline a Snapshot, a Snapshot an
  Item or one of its revisions, a Case an Item.

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
and Outlines — and each conflict with both sides, to keep this tree's or take
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
- A Template or Outline both sides have, different, is a conflict. So is a
  new Outline using a rule this tree has differently: taking it changes the
  rule for every Outline here using it. A tree still holding Views and
  Targets is read as the Outlines they become. Only the
  fields that differ are shown.
- What no choice resolves stops the merge: an Item whose type has no Template
  on either side, two Items matching one, a record holding another PDF here.
- PDFs are copied in with the same readback as import, before the sidecar
  naming them is written.

An exported folder imports back the same way: the Merge page opens a folder with a
`dgs-export.json` as it opens a sub-tree. It brings the revisions that were
exported and the Items' fields; it carries no Templates or Outlines.

## Layout on disk

```text
<tree>/
  dgs-doctree.yaml          # the marker: dgs doc init writes it, nothing else does
  templates/
    id_card.yaml            # one Template per type
  outlines/                 # one Outline per file, naming its rules
  rules/                    # one rule per file, shared by Outlines
  snapshots/                # one Snapshot per file
  cases/                    # one Case per file
  migrated/                 # views/ and targets.yaml, once they became Outlines
  trash/                    # deleted Items and Templates, never erased
  items/
    <item-id>/
      item.dgs-item.yaml    # the sidecar: everything known about the Item
      <sha256>.pdf          # optional PDF per revision, named by its content
```

- The marker is `dgs-doctree.yaml`, not `dgs-doc.yaml`, which reads too much
  like box's `*.dgs-doc.yaml` sidecars. It is visible, as box's is.
- An Item's folder is named by its ID, so changing `owner` or `country` moves
  nothing. Readable paths are what an Outline exports.
- A PDF is named by its SHA-256, so the same file is stored once and merging
  matches by digest.
- An export writes `dgs-export.json` in the folder it writes to (M6).
- PDFs anywhere else in the tree are loose, and allowed. Import copies a PDF in
  from wherever it is and leaves the original where it was.

## Template format

```yaml
type: id_card
description: Identity cards and household registers
names: {zh: 身份证明, en: ID card}   # optional: {type:zh} and {type:en} in a layout
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
    description: The country that issued it  # shown beside the field's name
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

### Types that inherit

Types form a tree, as classes do: a field is defined once, in the type where
its meaning starts, and the types below inherit it.

```yaml
# templates/record.yaml
type: record
abstract: true            # no Item is of this type; it only passes fields down
fields:
  - {key: owner, type: select, options: [alex, emma], required: true}
  - {key: country, type: country, required: true}
  - {key: name}
  - {key: date, type: date, shape: [day, span]}

# templates/money.yaml
type: money
extends: record
abstract: true
fields:
  - key: category
    type: select
    values: {utility: [水, 电, 气, 网], housing: [房租, 物业, 车位]}

# templates/bill.yaml
type: bill
extends: money
kind: record
fields:
  - {key: date, shape: span}
  - {key: due, type: date}
```

- `extends` names one parent. A type inherits its fields, in the parent's
  order and before its own, their `required` and `distinguishing`, and its
  `defaults` and `ignore_dates`. It does not inherit `names`,
  `description`, `kind` or `anchor`.
- An inherited field may be listed again to make it required,
  distinguishing or `per_revision`, narrow its shape, or give it its own description and
  patterns. Anything else — another type, options, suggest — is refused.
- An abstract type has no `kind` and no Item; Import does not offer it, and
  the Templates page draws it as a folder with the types extending it. Every other type states its
  `kind`.
- The tree is not fixed: changing `extends` changes only what a type
  inherits and what `type == <parent>` takes. A type another extends cannot
  be renamed or deleted first.
- A select field's `names` give its values in `en` or `zh`, as
  `names: {en: {房租: rent, 电: power}}`; a layout writes one with
  `{category:en}`, each of several values in turn, and a value without a
  name as it is kept. `if` still compares the kept value. Names are
  inherited with the field and cannot be changed below it.
- A `date` field's `shape` is `day` (the default), `span`, or both. A span
  is kept as `2025-01-01/2025-12-31`; an end not known is left empty,
  `2025-01-01/` or `/2030-03-01`, and an empty end is open. On the page a span is two date boxes; one that also takes a day
  keeps one day when the end is left empty.
- A `select` field may write `values`, its options in named groups, in
  place of `options`. A condition on a group takes each value in it. The
  groups are written only where the field is defined.

The file is named after its `type`. `description` is a short explanation shown
on the Import card and in the Templates list; older Templates may omit it.
A `select` field lists its allowed values under `options`. With `multiple: true` it
holds several, kept in the options' order joined by `, ` (`电, 水`), each
toggled on the page; its options may not hold a comma. A rule writes the
joined value, and a condition asks for one of them with `is` (`service is
水`). A text field with `suggest: true` offers, as it is typed,
the values Items of the same type have saved for it, in any country and
revision, retired Items among them; any other value can still be typed. A field's `description` says
what it holds; every form shows a `?` beside the field's name, which shows it
on hover and keeps it shown under the name when clicked. A field's `pattern` is a regular
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
fields: {owner: emma, country: AU}
head: <snapshot-sha256>
revisions:
  - id: <snapshot-sha256>
    snapshot: true
    type: id_card
    fields: {owner: emma, country: AU}
    digest: <pdf-sha256>   # empty when there is no attachment
    added: 2026-09-25T10:00:00+10:00
    source: scan.pdf
```

Legacy records have one revision and no `head`; snapshot records also use HEAD. `notes` and `tags`, when the
owner has written any, are Item keys in the sidecar. Each revision, with or without a PDF,
may also have `tags` of its own — a reissue, a copy — kept on that revision;
the tags a revision has are the Item's and its own. Browse filters and
searches an Item by the tags its HEAD has, edits the Item's tags and the
picked revision's apart, and shows each revision's own beside it. Item tags and revision tags can both be added, edited or cleared without changing snapshot content. Changing a revision's tags
is a history event. Tags are lower case, with
spaces joined by hyphens, and duplicates removed. Import and Browse offer
existing tags as the owner types; Browse filters by tags in the current tree.

## Later

- **Archive serial numbers and barcode separator pages**, if box wants them:
  they concern paper, not filed documents.

- **Clone** — a sub-tree cloned from the full tree would remember the state it
  was cloned from, so its merge could compare three sides, as git does.

Its settings — `doc.root`, `doc.web.port` — are in
[`configuration/doc.md`](../../configuration/doc.md).
