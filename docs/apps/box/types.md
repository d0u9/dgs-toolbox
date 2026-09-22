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

| Type | What it covers | Default lifetime |
| --- | --- | --- |
| `ticket` | Ticket stubs: cinema, concerts, local transport | the event date |
| `travel` | Flights, boarding passes, hotel confirmations, car hire | the event date + 90 days |
| `receipt` | Receipts and invoices | the event date + 7 years |
| `statement` | Bills and account statements | the event date + 7 years |
| `insurance` | Policies, certificates of cover | the event date + 1 year |
| `contract` | Contracts and agreements | none |
| `identity` | Identity documents, visas, licences | none — enter the expiry by hand |
| `medical` | Prescriptions, results, medical receipts | none |

Seven years is the span tax records are normally kept for; it is a default, and
`box.lifetimes` overrides it for a reader whose jurisdiction says otherwise.

`identity` has no default on purpose. A passport's expiry is printed on it and
is the whole point of recording it, so a guessed date would be worse than an
empty field that `view` can list as missing.

## Keepsake

Nothing here expires.

| Type | What it covers | Default lifetime |
| --- | --- | --- |
| `letter` | Correspondence worth keeping | none |
| `ephemera` | Printed matter kept for its own sake: leaflets, brochures, an advertisement, a programme | none |
| `object` | A scan of a small object rather than of a document | none |

`ephemera` is the collectors' term for short-lived printed matter and covers
leaflets, brochures and advertisements in one type on purpose. Nobody looks for
something by remembering whether it was a leaflet or an advertisement, so
splitting it would add a decision at intake and buy nothing.

`object` is separate because it is not a document. It has no event date, no
amount and no other party — only a description of what the thing is. `view`
shows it as an image with a caption and leaves the document fields out rather
than presenting a column of empty ones.

## The rest

| Type | What it covers | Default lifetime |
| --- | --- | --- |
| `other` | Anything real that no type above fits | none |
| `unsorted` | Not yet decided | none |

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
