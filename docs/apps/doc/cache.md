# Doc Cache

A question the page asks while a form is filled in — which Items a link field
may point to, which issuers a type has had — is answered today by reading
every sidecar in the tree. On a tree reached over SMB that is one network
round trip per Item, so the answer slows as the tree grows, whatever the
question. The cache keeps the tree's Items parsed and indexed, so a question
costs the Items it concerns and not the tree.

## Truth

The sidecars are the only truth, as [`AGENTS.md`](../../../AGENTS.md) requires
of every cache. The cache is a copy that can always be thrown away:

- It is rebuilt from the tree's sidecars and Templates alone. A test builds
  one, rebuilds it from the files, and compares.
- A cache that is missing, unreadable, or of a version this `dgs` does not
  recognise is discarded and rebuilt, never repaired or upgraded. There is no
  migration code.
- A write goes to the sidecar first. Only once the sidecar write has
  succeeded is the cache brought up to date, and only then does the page hear
  that the write succeeded — so the next question sees it.
- A write from the page checks the Item against the others — a unique
  field, a link, a PDF already kept — against the cache, once the change
  mark has confirmed it current, so a write reads the Item's own sidecar and
  not the tree. The Item being written is always read from its sidecar. A
  sidecar edited by hand is not seen by those checks until Reload; `dgs doc
  verify` always reads the sidecars. The command line's writes read the
  sidecars.

## Where it lives

In `doc.cache.dir` ([configuration](../../configuration/doc.md)), one
subdirectory per tree, beside the text already cached there. It is outside
the tree and belongs to one machine. Deleting it costs one full read of the
tree.

## What it answers

The cache holds each Item as its sidecar says, and indexes kept alongside:

| Index | Answers |
| --- | --- |
| Item ID, revision | one Item, its revisions, its preview and history |
| PDF SHA-256 | whether a PDF is already in the tree |
| type and the fields a Template matches on | the Items a link field may point to |
| type, country and field value | issuer and other suggestions |

A link field is answered by narrowing to the Items its `match` allows, then
checking `within` on those few. Every rule of matching stays as it is: a
field left empty in the form does not narrow, and a date is held by a span
exactly as `anchor.Holds` says.

## Parts

Kept apart, as [`AGENTS.md`](../../../AGENTS.md) asks of every algorithm:

- **Indexing and querying** — a doc domain package. Items in, answers out; no
  files, HTTP or configuration. The web page and `dgs doc link` use the same
  one.
- **Keeping** — reading and writing the cache under `doc.cache.dir`, noticing
  a version or a file it cannot read, rebuilding.
- **Serving** — the web handlers ask the cache and draw what it answers.

## Writes the cache follows

Every write `dgs` makes to a tree updates the cache before it returns:
adding and editing an Item, adding a revision, changing its HEAD or type,
deleting it to the trash, merging, linking, filling in fields in bulk, and a
Template change that alters what its Items inherit. A write that happens
while the cache is being rebuilt is not lost: a rebuild is published only if
no write has come in since it began, and is otherwise run again.

## Changes made elsewhere

A tree is also written by `dgs` on another machine, arrives by rsync, or has
a sidecar edited by hand. Two things notice it:

- **The change mark.** `dgs-doctree.changed`, at the tree's root beside
  `dgs-doctree.yaml`, holds a random token. Every write `dgs` makes to the
  tree writes a new one, after the write itself. The cache remembers the
  token it was built with; before answering a question the page reads the
  mark — one small read, whatever the tree's size — and a token that differs
  sends the cache to read the tree again before it answers. The token is
  compared rather than the file's time, which a network filesystem may keep
  too coarsely to tell two writes apart. A tree without the mark is read
  again once, and the mark is written. The mark carries no metadata: it says
  only that something changed.
- **Reload.** A sidecar edited by hand writes no mark. The page has a Reload
  that reads the tree again.

The mark is not proof the cache is right; only reading the sidecars is.
`dgs doc verify` reads them and does not use the cache.
