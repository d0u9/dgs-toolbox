# Box: view

`dgs box view` browses a Box and corrects what is in it. It also owns the
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

Every edit appends a line to the log at the Box root: what changed, from what,
to what, when. The log is never rewritten.

Batch editing is necessary — see [the case for
it](import.md#order) — and a batch edit is also the only way to get three
hundred records wrong at once. The log is what makes that recoverable, and
therefore what makes batch editing safe enough to offer.

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
| Tags | free-form |
| Producer | which scanner made it |
| Pages | one page, several pages |
| `needs_split`, `needs-render`, unknown type | deferred work |

`Producer`, page count and page size are worth having as filters precisely
because they cost nothing and exist for every scan, including everything that
was never described.

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

Nothing here is repaired automatically.

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
