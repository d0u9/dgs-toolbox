# Doc Roadmap

The parts of `dgs doc`, in the order they build on each other. The design is
in [`index.md`](index.md).

| Part | What | State |
| --- | --- | --- |
| M0 | Decide where metadata is stored and the file names | open |
| M1 | `init`, the repository marker, importing a PDF with a Template, records | not started |
| M2 | Documents: revisions, HEAD, distinguishing fields and their uniqueness | not started |
| M3 | Views: query, selection, layout keys, preview on the page | not started |
| M4 | Export to a filesystem Target: plan, dry run, verified publication, manifest | not started |
| M5 | Missing keys: the refusal, filling keys in on the page | not started |
| M6 | Merging a cloned sub-tree, with conflicts resolved on the page | not started |
| M7 | Merging a sub-tree created with `init` | not started |

M0 comes first because every later part writes metadata.

## Deferred

- **Snapshot** export.
- **Bundles**.
