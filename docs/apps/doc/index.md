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
only what it asks. Nothing classifies a document automatically.

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
`month` and `date` from `issued_at`), and `ext`.

### Missing keys

Export refuses to write anything while a PDF the View selects lacks a key the
layout uses, and lists each PDF with what it is missing. The page lets the
owner fill the key in, on one PDF or on several at once. Only PDFs the View
selects have to have it; the rest of the type is not checked.

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

## Open questions

- **File names** — the marker, sidecar and manifest names. `dgs box` already
  uses `*.dgs-doc.yaml` for its sidecars, so doc's must differ.
- **Template format.**
- **Whether box keeps `identity` and `insurance`** once doc exists — decided
  after doc is built.

## Later

- **Snapshot** — an export into a new dated directory that is never updated,
  recording what was submitted for a visa or a claim.
- **Clone** — a sub-tree cloned from the full tree would remember the state it
  was cloned from, so its merge could compare three sides, as git does.
- **Bundle** — a hand-picked set of Items for one purpose, exported once.
