# Box: view

The browse page of `dgs box` browses a Box and corrects what is in it. It also owns the
exceptions area, where anything the tool cannot explain about the tree is
shown.

Read [`index.md`](index.md) first for the layout, the sidecar and the lifecycle
rule. Taking new scans in is [`import.md`](import.md).

## It never writes a scan

`view` reads PDFs and images and writes only sidecars, the log and its own
cache. It can move a file into [`trash/`](index.md#deleting) and it can rewrite
metadata; it cannot alter a scan's bytes, and it has no code path that would.

This is why it is a separate command from [`import`](import.md). The two have
different risk: `import` copies, verifies and publishes, and needs a state
machine and confirmations; the worst outcome in `view` is a wrong label. One
entry point for both would force the careful ceremony onto the part that is
meant to be used casually.

## Correcting a classification is free

Today's `receipt` is next April's `invoice`. The cost of that change is one
sidecar rewrite, one appended log line and one cache entry:

- The file does not move, because [no path encodes
  metadata](index.md#paths-encode-only-what-cannot-change).
- The digest stays valid, because the bytes are untouched.
- No integrity check is triggered, because nothing was copied.

So it is instant, repeatable and safe, and the interface can treat
reclassification as an ordinary edit rather than as an operation. This is the
return on the layout decision.

Confirming a type also sets `reviewed: true`, which is what separates a checked
record from an intake guess.

## Changes are appended, not overwritten

Every edit appends a line to `dgs-box-log.jsonl` at the Box root: what changed,
from what, to what, when. The log is never rewritten.

Batch editing is necessary — see [the case for
it](import.md#order) — and a batch edit is also the only way to get three
hundred records wrong at once. The log is what makes that recoverable, and
therefore what makes batch editing safe enough to offer.

Cards are marked with Shift-click and the batch is one request. It sets only
what many documents share — the type, tags, and whether the type is
confirmed — because a description and an amount belong to one document. A
batch only adds tags: one list typed for three hundred scans never replaces
what each of them already carries. A
record the batch refuses is reported on its own and does not undo the ones that
were written: a batch is a convenience over a list of edits, not a transaction.

The trash is shown as advice: how much is in it, how old the oldest is, and how
much of it is older than `box.trash.keep`. Nothing is ever removed for it.
Emptying `trash/` stays something done by hand.

## Browsing, not searching

There is no search box in the first version, because there would be nothing
behind it. With no text layer and no OCR, the only searchable words are the
ones typed at intake, and intake deliberately leaves much of the Box as
`unsorted` with no description. A search field that returns nothing for most of
the collection teaches its owner not to trust the tool.

Filters are the whole retrieval mechanism, and they are built on what is
actually known:

| Filter | Comes from |
| --- | --- |
| Year | the event date, falling back to the scan date |
| Type | the [catalogue](types.md) |
| Current / dead / permanent | [computed](index.md#lifecycle) |
| `reviewed` | whether a human confirmed the type |
| Amount range | within one currency |
| Tags | free-form; every tag chosen must be carried ([tags](#tags)) |
| Producer | which scanner made it |
| Pages | one page, several pages |
| `needs_split`, `needs-render`, unknown type | deferred work |

`Producer`, page count and page size are worth having as filters precisely
because they cost nothing and exist for every scan, including everything that
was never described.

**Splits as documents** is a switch beside the filters, off by default. Off,
the grid shows one card per scan. On, a split scan becomes one card per split,
each with that split's own type, description, date, amount and first page, and
the filters and currency totals read those values. A split's card opens the
scan at that split. The switch is a way of looking, not a filter: clearing the
filters leaves it, and the browser remembers it.

A card's picture carries a folder button, shown on hover, that reveals the
scan's file in the file manager of the machine serving the page, selected in
its folder. The sample Box has no files and shows no button.

### Tags

Tags are free-form, and free-form drifts: `Japan`, `japan 2019` and
`japan-2019` are three tags to a filter. So a tag has one spelling — lower
case, spaces joined by a hyphen — applied when it is typed and again when it is
saved, and the spelling the Box already uses is the one offered first.
`GET /api/tags` is every tag in the Box with how many scans carry it; a scan
counts once, whether the file or one of its splits says it.

Everywhere a tag is typed — intake, the detail, a batch — each tag is a
bubble. Typing offers existing tags that start with, then contain, the text,
most used first. `Enter` or `Tab` takes the highlighted one, `,` takes the text
as typed, a first `Backspace` on an empty field picks the last bubble and a
second removes it. A tag no scan has yet is drawn dashed, so a near twin is
seen before it is saved. Renaming or merging tags is not done here.

The **Tags** filter takes only tags that exist. Several tags narrow with each
other: a card is shown when it carries every one. Beside each suggestion is
how many of the cards shown carry it. **All tags** opens a strip under the
filters of every tag in what is shown, most carried first, each a switch; the
strip stays open or closed as it was left. A tag on a card is the same switch.
The chosen tags are in the address, `?tag=japan-2019&tag=keep`, so a view by
tag is a bookmark.

### Pages

The detail of a PDF has the same page view as intake: a strip of every page
under the scan, shown and hidden with **Pages**, the picked page drawn large.
A scan [split](import.md#splitting-one-pdf) into documents carries a badge
with their count, and its splits are corrected here the way they were made at
intake: the same strip, `Space` to mark, `x` to ignore, `:` to type them all,
`-` to remove one, and the detail's fields describing whichever split is
picked. **Save** rewrites the sidecar; the file does not move.

### Reading

**Read** (`Enter`, or a double-click on a card) opens the picked card in a
reader: every page down one scrolling column, a strip of pages beside it, the
way a PDF viewer shows a document. It only reads — splits are marked in the
details, where the fields they describe are.

The reader is part of Browse, not a page of its own. It takes the place of the
filters and the grid and nothing else: the top bar keeps saying Browse, with
what is being read after it, and the status bar keeps its row, showing the page
and the reader's keys. **← Browse** or `Esc` puts the grid back, scrolled where
it was. The strip of pages is dragged wider or narrower at its edge, and the
width is remembered.

A split's card is read as its own document: only its pages, numbered within
it, with the page of the scan named beside the number. A scan's card reads the
whole scan. What is on screen is what the split will be taken to be, and a
split that looks wrong is corrected from the scan's card.

A page is drawn only while it is near the window, and one scrolled far away
gives its picture up again, so a long scan never holds every page decoded at
once. Each page's place is held from the start at the shape of a sheet of
paper, so the scrollbar is right before anything has loaded. The pictures are
the same 1600 px previews the details show, from the same cache.

`j`/`k` or the arrow keys turn a page, `g`/`G` go to the first and last,
`+`/`-` or `⌘`/`Ctrl` with the wheel size the pages — the wheel about the
cursor — and `Esc` closes. Two sizes follow the window: **Width** (`w` or
`0`) makes pages as wide as it, for reading small print down a page; **Page**
(`p`) makes each page exactly as tall as it, so a whole page is seen at once
and every turn lands on one, each page keeping its own shape. The size is
remembered by the browser.

### Unfiling

A scan filed wrongly as a whole — the wrong pages together, the wrong file —
can be taken back to intake with **Unfile**, offered only while the inbox file
it came from is still there. Nothing is deleted: the Box's copy goes to the
trash with the reason `unfiled to be filed again`, and the scan is waiting at
intake again with everything its sidecar said as its draft. Duplicate
detection passes over a trashed copy with that reason, so the scan comes back
as a scan and not as something already thrown away.

### Sorting

Sorting uses the event date where there is one and the scan date otherwise, and
**says so on screen when it has fallen back** — otherwise a group of scans
sorted by two different meanings looks like one ordering that is subtly wrong.

Absolute instants sort; local readings display, with their zone named. The
description takes no part in sorting: it will be written in whichever language
suits the document, and a collation rule for that is a problem worth not having
while the first version has no search.

### Amounts are summed per currency, never combined

```text
AUD  1,240.50   (12)
USD    380.00    (3)
CNY  2,100.00    (5)
```

A combined figure would need a rate, which is a fact about an instant,
[is not available offline and is never
stored](index.md#amounts-are-integers-plus-a-currency).

This shapes the amount filter as well: "over 100" has no meaning across
currencies, so the range filter applies within a selected currency.

### `object` shows fewer fields

A [scan of an object](types.md#keepsake) has no event date, no total and no
other party. It is shown as an image with its description, not as a document
form with a column of empty fields.

## Expiry and clearing out

The lifecycle rule gives every scan a state without anything being stored, so
`view` can answer "what is dead" at any moment. That filter is the practical
handle for clearing out: select the dead ticket stubs, send the selection to
the trash in one action.

Emptying the trash is not `box`'s job — it is done in Finder or a shell. `view`
shows how much is in there and how old it is, and treats `box.trash.keep` as
advice rather than as a timer, because with no NAS snapshots behind it the
trash is the only undo in the system.

Filling in a missing expiry is its own pass: `identity` and `insurance` are the
types where the date is printed on the document and matters, and a filter for
"no expiry on a type that needs one" is how that list is produced.

## The exceptions area

Three things can be true of a tree that has been touched outside the tool, and
they are different enough to need different answers. They live in their own
area, not mixed into the browsing list — exceptions mixed into a daily list are
exceptions nobody reads after the third day.

That area is its own page, `/check/`, not a band on browse. Exceptions are
rare, so browse shows only a link in its top bar with their count, and shows
nothing at all when there are none. The page groups them by kind; each group
says in one paragraph what it means and what to do, then lists the files,
name first and directory after.

| What | What it means | What is offered |
| --- | --- | --- |
| A scan with no sidecar | put there by hand, or a lost sidecar | **adopt**: describe it in place, no move |
| A sidecar with no scan | the file was moved or removed by hand | reported only; it may have been moved on purpose |
| A digest mismatch | the bytes changed | see below |

**Adopt matters.** Dropping a file into the tree by hand is a deliberate act,
and the right response is to take it in where it is rather than to call it an
error. It runs the same metadata and thumbnail work as
[`import`](import.md#metadata-and-thumbnails-in-one-read) without copying
anything.

The file stays in its own directory, because that directory records an intake
date adopting does not get to rewrite. One thing about it does change: a
sidecar is paired with its scan by the digest prefix both names carry, so a
file that arrived without one is renamed **inside that same directory** to gain
it. Without that, the sidecar written for it would be an orphan the moment it
was saved. A file that already carries the right prefix is not touched at all.

A file whose name carries no digest at all is still reported. Pairing skips it,
and a file the tool passed over silently would be a file nobody is ever told
about — which is the same failure as an unreported skip at intake.

Nothing here is repaired automatically.

Verify is offered on that page as well as from `dgs box verify`, behind a button that
says what it costs: it reads every byte of every file, which is tens of
gigabytes over a network filesystem. What it finds joins the exceptions.

Thumbnails are a local cache, so a broken one is mended by drawing it again
rather than reported. Browse's detail panel has *Redraw thumbnail* for one
scan; the check page has *Redraw all*, which reads every file like verify
does. Both drop the scan's stored pictures and draw page one again from the
file (`POST /api/redraw`, an empty digest meaning every filed scan). Only the
cache changes; the Box is never written to. A scan with no picture in it is
listed as one that could not be redrawn.

### A mismatch is never "corrected"

A file whose bytes no longer match its recorded digest is reported loudly,
marked, and left alone. Rewriting the sidecar to match the new bytes would
erase the only evidence that anything happened, and on a NAS with no snapshots
that evidence may be the first sign of bit rot.

The report names the recorded digest and when it was recorded, so "this file
was still intact on 2026-09-22" is readable. Re-registering the file requires
an explicit confirmation and keeps the old digest in the log.

## `verify`

`dgs box verify` recomputes every digest from the file's bytes and is the only
thing that can find a mismatch. It reads tens of gigabytes across the network,
so it is an explicit command with progress, interruptible and resumable, and
is never run as a side effect of anything else. Rebuilding the index is the
[cheap operation](index.md#rebuilding-is-not-verifying) and is a different
command on purpose.
