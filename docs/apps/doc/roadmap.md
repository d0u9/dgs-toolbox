# Doc Roadmap

The parts of `dgs doc`, in the order they build on each other. The design is
in [`index.md`](index.md).

Every part ends with something to try on the page, so the tool can be used a
step at a time as it grows. The page comes first; what it writes follows.

| Part | What there is to try | State |
| --- | --- | --- |
| M1 | The page: `dgs doc` serves it, lists the PDFs in a folder and previews one. Writes nothing | done |
| M2 | Import on the page: pick a Template, answer its fields, see the Item | done |
| M3 | Documents on the page: add a revision, see HEAD move, move it back; distinguishing fields | done |
| M4 | Views on the page: build a layout from keys, see the preview tree; missing-key default, clean segments, numbered duplicates | done |
| M4a | `dgs doc verify` | done |
| M4b | Field types (`text`, `date`, `select`, `item`), dates found in text, notes | done |
| M4c | Suggesting a type from filed Items' text | done |
| M5 | Missing keys on the page: the list of what is missing, filling keys in | done |
| M6 | Export from the page: the plan, dry run, then writing to a folder Target; incremental | done |
| M7 | Merging a sub-tree created with `init`, with conflicts resolved on the page; a Target imports back through the same matching | done |
| M8 | Searching Items by their text on Browse, and "more like this" | done |
| M9 | Several trees (`doc.trees`), switched from the top bar | done |
| M10 | Cases: Items added as a matter needs them, still-needed notes, export, archive | done |
| M11 | Snapshots: fixed subtrees taken from a rule and edited by hand, put in an Outline's tree as folders beside its rules | done |

The layout and formats are in [`index.md`](index.md#layout-on-disk).

## Deferred

- **Convergence** — types that inherit, conditions as one `if` expression, a
  rule's paths nesting: built, and in [`index.md`](index.md). What is left is
  the one-off migration of the real tree. [`convergence.md`](convergence.md)
  keeps the reasoning.

- **Accounts on a server** — not decided. The same web pages served to several
  people, each seeing and editing only part of the Items.
  - Sign-in with accounts in a YAML file, password hashes only; HTTPS.
  - Access is per Item, not per folder: a generated tree is a view, and one
    Item appears in several. What a person sees is the tree built from the
    Items they may read.
  - An `acl` field on `record`, inherited by every type, holds groups, not
    people. A Template's `defaults` fills it when a PDF is added. Only an
    owner or an admin may change it.
  - `users.yaml` holds groups and a `policy` of `if` expressions per group,
    for access without touching sidecars.
  - Readers are the owner, the members of the groups in `acl`, and the
    groups a policy matches. Grants only add; there is no deny. Write access
    comes from policy only.
  - Every endpoint filters: search, Browse, an Item, its PDF, `/api/values`,
    `about` links (a link to an unreadable Item shows no content), export,
    Outlines, Snapshots. Templates and Rules are admin only.
  - Concurrent writers need a lock or a version check per sidecar; history
    events record who made them.
  - Shipped as the same `dgs` binary in a Docker image, run as a server mode
    of `dgs doc`, not as a second program.
  - Groundwork worth doing before: every Item read goes through one function;
    history events carry an `actor`; `internal/doc/expr` knows nothing of
    rules.

- **Integrity** — not decided. The PDFs are the data to protect, so the
  answer is at the file level, not a database: content addressing, atomic
  writes, `verify` and backups.
  - An operation touching several files (a move on a changed date, a merge)
    writes each file as a temporary, reads it back, then renames it, as Photo
    Import does. Steps are ordered new before old and are idempotent, so an
    interrupted run leaves at worst an extra file, which `verify` reports and
    a rerun finishes.
  - `verify` also checks fields: a required one missing, a value its type
    rejects, an `about` naming an Item the tree lacks. Run before and after
    the convergence migration.
  - `verify --deep` hashes every PDF again to catch bit rot. A server runs
    `verify` daily and `--deep` weekly, and reports what it finds.
  - A deleted or damaged PDF comes back only from a backup (NAS snapshots, an
    off-site copy); `verify` finds it, and its digest checks the restore.

- **Clone**, and the three-way merge a cloned sub-tree allows.
