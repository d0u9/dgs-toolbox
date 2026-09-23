# Box: import

`dgs box import` takes new scans from a temporary folder into a Box. It is the
only `box` command that performs real file operations, and it follows the
integrity contract in [`index.md`](index.md#the-integrity-contract).

Read [`index.md`](index.md) first for the layout and the sidecar. The type
catalogue is [`types.md`](types.md).

## The one requirement everything else serves

**The inbox must always be drainable to empty.**

Sorting one at a time stops at the first scan that needs thinking about, and
everything behind it stays in the temporary folder, which then becomes a second
archive nobody trusts. So "I do not know what this is" is a **legal outcome**,
not a blocked one: the scan is filed as `unsorted` with one keystroke and
decided later in [`view`](view.md), where there is time.

Every deferral in the first version works this way. A PDF holding five
unrelated receipts can be [split](#splitting-one-pdf) on the spot, or marked
`needs_split` and filed when that is not worth doing now. A page whose image cannot
be extracted is marked `needs-render` and filed without a thumbnail. Nothing
waits in the inbox for a feature.

## Where the work happens

Two surfaces, split by what each is good at.

- **The TUI** chooses the inbox and the Box root, runs the duplicate pass,
  shows progress, and starts the local server. Picking paths and watching a
  long job are what a terminal does well.
- **A served page** shows the scans and takes the metadata. A 200 dpi
  monochrome receipt has to be legible enough to read a date and a total off,
  which rules out the terminal even with image support.

The server binds a loopback address and the first version has no remote access
at all. A Box holds identity documents and medical records; opening it to the
network is a separate design with its own access control, not a configuration
key.

## Before anything is shown

Three passes run first, so the owner's attention is spent only on scans that
need a decision.

### Completeness

Every candidate must end with `%%EOF` and must parse far enough to yield a page
count. Both checks are free, because the page count is wanted anyway, and
between them they catch a truncated file, a bad copy and a damaged old backup.

A file failing either is listed as `incomplete` and not taken in. It is never
silently skipped: a skip that is not reported means believing the inbox is
empty when it is not. The page says so in its own area beside the inbox — what
was held back and why — rather than leaving it to a log nobody opens.

There is deliberately no wait for the modification time to settle. Scanning
finishes before the tool is run, so a partially written file is not a case that
occurs here, and the two content checks would catch it anyway.

### Deduplication

Two digests per file, both compared against the whole Box — including
[`trash/`](index.md#deleting).

- **The whole file**, SHA-256. Catches rescans and restored backups.
- **The embedded image streams alone**, ignoring the PDF container. Catches the
  same scan rewrapped by different software: merged, re-saved, passed through a
  mail client, stripped of metadata. The container differs, the scan does not.
  Whole-file hashing misses every one of these, and for scanned material they
  are the common case.

Duplicates are collapsed into one group and dealt with in a single action
rather than presented one at a time. A match against the trash says when that
file was thrown away, so the same judgement is not made twice. On the page that
is one line — how many of the waiting scans the Box already holds, and how many
of those are in its trash — and one button that rejects all of them.

This pass is worth having on its own and is also `dgs box dedupe`: it needs no
input, cannot classify anything wrongly, and reduces the pile immediately. It
is the right first thing to run against a folder that has never been through
the tool.

### Metadata and thumbnails, in one read

Hashing reads every byte of the file over the network, so everything derivable
comes out of that same pass: both digests, the page count, the page size, the
PDF's `Producer` and `CreationDate`, the embedded image, and both thumbnail
sizes. Coming back for any of it later would mean fetching the same file from
the NAS again.

Images — the remaining 1% — go down the same path with EXIF in place of PDF
metadata, and keep `kind: image`. They are not converted to PDF: a conversion
rewrites the bytes and loses the original.

What this yields without a single word being typed is worth stating, because it
is the whole automatic half of the tool:

- **`Producer`** names the scanner or software, which in practice separates a
  Box into eras. Changing scanners once partitions thousands of files for free.
- **`CreationDate`** is when it was scanned, and is more reliable than the
  file's modification time, which copying and syncing have overwritten.
- **Page count and page size** are a usable prior: one A4 page is a document,
  five A4 pages are a contract or a policy, something small is a stub.
- **Resolution and colour mode** record a judgement already made. Monochrome at
  200 dpi went through the feeder in a batch; colour at 600 dpi was placed on
  the glass deliberately.

Together these order the inbox from probably-important to probably-a-stub
without a line of classification logic. Ordering is what is actually needed —
only the top of the list deserves care.

## Order

Scans are presented in **scan-time order**, from `CreationDate`, not in
filename order. What was scanned consecutively is related: the flight, the
boarding pass and the hotel receipt sit next to each other. That adjacency is
what makes assigning a type to a range of scans at once correct rather than a
gamble, and filename order destroys it.

Selecting a contiguous range and giving it one type is a first-class action. A
few hundred scans at a few seconds each is an hour of uninterrupted work that
nobody finishes, so batch assignment is not a convenience — it is the only way
the tool gets used twice. Individual attention is for the handful that are
worth it.

## The page takes five fields

Deliberately four. Every further field multiplies by the number of scans and is
paid for in sessions that are never finished.

| Field | |
| --- | --- |
| Type | one keystroke, from [`types.md`](types.md) |
| Event date | the date on the document, with its zone |
| Description | one line, free text |
| Total | optional, skippable |
| Tags | comma-separated keywords; shared across a selected run |

Everything else — the other party, the expiry, grouping beyond one key — is
filled in [`view`](view.md), on the few scans that need it. Splitting is the
exception: it is done here, while the pages are on screen.

**The page is operated entirely from the keyboard.** No action requires the
mouse. One scan or one batch fills the screen, a single key sets the type, two
fields take the date and the description, optional tags classify across types,
and `Enter` moves on. It is a
terminal interface that happens to be rendered in a browser, because this is
several hundred repetitions and a hand leaving the keyboard is a real cost. The
[browsing page](view.md) is the opposite and uses the mouse normally.

Three keys exist beyond the fields:

- **`keep`** clears the expiry, making the scan permanent. This is how a
  concert ticket worth keeping escapes `ticket`'s same-day default without
  needing a type of its own.
- **`same as previous`** puts this scan in the same `group` as the one before
  it, for a document scanned in several passes. It costs one keystroke while the
  parts are on screen together and is close to unreconstructable later, which
  is why the field is collected in the first version even though the group
  browser is not built.
- **`reject`** turns the scan away with a reason, for a failed scan or
  something not wanted. Nothing is in the Box yet, so nothing is moved or
  deleted: the file stays in the inbox and only its record in the state
  becomes `rejected`. The inbox column has an Inbox tab and a Rejected tab; Rejected lists what was turned away, and
  **Restore** puts one back in the list to be decided again — a Backspace
  pressed by mistake is one click to undo.

### The zone is searched, not typed

An IANA name is not something anyone types from memory. A few letters in the
zone field list the zones they could mean — by city (`syd`), by country
(`aus` is every zone in Australia, and Austria's), by region or by country
code — and `↑` `↓` and `Enter` pick one. What is typed is a search; only a
picked zone, or a whole name left in the field, is kept. The catalogue is
tzdb's own `zone.tab` and `iso3166.tab`, compiled in, and the search is
[`internal/zonesearch`](../../../internal/zonesearch/zonesearch.go).

### Looking at every page

A PDF shows its first page. A page view — open unless the person has closed
it with `p`, and kept open or closed across scans — puts a strip of one thumbnail per page beside it; `←` and
`→` turn the page drawn large. Pages are drawn when the strip asks for them,
not during the read of the inbox: a fifty-page document is fifty pictures
nobody may look at. A page past the first of a scan not yet filed is drawn
from the inbox file each time and never cached, because the cache belongs to
the Box and a rejected scan must leave nothing of itself behind.

The inbox column is as wide as it is dragged, from 220 px up to half the
window, and remembered per browser. Its rows are set at 14 px on 1.5 — the
design language's `caption-md` — because they are read for a whole sitting.

### Splitting one PDF

A PDF often holds several documents: a bank statement, then the letter that
came with it, then two receipts. The file is **never cut**. The sidecar records
which pages are which document, so the bytes and the digest stay what arrived,
and a split made wrongly is corrected by editing text rather than by digging a
file out of the trash.

```yaml
type: unsorted
pages: 10
documents:
  - pages: 1-3
    type: statement
    description: "bank, June"
    event_date: 2024-06-30
    event_tz: Australia/Sydney
  - pages: 2-5
    type: receipt
    total_minor: 1250
    currency: AUD
  - pages: "6"
    type: medical
    tags: [dentist]
ignored_pages: 7-10
```

Ranges may overlap. A covering letter that belongs to two documents is a real
case, and a page in two ranges is simply in both.

A page in no document is either **ignored** — a blank, a separator sheet — or
forgotten. `ignored_pages` says which, so a forgotten page is a warning rather
than something that silently drops out of every filter. A page cannot be both
ignored and in a document.

A PDF is often a pile scanned together in no particular order, so **each
split is described as a PDF of its own**: the same type keys and the same
fields the desk always has — event date, zone, description, total, tags. A
line above the desk says which split the fields are describing, as
`Split 2 of 3 · p 4-5`. Once a scan has splits, the file itself is filed as
`unsorted` with nothing else said about it; nobody cares what the whole pile
is. A dated split with no zone of its own is read in `box.zone`.

**Inheritance only adds.** The sidecar still lets a file carry fields every
document shares, for a sidecar edited by hand. A document may set a field the
file leaves empty — type, event date, zone, description, total — and it may
add tags, but it cannot replace or remove anything the file says: two answers
to one question in one sidecar is the metadata nobody can trust. The file's
type counts as unset while it is `unsorted`, which is what unsorted means.

Recording a split clears `needs_split`: saying where the documents are is what
it was waiting for.

There is no separate mode. On a PDF of more than one page:

| Key | |
| --- | --- |
| `←` `→` | turn the page; the desk follows to the split that page is in |
| `Space` | mark where a split starts; again where it ends, and turn to the next page |
| `↑` `↓` while marking | move along the pages — the strip runs top to bottom — instead of to another scan |
| `x` | ignore the page, or the marked range |
| `:` | type every split at once — `1-3, 2-5, 6, !7-10`, where `!` ignores |
| `-` | remove the split on the desk, also a button beside it; its pages are not touched |
| a type key, the fields | describe the split on the desk |
| `Enter` | file the scan with all its splits |
| `Esc` | let go of a mark, then of everything unsaved |

While a split is being marked, a bar across the page says so — which pages,
Space to end, Esc to cancel — and the line above the desk turns solid. A mode
entered by one keystroke must never be one nobody can see they are in.

Every page in the strip is framed in its split's colour and shows the numbers
of the splits it belongs to; the pages of the split on the desk are outlined
as well. The numbers above the desk pick one. Blue is kept for the mark being
made — tinted pages with a tick — so no split is drawn in it. `Enter` with pages in no split stops once and
names them; a second `Enter` ignores them and files. The split is filed with
the scan, like every other field. Nothing is written into the Box before
that; until then the split is a [draft](#resuming).

`keep` and `same as previous` still belong to the file as a whole.

Exporting one document as a PDF of its own is not built. When it is, the result
goes outside the Box: it is a copy made on request, not a second truth.

### The amount is one field

Typing `123.50` uses the configured default currency; typing `USD 45`
overrides it. Two separate inputs would add a keystroke to every scan that has
a total.

Filling it at intake is optional on purpose: reading a total off a scan means
deciding whether it includes tax, which is slower than everything else on the
page. `view` fills them in afterwards in a pass of its own, over exactly the
scans that need one.

### `reviewed` records that this was a guess

Intake is fast, which means types are guessed. `reviewed: false` marks a guess
and is set true only when a human confirms the type in `view`.

Without this field guesses and confirmations are indistinguishable and the
whole metadata set is equally untrustworthy. With it, "the three hundred I
guessed at in September" is a filter.

## Publication

For each accepted scan:

1. Copy into the intake date's directory under a `.dgs-part` name in that same
   directory, hashing the source bytes during the copy.
2. Read the destination copy back independently and hash it.
3. Only if the digests match, link it to its final name, which fails rather
   than replace a file that appeared in the meantime.
4. Write the sidecar.
5. Append one line to `dgs-box-log.jsonl` at the Box root.

A failure at any step retries the whole file rather than resuming part of it. A
partial copy is never published and never left under a name that looks final.

The source file in the inbox is **not** deleted. Whether to empty the temporary
folder is a decision for its owner, made once the Box is known to hold the
files; `box` does not make it.

## Resuming

A hundred scans are not sorted in one sitting. State lives in
`box.state_file` inside the inbox, versioned, holding one record per candidate:

```text
pending | classified | published | duplicate | incomplete | rejected
```

Closing the tool and coming back continues where it stopped. What has been
typed about a `classified` scan — its fields, its splits — is saved as a draft
half a second after the last keystroke, and again as the tab closes, so
reloading the page halfway through a long PDF loses nothing. The line under
the progress says whether the draft is saved. A draft lives only in the
inbox's state, never in the Box: nothing there is filed yet. The same pattern,
and the same reasoning, as Photo Import's
[`.dgs-state`](../photo/import.md).

Work happens through bounded workers — `box.workers` — because both ends are
usually network filesystems where a handful of concurrent whole-file reads is
faster than one, and unbounded reads are slower than both. Reading the inbox is
what this buys most: every candidate that is not already settled is a whole
file pulled across, and that is the wait at the start of a sitting.

Filing is still one scan at a time. It copies bytes, each copy is verified
against its own digest before it is published, and a failure has to stop that
scan rather than the two hundred behind it.

## The existing archive

Folders sorted by hand before there was a tool need no special path: point
`import` at one as the inbox. Deduplication runs, metadata and thumbnails are
built, and everything is filed as `unsorted`. The result is a Box where
everything is visible and the duplicates are gone, with the describing left to
be done gradually in `view`.

There is no migration mode, and that is the point: it is the same code path,
so there is nothing extra to write, test or trust.
