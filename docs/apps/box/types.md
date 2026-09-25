# Box: types

Every type a scan in a Box can have, and how long that type is useful for. A
test checks that this document names every type `dgs` registers and the default
lifetime it registers it with, the same way
[`../cred/actions.md`](../cred/actions.md) is pinned for Actions. The test
cannot check that a description is still true; that part belongs to whoever
changes one.

How a lifetime becomes an expiry date, and what overrides it, is in
[`index.md`](index.md#lifecycle). The keys that override a default are in
[`../../configuration/box.md`](../../configuration/box.md).

## Two natures

The catalogue is in two groups because the material is. A **utility** scan is
kept because it may be needed, and stops being useful at a knowable point. A
**keepsake** is kept because its owner wants it, and never stops. This is why
half a Box has no expiry and no amount, and why asking for those fields on
every scan would only make sorting slower.

The grouping is not a stored field. It is the reason a type has a default
lifetime or has none.

## Utility

| Type | Key | What it covers | Default lifetime |
| --- | --- | --- | --- |
| `ticket` | `t` | Ticket stubs: cinema, concerts, local transport | the event date |
| `travel` | `a` | Flights, boarding passes, hotel confirmations, car hire | the event date + 90 days |
| `receipt` | `r` | Receipts and invoices | the event date + 7 years |
| `statement` | `s` | Bills and account statements | the event date + 7 years |
| `contract` | `c` | Contracts and agreements | none |
| `medical` | `m` | Prescriptions, results, medical receipts | none |
| `manual` | `h` | Instruction manuals and user guides for things owned | none |

Seven years is the span tax records are normally kept for. It is not
configurable: a lifetime is part of what a type is, so changing one is a change
to this table and to the code that registers it, in the same commit. An expiry
that matters on one scan is entered on that scan, which always wins over the
default.

"The event date" and "none" are different answers, not two spellings of one.
A ticket stub expires on the day of the event; a contract has no end to compute
at all. Both are permanent only in the sense that neither needs arithmetic
afterwards.

Identity documents and insurance policies are not Box types: they are
important documents, kept by [`dgs doc`](../doc/index.md) with their expiry
entered from the document. A scan whose sidecar still says `identity` or
`insurance` is read as it is and shown as an unknown type, with no expiry; its
sidecar is never rewritten.

`manual` has no lifetime because a manual is useful for as long as the thing it
describes is owned, and no date on it says how long that is. It is a utility,
not a keepsake: once the thing is gone the manual can be discarded by hand.

## The key column

Each type answers to one keystroke on the intake page, which is what makes a
few hundred scans in a row possible. The keys are a mnemonic where one is free
and arbitrary where it is not — `a` for travel (air), `b` for memorabilia
and `h` for a manual (handbook) because `t`, `i` and `m` are taken. A test refuses duplicate keys, and the intake
page refuses to start when a type key collides with one of its own commands,
case-insensitively, so a new type picks a free one rather than quietly stealing
an established reflex.

## Keepsake

Nothing here expires.

| Type | Key | What it covers | Default lifetime |
| --- | --- | --- | --- |
| `letter` | `l` | Correspondence worth keeping | none |
| `memorabilia` | `b` | Personal keepsakes: yearbooks, commemorative books, invitations and cards kept for their memories | none |
| `ephemera` | `e` | Printed matter kept for its own sake: leaflets, brochures, an advertisement, a programme | none |
| `object` | `o` | A scan of a small object rather than of a document | none |

`memorabilia` is for something kept because it recalls a person, place or time.
A friend's card, invitation or yearbook belongs here when the memory is the reason for
keeping it; a letter kept for its correspondence remains `letter`.
`ephemera` is the collectors' term for short-lived printed matter and covers
leaflets, brochures and advertisements in one type on purpose. Nobody looks for
something by remembering whether it was a leaflet or an advertisement, so
splitting it would add a decision at intake and buy nothing.

`object` is separate because it is not a document. It has no event date, no
amount and no other party — only a description of what the thing is. `view`
shows it as an image with a caption and leaves the document fields out rather
than presenting a column of empty ones.

## The rest

| Type | Key | What it covers | Default lifetime |
| --- | --- | --- | --- |
| `other` | `z` | Anything real that no type above fits | none |
| `unsorted` | `u` | Not yet decided | none |

`unsorted` is a **state**, not a kind of document, and it is what makes the
inbox drainable: a scan nobody can classify in the two seconds available is
taken in anyway and decided later. It appears in this table because it occupies
the same field, not because it describes anything.

## Why the list is this short

A type is chosen with one keystroke a few hundred times in a row, so the
catalogue has to fit in one glance. Distinctions that matter are better made
with `tags`, which are free-form and cost nothing to add or drop.

`receipt` and `invoice` are the obvious candidate for an eleventh type and are
deliberately one. They are told apart by whether the thing was already paid,
which is not how anyone searches, and a tag separates them for the rare case
that needs it. Renaming a tag is a metadata edit; splitting a type is a rewrite
of every sidecar that used it.

## Changing this list

- **Reading accepts an unknown type.** A sidecar naming a type this build does
  not register is kept as it is, shown as unknown, and never silently changed to
  `other`. Writing validates.
- **Renaming a type rewrites every sidecar that used it** and appends one line
  per file to the log. There is no alias mechanism; a type is a name in the
  files, not a pointer.
- **Adding or renaming a type, or changing a default lifetime, is not finished
  until this document says so**, because the test pins the two together.
