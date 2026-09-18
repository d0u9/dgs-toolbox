# Configuration: Inspect

`dgs conf inspect` answers "what does this configuration actually say" from
four angles — a node, an instance, a user, a secret — and draws the whole
thing as a connectivity graph. [`inventory.md`](inventory.md) is the model;
this page is a way to look at it without reading YAML by hand.

## Why a separate command from `export`

`export` answers "give me these files." Inspect answers "what is true here,
and why" — it renders nothing to disk, and its one side effect, editing a
secret, is opt-in and explicit. Splitting them keeps `export`'s tree about
selection and keeps inspect free to be a much denser, cross-referenced view
without competing for the same screen.

## Layout

Two regions, composed.

**Tabs** — Nodes and Users — in the shell's shared top-bar tab strip, per
[`tui.md`](../../tui.md#top-bar): `Tabs()` names them, `[` and `]` cycle, a
click sends `TabSelectedMsg`. Node and user are the two kinds with enough of
their own members (instances under a node; a device list, an access list
under a user) to want a whole index to themselves; instance and secret are
reached from inside those, not given a third tab, because neither has an
independent list worth browsing on its own — every instance already appears
under its node, every secret under its instance.

**Two columns, leading narrow**, per
[`tui.md`](../../tui.md#shared-column-skeletons), inside whichever tab is
active: that tab's index on the left, one third of the width; the detail pane
on the right, the rest. Moving the cursor in the index updates the detail
pane immediately — there is no separate open or closed state, and no
full-screen swap between them. Both stay on screen together, so a reader can
compare an instance's detail against its neighbour in the index without
losing either. Switching tabs resets the cursor to the top of the new index
and the detail pane to whatever that lands on — a tab switch is a fresh view,
not a continuation of the last one.

This is the first command to use the column skeleton `tui.md` already names
but no command yet implements it, and the second to use the tab strip
(`cred`'s key manager is the first).

## The four views

Node and user are tabs; instance and secret are reached inside the Nodes
tab, an instance being a row under its node and a secret a line of an
instance's own detail — neither has a list of its own worth a third tab, since
every instance already appears under its node and every secret under its
instance.

Each view starts from one identifier and shows everything reachable from it
one hop out. A reference to another thing — an instance's node, a user's
device — is named, not linked: the index beside it already lists everything
in the same tab, so following one within a tab is moving the cursor to that
row; following one across tabs — a user's device, an instance's node from the
Users tab — means switching tabs first. Whether cursor-following is still
enough once an index is long enough to scroll past what is visible is worth
revisiting; it is not, yet.

- **Node** — its networks and what it reaches; every instance on it; for a
  client node, its owner and the routes they are granted.
- **Instance** — its service, role, ports; the node it runs on; every route it
  is a hop of; its principals per port, by name only; its own secret names
  (not values); its upstream, if it has one.
- **User** — managed or unmanaged; their access list; for a managed user,
  their devices and the instance each derives; for unmanaged, the single
  identity every grant uses.
- **Secret** — one credential's path, decoded per
  [Secrets](inventory.md#secrets): which instance, which port, which
  principal. Masked by default. `Enter` reveals it in place, with the same
  warning [preview](export.md#previewing) already uses: plaintext, stays in
  this terminal's scrollback. `e` opens it for editing — see
  [Editing a secret](#editing-a-secret).

Jumping between views does not re-read the inventory; one load serves the
whole session, the same as `export`'s tree does today.

## The connectivity graph

The same load produces a graph: one container per node, one shape per
instance inside its container, one edge per resolved connection — the model
[`inventory.md`](inventory.md#what-is-derived) already computes as
`derive.Model.Edges`. An unmanaged user's derived instances have no
container, the same way their targets have no node in `export`'s tree.

`g` from any view opens it. In the terminal it is a static layout — enough to
see shape, not to explore. `w` opens the same graph in a local web page,
where it is force-directed and draggable, and clicking a node opens that
node's TUI view the way an instance row in `export`'s tree does.

The web page follows the pattern [`geo/gpx`](../geo/gpx.md) already
establishes: an embedded static bundle and a small JSON API. It draws with
[Cytoscape.js](https://js.cytoscape.org/), vendored the same way `geo/gpx`
already vendors MapLibre and uPlot — a static file, no build step. It is the
established choice for exactly this shape of problem — a node-link graph,
force-directed by default, with panning, dragging and click events built in.

Unlike `geo/gpx`'s `web/`, which is mapping and GPX editing mixed into one
bundle nothing else can reuse, the page and its vendored library live in a
package of their own: `internal/webgraph`, taking a generic node/group/edge
shape and knowing nothing about nodes, instances or secrets. It exposes an
`http.Handler` and a small JSON contract; a caller supplies a function
producing its own graph as that shape. This inventory's `topology.Graph` maps
onto it in a few lines inside `internal/apps/conf`, which is where the
Container/Shape/Edge vocabulary belongs and where it stays — `webgraph` never
imports `topology`, or anything else about this design. The next place `dgs`
wants a topology or dependency graph — GPX's own stop graph, one day, or
something in `cred` — imports `webgraph` and writes the same few lines,
rather than choosing a library or building a page again.

Secret values never reach the graph or its API. A node's tooltip names what a
credential is for, never its value.

## Editing a secret

`e` on a secret view opens every credential under that view's instance —
[`secret edit`](inventory.md#viewing-and-editing-several-at-once), which this
command is what implements it. This is also the answer to the open question
[`export.md`](export.md#open-questions) has carried since the TUI-first
design: a leaf command's page performing a write, inside the TUI, guarded by
the same confirmation dialog every other write in `dgs` uses. It is not a
Cobra subcommand and needs no separate decision about the command model —
[`secret show`](inventory.md#viewing-and-editing-several-at-once)'s read-only
half is the same view with `e` left unpressed.

Rules carried over unchanged from the design:

- A value changed is written back to its own file, one file per changed
  value.
- A key removed in the editor is left alone; deleting a credential is
  `secret sync`'s decision, not an edit session's.
- A key added that names no path the inventory implies is refused.

## What is not here

- **Editing anything but a secret.** The node, instance, user and route views
  are read-only; changing the inventory is still done in its YAML files.
- **A saved or exported image of the graph.** The web page is for looking, not
  for producing a document; if that turns out to be wanted, it is a small
  addition once the layout exists, not a reason to delay this.

## Milestones

1. **The graph model.** A package turning an inventory and its derivation into
   containers, shapes and edges — no TUI, no HTTP, tests over the same worked
   example [`inventory.md`](inventory.md) uses.
2. **The four views, read-only.** Node, instance, user, secret, masked,
   cross-linked. No graph yet.
3. **Revealing a secret.** `Enter` on a secret view, the same warning
   `export`'s preview already carries.
4. **Editing.** `e` on a secret view, the three rules above.
5. **The graph in the terminal.** A static rendering of milestone 1's model.
6. **The graph on the web.** The embedded page, force-directed, click-through
   to a TUI view.
