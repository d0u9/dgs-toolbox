# Doc Roadmap

The parts of `dgs doc`, in the order they build on each other. The design is
in [`index.md`](index.md).

Every part ends with something to try on the page, so the tool can be used a
step at a time as it grows. The page comes first; what it writes follows.

| Part | What there is to try | State |
| --- | --- | --- |
| M1 | The page: `dgs doc` serves it, lists the PDFs in a folder and previews one. Writes nothing | not started |
| M2 | Import on the page: pick a Template, answer its fields, see the Item. Needs M0 | not started |
| M3 | Documents on the page: add a revision, see HEAD move, move it back; distinguishing fields | not started |
| M4 | Views on the page: build a layout from keys, see the preview tree | not started |
| M5 | Missing keys on the page: the list of what is missing, filling keys in | not started |
| M6 | Export from the page: the plan, dry run, then writing to a folder Target | not started |
| M7 | Merging a cloned sub-tree, with conflicts resolved on the page | not started |
| M8 | Merging a sub-tree created with `init` | not started |

Metadata is kept in sidecars with a rebuildable cache, as in `dgs box`. **The
file names — marker, sidecar, manifest — are open.** M1 writes no files and can
be built before they are decided; M2 cannot.

## Deferred

- **Snapshot** export.
- **Bundles**.
