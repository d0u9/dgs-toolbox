# Web API

The doc pages talk to `dgs doc` through JSON endpoints under `/api/`. This
records what they are, where they have converged and where not, and which of
them only read, so the reading part can later be served on its own.

## What holds

- **Thin handlers.** A handler decodes the request, calls package `tree` (or
  `view`, `export`, `merge`, `check`) and encodes the answer. The rules live
  in those packages, not in the web layer.
- **One vocabulary.** Requests name an Item `item` and a snapshot `digest`;
  `fields`, `tags`, `dir` and `path` mean the same everywhere.
- **One tree per request.** Every request carries `?tree=`; the server
  answers for that tree.
- **One error shape.** A failure is `{"error": "…"}` with a 4xx or 5xx status.
- **One client path.** Pages call the server only through `api()` and `post()`
  in `common.js`.

## Current contracts

- Single-Item mutations answer `{item, trash?, warning?}`. Item deletion
  includes the Item that was moved. Export and merge jobs retain their bulk
  result shapes; local cache actions retain their queue/OK answers.
- `POST /api/items/meta` takes `{item, digest?, notes?, tags?, shared_with?,
  retired?, reason?, frequent?, locked?, revision_tags?}`. Omitted fields are kept;
  empty strings and arrays clear them, and explicit `false` is set.
  Revision tags require `digest`. All requested changes validate before one
  sidecar write, retaining the existing history events and domain checks.
  HEAD remains its own endpoint. The former per-property endpoints are removed.
- In-place only: `POST /api/files/check` takes `{}` and answers
  `{report, recorded}`. The report uses `Items`, `Checked`, `Bytes`, `Problems`;
  each problem has `Kind`, `Path`, `Detail`, and file problems also carry
  `item`, `digest`, `candidates?`, `actual?`, `locked?`, `lost?`, `original?`.
  Missing content found elsewhere has kind `moved`; paths wait for confirmation.
  Unlocked changed PDFs are recorded as revisions and their Item IDs are in
  `recorded`. Per-file write failures remain in the report; successful writes
  stand and the Item cache follows them.
- In-place only: `POST /api/files/confirm` takes
  `{item, digest, previous, path}` and answers `{item}`. The sidecar's previous
  path and candidate bytes are checked again before publication. Metadata
  `locked` also requires an in-place tree.
- Handlers use a `tree.Store` opened once per tree, with domain operations
  composed through it. Domain packages and the cache retain that same Store;
  there is no unchecked root-based fallback. Storage paths are owned by the Store; missing-marker
  source browsing remains available, and writes still require the marker.
- All five definition kinds use the shared [definition routes](definitions.md#web-routes).

## Plan

1. **Done — with [layout step 1](layout.md#plan).** Handlers work through
   `tree.Store`, opened once per tree, and never pass a root, join a path or
   call `PDFPath`. No endpoint or answer changes.
2. **Done — one answer for a changed Item.** Every endpoint that changes an Item
   answers `{item, trash?, warning?}`: the Item as it now is, the trash path
   when something was moved there, and a warning when the change succeeded
   but a follow-up did not.
3. **Done — one endpoint for an Item's metadata.** Notes, tags, shared, retired,
   frequent and revision tags become `POST /api/items/meta` with
   `{item, digest?, notes?, tags?, shared_with?, retired?, frequent?,
   revision_tags?}`: what is present is set, what is absent is kept. HEAD
   stays its own endpoint: it changes which snapshot is current, not a
   property. The pages move to it in the same step.

Steps 2 and 3 change the pages and are done after the layout work, together
with one set of endpoints for every kind of definition
([Definitions](definitions.md#plan)).

## What only reads

The endpoints below change nothing in a tree. They are the candidates for a
read-only web service; those marked *local* read or act on the machine
running `dgs` rather than the tree, and stay out of it.

### Tree data

| Endpoint | Answers |
|---|---|
| `GET /api/state` | Items, Templates and the tree's settings |
| `GET /api/defs/templates` | Templates |
| `GET /api/defs/outlines` | Outlines and rules |
| `GET /api/defs/rules` | Rules, usage and editor support |
| `GET /api/defs/snapshots` | Snapshots |
| `GET /api/defs/cases` | Cases |
| `GET /api/history` | the tree's log |
| `GET /api/revision` | one snapshot of an Item |
| `GET /api/values` | the values a type's key has |
| `GET /api/issuers` | issuers for a type and country |
| `GET /api/supersession-candidates` | Items a new one may replace |
| `GET /api/search` | Items matching a query, text included |
| `GET /api/similar` | Items alike to one |

### Files

| Endpoint | Answers |
|---|---|
| `GET /api/original` | a stored PDF or page image |
| `GET /api/pages` | a PDF's page count; *local* for an import source |
| `GET /api/page` | one page as an image; *local* for an import source |
| `GET /api/text` | one page's text; *local* for an import source |

These four serve both a stored PDF, named by Item and digest, and a file in
the folder being imported from, named by `dir` and `path`. A service serves
only the first.

### Computed, sent as POST

They take a draft in the body, too large or structured for a query, and
compute an answer without saving it.

| Endpoint | Answers |
|---|---|
| `POST /api/export/plan` | what an export would write |
| `POST /api/cases/export/plan` | what a Case's export would write |
| `POST /api/merge/plan` | what a merge would copy |
| `POST /api/outlines/explain` | why an Item lands where it does |
| `POST /api/outlines/group` | an Outline's folders, grouped |
| `POST /api/rules/if` | a condition, parsed |
| `POST /api/rules/layout` | a rule's layout, parsed |
| `POST /api/snapshots/names` | names a Snapshot would give |
| `POST /api/links` | Items a field may link to |

### Local, not for a service

| Endpoint | Does |
|---|---|
| `GET /api/source`, `GET /api/source/file` | lists and reads the folder being imported from |
| `GET /api/export/progress` | reports this process's running export |
| `POST /api/reveal` | shows a file in Finder |
| `POST /api/read-all`, `POST /api/ahead` | read PDFs' text into the local cache ahead of need |
| `POST /api/items/reload` | rereads the sidecars into this process's cache |

Every other endpoint changes the tree: imports, edits, revisions, HEAD,
deletion, Templates, Outlines, rules, Snapshots, Cases, exports and merges.

### Doc 2 in-place trees

Imports and revision imports accept `folder` for verified copies. Generated
PDFs also accept `pdf_name` (a filename ending in `.pdf`), defaulting to the
first image stem plus `.pdf`. The PDF goes directly in `folder`; retained
images keep their filenames under `<folder>/<PDF stem>/`. Existing PDFs or
original-image folders and duplicate original filenames are refused. External ordinary revision files keep their
names directly in `folder`, with conflicts refused; `suffix` instead names a
copy beside the current file. Item and revision deletion count live references
before moving
objects to trash. Ordinary files import unchanged; PDF page/text endpoints
reject other file types. `GET /api/thumbnail?item=…&digest=…` serves a cached
native JPEG thumbnail, with a clear unsupported response outside macOS/cgo.
Show in Finder uses the existing reveal endpoint.

The existing rule and Outline definition endpoints use the Doc 2 `folders`,
`defaults`, `mount` and `pins` format for trees with `outlines: folders`. Group returns
empty folders, file numbers and `unplaced` Item IDs. Explain uses the same
placement engine. Export plans include skeleton directories and retain each
file's actual extension. Validity uses one date for the complete export plan.
Trees with `outlines: paths` retain Doc 1 definition formats. `POST /api/init`
accepts independent `layout` and `outlines` choices. `/api/state` returns
`outlines` and operation `capabilities`, rather than a layout indicator.
