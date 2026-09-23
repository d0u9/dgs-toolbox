# Box Design

## Scope

This document owns the decisions shared by every `dgs box` command: what a Box
is, how it is laid out on disk, where metadata lives, and what the discardable
index is allowed to hold. The two workspaces have their own documents —
[`import.md`](import.md) for taking new scans in, [`view.md`](view.md) for
browsing and correcting what is already in. The type catalogue is
[`types.md`](types.md). Its configuration keys are in
[`../../configuration/box.md`](../../configuration/box.md).

Read [`../../tui.md`](../../tui.md) before changing the TUI parts and
[`../../web.md`](../../web.md) before changing the served pages.

## What a Box is for

A Box holds scans of paper: cinema tickets, boarding passes, insurance
policies, invitations, letters, leaflets picked up while travelling. Roughly
99% are PDFs produced by a scanner; the rest are images. They arrive in a
temporary folder, are sorted once, and are then kept — most of them forever,
some of them only until they expire.

Two properties of this material drive the whole design.

**The scans carry no text layer.** A scanner produces images wrapped in a PDF.
Without OCR — which `dgs` does not do, and which no network service may be
asked to do — there is nothing to index, search or classify automatically.
Every word in a Box is a word the owner typed. The tool's job is therefore not
to understand the documents but to make describing them fast, and to make
everything it can derive without reading text free.

**The material is of two natures.** Some scans are kept because they may be
needed: receipts, policies, statements. They have an end of usefulness. Others
are kept because the owner wants them: letters, leaflets, an advertisement
worth keeping. These never expire, have no amount, and asking for those fields
would only slow sorting down. The type catalogue is grouped along this line and
the default lifetimes follow from it.

## The integrity contract

`box` publishes files under the same contract as
[`../photo/import.md`](../photo/import.md): a destination file is not published
under its final name until bytes read back independently from the destination
produce the same SHA-256 digest as the bytes read from the source. The digest
is recorded in the file's sidecar and is what identifies the file from then on.

As in Photo Import this describes integrity at publication time only. It cannot
promise the bytes stay intact afterwards; that is what
[`view.md`](view.md#verify) covers. Nothing here promises more than the
user-space filesystem API gives, which matters more than usual because a Box
normally lives on a NAS over SMB or AFP, where flushing semantics are weaker
than on a local disk.

## Layout

A Box is one directory tree. It is the only thing that has to be backed up, and
it is complete on its own: the metadata is in it, and every cache can be thrown
away and rebuilt from it.

```text
<root>/
  dgs-box.yaml                   the marker proving this is a Box
  dgs-box-log.jsonl              every intake and every edit, appended
  2026/
    2026-09-22/                  the day the file was taken in
      scan-0012-a1b2c3d4.pdf
      a1b2c3d4.dgs-doc.yaml
  trash/
    2026-09-22/                  the day the file was thrown away
      scan-0044-77f0e1b9.pdf
      77f0e1b9.dgs-doc.yaml
```

### Paths encode only what cannot change

The directory a file lands in is chosen from its **intake date** and never
changes again. Type, subject, amount and expiry are all things the owner will
correct later — today's `receipt` is next April's `invoice` — and a tree that
encoded any of them would turn every correction into a file move, which means a
copy, a readback, a broken external reference and a re-upload to the NAS. With
this layout a reclassification is one sidecar rewrite and one log line. The
file never moves and its digest stays valid forever.

Bucketing by intake date rather than by the date on the document is the same
decision seen from the other side: the intake date is a fact about this Box, is
known exactly, and is never revised. It also groups a batch usefully, because
what was scanned on one day is usually related.

`unsorted` is a state in the sidecar, not a place. There is no `unsorted/`
directory, because a file whose type is not yet known is otherwise a normal
member of the Box and moving it once decided would defeat the paragraph above.

### Filenames

A published file keeps the stem it arrived with and gains the first 8 hex
digits of its digest: `scan-0012-a1b2c3d4.pdf`. Keeping the original stem first
means the directory still sorts the way the scanner named things; the digest
makes the name unique and lets a file be matched by eye against a report. When
8 digits would collide inside one directory the prefix is extended until it does
not.

The sidecar is named from the digest alone — `a1b2c3d4.dgs-doc.yaml` — and so is
always plain ASCII. This is deliberate. macOS hands out filenames in NFD while
software usually produces NFC, so `café.pdf` can exist as two byte sequences
that look identical, and a NAS exporting a byte-exact filesystem will happily
keep both. Pairing on a digest instead of on a filename removes that failure
from the part of the tool where it would do real damage: a sidecar that cannot
be found is reported as an orphan, and an orphan report nobody trusts is worse
than none. Normalisation then only matters for display and comparison, where
the rule is to normalise both sides to NFC and to keep the bytes as they are on
disk. The stem as it arrived is recorded in the sidecar as well.

### Nothing a person needs is hidden

None of the files `box` writes into a Box or an inbox begins with a dot:
`dgs-box.yaml`, `dgs-box-log.jsonl`, the sidecars, and `dgs-box-state.json` in
the inbox. Finder hides dotfiles, and a Box whose metadata is invisible looks
like a folder of PDFs with nothing beside them — which is the opposite of the
point, since the sidecar is meant to outlive the tool.

The one exception is `.dgs-part`, the temporary name a file is copied under
before it is verified. It exists for seconds, it is not something anyone should
open, and Photo Import already writes that name for the same mechanism.

### The marker file

Every command refuses to write anything unless `dgs-box.yaml` exists at the
root, and never creates it as a side effect — `dgs box init` is the only thing
that writes it. Writing that file is the whole of `init`, so it opens no
workspace: it is one of the four [CLI actions](#first-version),
`dgs box init [<dir>]`, and a keystroke in front of it would stand between a
script and the only command that can create the Box every other command
requires.

On macOS, writing to a path under an unmounted `/Volumes/...` mount point
silently creates a local directory instead, so without this check a missing NAS
produces a second, empty, plausible-looking Box on the local disk and nothing
says so for weeks.

The file holds the Box's format version and when it was created, rather than
being empty. An empty marker can only answer whether this is a Box; one with
contents can also answer which version made it, which is what a later format
change needs.

## Metadata: the sidecar is the truth

One YAML file per scan, beside it, holding everything the owner has said about
it and everything derived from its bytes.

```yaml
version: 1
digest: sha256:a1b2c3d4…
size: 4812390
kind: pdf                       # pdf | image
original_filename: "Scan_0012.pdf"
ingested_at: 2026-09-22T14:30:00+10:00

type: travel
reviewed: false                 # true once a human has confirmed the type
description: "Haneda → Sydney boarding pass"
tags: [japan-2019]

event_date: 2019-03-11
event_tz: Asia/Tokyo
expires_at: 2019-06-09          # empty means the type's default applies
expiry_cleared: false           # true when an expiry was emptied on purpose

total_minor: 12350              # absent, not zero, when there is no amount
currency: AUD

pages: 1
page_size: A4
producer: "ScanSnap Manager"
scan_created_at: 2019-03-11T20:30:00+09:00

group: 8f2a1c                   # set when several files are one document
needs_split: false              # one file, several documents, not yet split
documents:                      # absent: the whole file is one document
  - pages: 1-3
    type: statement             # only what the file leaves empty
    description: "bank, June"
    event_date: 2024-06-30
    event_tz: Australia/Sydney
    tags: [bank]                # added to the file's tags
  - pages: 2-5,9
ignored_pages: 6-8              # in no document on purpose
```

A split is described in [`import.md`](import.md#splitting-one-pdf). The file is
never cut, and a document can only add to the file's fields.

YAML because it is the format a human edits by hand, and a sidecar is the one
thing in a Box that is meant to survive the tool. `grep -r insurance` over the
tree is a supported way to find something.

The sidecar travels with the file, including into `trash/`, where it gains
`trashed_at`, `trashed_from` and `reason`.

`expiry_cleared` is how an emptied expiry is told apart from one never filled
in. Both leave `expires_at` blank, and without the flag the type's default
lifetime would come back on the next read, which is the opposite of what
emptying the field was meant to say.

A key this build does not know is **refused, not dropped**. A sidecar is edited
by hand and is the only copy of what it holds, so reading one, changing a field
and writing it back must never be the step that silently loses a line someone
typed; a mistyped key is reported instead. A file from a newer format says so in
its `version`, which is read first so that it — not one unknown key — is what
gets reported.

### Dates always carry a zone

The owner remembers local time. A ticket bought in Tokyo happened on 11 March
in Tokyo regardless of where it is being sorted, so nothing is converted, and
nothing is stored as UTC — that would lose the only reading that means anything
to the person looking at it.

An event is stored as a plain date plus an IANA zone name, not an offset:
`Asia/Tokyo`, not `+09:00`. Offsets change with daylight saving, so an offset
recorded now is wrong when the same date is recomputed in another month.
Timestamps that have a real instant — intake, the PDF's own creation date — are
stored in RFC 3339 with their offset, as read.

Sorting uses the absolute instant; display uses the local reading and names the
zone. They are separate concerns and the code keeps them separate.

Using IANA names means the zone database must be available, which the one
binary rule makes a build decision rather than a deployment one: `box` imports
`time/tzdata` so a copied `dgs` resolves zones on a machine with no
`/usr/share/zoneinfo`. It costs about 450 KB.

### Amounts are integers plus a currency

`total_minor` is the amount in the currency's minor unit, with `currency` an
ISO 4217 code. Summing dozens of `float64` values produces answers like
`1234.9999999998`, which is not acceptable for anything a receipt is kept for.
The minor-unit exponent per currency comes from a small compiled-in table —
most currencies have 2, JPY has 0, a few have 3.

The field is `total`, not `amount`, because an invoice has a subtotal, a tax and
a total and the first version records only one of them; a name that says which
one leaves room for the others later.

Three rules follow:

- **No amount is not a zero amount.** An invitation has no total. The field is
  absent, never `0`.
- **Negative totals are allowed** — refunds, cancelled tickets.
- **No converted amounts are ever stored.** A rate is a fact about an instant,
  is not available offline, and would never be updated. Totals are summed per
  currency and shown per currency; a single combined number would be a
  fabrication.

## Lifecycle

Whether something is still current is **computed, never stored**:

```text
expiry = expires_at, or event_date + the type's default lifetime,
         or nothing at all when the type has no default
dead   = expiry is set and is before today
```

Nothing has to run on a schedule and no stored field can go stale. A policy
scanned a year ago is expired because `insurance` has a one-year default
lifetime, not because anything was recalculated.

Three states exist: current, dead, and permanent — permanent being a type with
no default lifetime and no date filled in. A hand-entered `expires_at` always
wins over the type default, and an emptied one makes a file permanent, which is
how a concert ticket worth keeping escapes `ticket`'s same-day default. The
default lifetimes are in [`types.md`](types.md).

## The index is a cache, and lives on the local disk

`box` keeps one JSON file holding a record per scan, read into memory at
startup, and that is the whole query engine. A Box of a few thousand scans is a
couple of megabytes of metadata; filtering is a linear scan over a slice and
sorting is `sort.Slice`. At this size that beats a database on speed, on lines
of code and on dependencies, and the decision is cheap to revisit precisely
because the cache is discardable.

JSON rather than a binary encoding because the one thing wanted from a cache
that is behaving oddly is to read it. YAML for the truth a person edits, JSON
for the cache a program writes: the extension says which is which.

The cache is **not** in the Box. It lives under `box.cache_dir`, keyed by a hash
of the root path, for three reasons: SQLite-style databases on SMB or NFS are a
known way to corrupt data and to hang a process; a cache is per-machine by
nature, so two Macs get one each and never conflict; and keeping it out of the
tree keeps the tree's meaning simple — everything in the Box is truth.

```text
<box.cache_dir>/<hash of root>/
  index.json
  thumbs/a1/a1b2c3d4-300.jpg
  previews/a1/a1b2c3d4-1600.jpg
  previews/a1/a1b2c3d4-p2-300.jpg
```

Rules the cache obeys:

- **Nothing exists only in the cache.** Every field in it is derivable from a
  sidecar or from the file's own bytes. A test rebuilds an index from scratch
  and requires the result to equal the original; that test is the only evidence
  the word "discardable" is true.
- **A version mismatch rebuilds; it never migrates.** Unparseable, truncated,
  written by another schema version — all the same answer. Migration code for a
  discardable cache is code that can only be wrong.
- **A lost write costs nothing.** No WAL, no transactions, no careful fsync.
  The file is rewritten whole into a temporary file in the same directory and
  renamed. It is rewritten after every change rather than on the way out: a
  process closed by its window being shut never reaches a way out, and a cache
  written once makes every start pay for a refresh it does not need.
- **Staleness is per entry**, on `(relative path, size, mtime)`. A startup walk
  reads the directory listing only, never file contents, and re-reads just the
  sidecars whose triple changed. Entries in the cache with no file on disk are
  reported as orphans rather than dropped. A file whose name carries no digest
  prefix cannot pair with any sidecar, so it is reported too — see
  [adopt](view.md#the-exceptions-area) — rather than passed over.

### Rebuilding is not verifying

Two operations with costs three orders of magnitude apart, and so two commands
that must never be confused:

| | Reads | Cost on a NAS |
| --- | --- | --- |
| `dgs box index` | sidecars only; the digest is taken from the sidecar | seconds |
| `dgs box verify` | every byte of every file, to recompute digests | tens of GB |

Sharing one name would mean either never dares be run: the cheap one because it
might take an hour, or the expensive one because it seems to have already
happened.

## Thumbnails

A pyramid beside the index, following Lightroom's arrangement — previews next to
the catalogue, several sizes, discardable, the large ones expiring — with the
top of the pyramid left off:

- **300 px thumbnail** for grids. A few KB; never expires.
- **1600 px preview** for looking at one scan. Dropped after
  `box.preview.keep` without use. The sweep runs when a Box is opened, which is
  often enough for something whose only cost is disk.
- **Pages past the first**, at both sizes, for the page view of a multi-page
  PDF. Drawn when a page is first looked at, never during `import`, and kept
  under `previews/` whatever their size, so they expire with the previews: a
  long document read once must not leave pictures that are never swept.
- **No 1:1 preview.** Reading the original means opening the PDF in the system
  viewer. Writing a zooming document viewer is not this tool's job.

JPEG, not PNG: the content is photographic and PNG would be several times
larger. Images come from the page's embedded image stream, which is what a
scanner puts there, so no rasteriser is needed and no C library has to be
compiled in. A page that is not a single embedded image — a vector page, a
composite — is marked `needs-render` and simply has no thumbnail. Accepting
that gap is what keeps `dgs` one binary.

**They are generated during `import`, in the same pass as the digest.** Hashing
already reads every byte of the file over the network; the metadata, the
embedded image and both thumbnail sizes come out of that same read. Doing it
later would mean fetching the same file from the NAS three times, which is the
most expensive mistake available in this design.

## Deleting

Nothing is deleted. `view` moves a file into `trash/<date it was thrown away>/`,
with its sidecar, which gains where it came from and why.

Bucketing by the discard date matches how emptying works — "clear out what I
threw away before June" — where bucketing by the original date would mean
walking the whole tree.

`trash/` is inside the Box root so the move is a same-volume rename: atomic,
instant, no copy and no readback. Emptying it is a manual act, done in Finder or
a shell; `box` never removes a file. With no NAS snapshots behind it, the trash
is the only undo in the system, which is why `box.trash.keep` defaults to 90
days and is advice shown in `view` rather than a timer.

**The trash takes part in deduplication.** A digest in `trash/` is still known,
marked as discarded. Rejecting something during intake and being shown it again
on the next run — needing the same judgement a second time — is exactly the
tedium the tool exists to remove. Instead it says when the file was thrown away
and offers one key to skip it.

Rejecting during intake and discarding after filing are the same mechanism; only
`reason` in the sidecar differs. Two directories for these would be two things
to empty.

## First version

Two of the six open a workspace. Import and view are what a Box is looked
after with: a person chooses, corrects and decides, and there is something to
watch while bytes are copied.

The other four report and return. None of them writes anything into the Box —
init writes the marker, index saves the cache, which lives outside the Box, and
verify and dedupe write nothing at all — and none of them asks anything once it
starts. Nothing is repaired: an orphan, a mismatch and a duplicate are all
reported and left alone, because guessing which side is wrong is how a tool
destroys what it was asked to look after. A report with nothing to decide is
stdout's shape, and a keystroke in front of it keeps the slow one out of a
scheduled run.

They stay four rather than one because their costs differ by three orders of
magnitude: index and dedupe read sidecars, verify reads every byte in the Box.
Verify prints each file as it reads it, since a run that said nothing for an
hour could not be told from a hung one, and `--quiet` reduces it to the result.
It is also the one action that exits non-zero, when the Box no longer holds
what it says it holds — the finding a schedule must not see as success.

Confirmed for the first version:

| Command | |
| --- | --- |
| `dgs box init [<dir>]` | writes `dgs-box.yaml` at a root; a CLI action, not a TUI command |
| `dgs box import` | [intake](import.md): dedupe, metadata, thumbnails, verified publication, resumable |
| `dgs box view` | [browse and correct](view.md), expiry, trash, the exceptions area |
| `dgs box index [<dir>]` | rebuild the cache from sidecars; a CLI action |
| `dgs box verify [<dir>]` | re-hash and report mismatches; a CLI action, and the only one that fails |
| `dgs box dedupe [<dir>]` | the duplicate pass on its own, with no filing; a CLI action |

Deliberately not in the first version, each with a way through so nothing can
be stranded in the inbox:

| Not yet | What happens instead |
| --- | --- |
| Exporting one document of a split as its own PDF | the split is recorded in the sidecar; the file stays whole |
| The group browser | the `group` field **is** recorded at intake; only the interface waits |
| OCR | nothing |
| Full-text search | filters only: year, type, amount range, `reviewed`, current/dead |
| Remote or phone access | the server binds a loopback address |
| A generated tree of links for Finder | none; `view` is the view |
| Several inboxes | one configured default, overridable per run |

`group` is the one exception to leaving things out, and for a specific reason: a
contract scanned in three passes because the feeder jammed costs one keystroke
to bind while the three files are on screen together, and is close to
unreconstructable six months later. Collecting the data early is cheap
insurance; the interface is not.
