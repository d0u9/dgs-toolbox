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
| M7 | Merging a sub-tree created with `init`, with conflicts resolved on the page; a Target imports back through the same matching | not started |

The layout and formats are in [`index.md`](index.md#layout-on-disk).

## Deferred

- **Snapshot** export.
- **Bundles**.
- **Clone**, and the three-way merge a cloned sub-tree allows.
