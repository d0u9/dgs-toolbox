# Configuration: Inspect

`dgs conf inspect` answers "what does this configuration actually say" from
six angles — a node, a user, a service, an instance, a port, a secret — and
draws the whole thing as a connectivity graph. Four of them are tabs; the
other two are rows inside them. [`inventory.md`](inventory.md) is the model;
this page is a way to look at it without reading YAML by hand.

## Why a separate command from `export`

`export` answers "give me these files." Inspect answers "what is true here,
and why" — it renders nothing to disk, and its one side effect, editing a
secret, is opt-in and explicit. Splitting them keeps `export`'s tree about
selection and keeps inspect free to be a much denser, cross-referenced view
without competing for the same screen.

## Layout

Two regions, composed.

**Tabs** — Nodes, Users, Services and Secrets — in the shell's shared
top-bar tab strip, per [`tui.md`](../../tui.md#top-bar): `Tabs()` names them,
`[` and `]` cycle, a click sends `TabSelectedMsg`. Each has enough of its own
members to want a whole index: instances under a node; the devices and
credentials a person's files hang off, and the files under each; the
deployments of a service under it; the whole store under Secrets. Instance and port get no tab of their own,
because neither has an independent list worth browsing — every instance
already appears under its node and under its service, and every port under
its instance.

**Nodes holds machines and nothing else.** What is rendered for a person with
no device file has no node file behind it, so it is not a node and does not
appear there; Users is where it sits. The two answer one question each — what
runs on this machine, and what does this person receive — and a tree mixing
them answered neither cleanly, since the row standing in for a person had to
call itself a node and contradict itself in the same breath.

Under Users a device and a credential sit at one level, because they answer
one question: which of a person's identities this file was rendered for. A
person with device files shows those; one without shows the credentials
themselves.

Nodes and Services are the same set of instances indexed from opposite ends.
A node answers "what runs on this machine", which is what an export is for; a
service answers "where is this deployed", which otherwise means reading every
node file in turn and is the question asked when a service is upgraded or
its template changes. Neither can be derived from the other by folding: one
groups by machine and the other by software, and both groupings are wanted.

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

## The six views

Node, user, service and secret are tabs; instance and port are reached
inside them — an instance is a row under its node or its service, and a port
a row under its instance in the Services tree.

Each view starts from one identifier and shows everything reachable from it
one hop out. A reference to another thing — an instance's node, a user's
device — is named, not linked: the index beside it already lists everything
in the same tab, so following one within a tab is moving the cursor to that
row; following one across tabs — a user's device, an instance's node from the
Users tab — means switching tabs first. Whether cursor-following is still
enough once an index is long enough to scroll past what is visible is worth
revisiting; it is not, yet.

- **Node** — its networks and what it reaches; every instance on it, derived
  ones marked as such, since a client node's instances are all derived and
  listing only the authored ones leaves the heading empty; for a client
  node, its owner.
- **Instance** — its service, ports and its own values; the node it
  runs on; every route it is a hop of, and for a derived client, the route it
  was derived for, which is not the same relation; every secret it holds, by
  path and by who holds it, never by value; its upstream, if it has one.
- **User** — their credentials, each with its note; their access list, each
  route with the hop it enters, since a route name is chosen for the person
  picking it in their client and says nothing on its own here; their devices
  and what each granted route derives for each of them, or, for someone with
  no device file, what each credential derives. A granted route that derives nothing —
  an entry service with no `exports` — is listed saying so, because the
  person still holds a credential for it and an absent line cannot be told
  from a missing route.
- **Service** — what its manifest declares: `auth`, the ways `exports` names,
  the template, the defaults kind, the output name, its own secrets and which
  of them `shared` sends outward; where it is deployed; and how many files the
  routes into it produce for people. The manifest half belongs here and
  nowhere else: a node view is about one machine, and a template path or an
  output name is a fact about the software. An export is not in this index —
  nothing deploys one — and is named on the service that offers it.
- **Port** — one named port of one deployed instance: its number, its
  instance and node, the routes entering it, and the account table it holds.
  A port is the unit a table belongs to, so two ports of one process carry
  two independent sets of credentials and the view is per port.

## The Secrets tab

Every other view reads the inventory. This one reads the store as well, and
shows the two against each other: the tree is laid out as the store is —
instance, then port or `own`, then one row per credential — and each row
says whether the inventory implies it, whether a file is there, and both
when they agree.

That comparison is the tab's reason to exist, because each way they can
disagree is silent everywhere else. A path with no file renders an error in
the middle of an export. A file nothing implies is a credential `secret
sync` will never regenerate and never delete, and on disk it looks exactly
like one that is still in use. The status bar carries the counts, and says
`in step` when there is nothing to report.

A `.previous` beside a value is named with its age, and past seven days it
says the rotation is unfinished — the same limit
[`validate`](inventory.md#rotation) enforces, reached here by looking rather
than by running a check.

**No value is read.** The path, the account it belongs to and its state are
all derived from the inventory and from the store's shape. Revealing one is
[its own step](#editing-a-secret): a view that printed values on the way
past would put every credential in the terminal's scrollback.

The Services index is **deployments only, three levels deep**: a service,
the instances of it that run somewhere, and under each of those the ports it
listens on with the accounts on each.

An export instance is not in it. It is a file handed to a person, and
it belongs under that person in the Users tab; listing it here made one
Shadowsocks server serving five people read as six instances of the service,
which answers neither "where does this run" nor "who has this". The number
of client files is kept, beside the instance that serves them.

The index goes one level deeper than the Nodes index because the port is
where the account table is. An index stopping at the instance can say a
Shadowsocks server has two ports and cannot say that four people are on one
of them and one relay on the other, which is the question a per-principal
service is looked at for.
- **Secret** — one credential's path, decoded per
  [Secrets](inventory.md#secrets): which instance, which port, which
  principal, and the account name it renders as. Its state against the
  store, and its rotation leftover when it has one. The value is not read;
  `Enter` reveals it in place, with the same warning
  [preview](export.md#previewing) uses: plaintext, stays in this terminal's
  scrollback. `e` opens it for editing — see
  [Editing a secret](#editing-a-secret).

Jumping between views does not re-read the inventory; one load serves the
whole session, the same as `export`'s tree does today.

## The connectivity graph

The same load produces a graph, and **a port is the unit of it**: a port is
what holds principals, what a grant is written against and what a secret
belongs to, so a server listening on two of them is two shapes and a line says
which one it lands on. The boxes nest — a group, a node inside it, a process
inside that holding the ports one running program serves — and every edge is a
resolved connection from the model
[`inventory.md`](inventory.md#what-is-derived) computes as
`derive.Model.Edges`. A line carries the account crossing it, because the port
it lands on says the rest.

**A line out of a reverse proxy carries the name that route arrived at**
instead — the `published` of the port it lands on. Every line out of a
[fan-out instance](inventory.md#a-service-that-fans-out) leaves one entrance,
so what tells them apart is exactly that name; the proxy itself holds no
credential, and repeating its instance name on each line would say nothing the
shapes at either end do not already. An ordinary edge into a published port is
unchanged: one line has no siblings to be told apart from, and the account
crossing it is still the thing on it.

A site a proxy serves from its own disk has no downstream instance and so no
line at all — it is the proxy's own configuration, not an edge — which is why
the picture shows fewer names than the rendered file does.

Someone with no device file has no node box, so the box standing in for one is
named after the credential this file authenticates with — the same name the
secrets tree files it under.

`t` from any view opens it, in a local web page: force-directed, draggable,
and fitted to what it holds. It is `t` for topology because `g` is the first
row and `w` folds everything, both from the File Explorer's keys.

**There is no terminal rendering of it.** A graph is the one thing an index
cannot be — an index is a tree, and what this shows is the edges between its
branches — and a static block-character layout would be a worse picture of
that than the four indexes already are. The web page is where a graph is
worth drawing, so it is the only place it is drawn.

The page reads `/api/graph` on every load, and the graph is built from the
root on disk when that request arrives: reloading after an edit shows the
edit. It listens on loopback only — the shape of a whole inventory is not a
thing to serve to the network by default — and one page runs per process, so
reopening the command reuses it rather than failing to bind.

An instance is drawn as a server or a client by **where it is written**, not
by how many edges touch it: a service reached from a browser has no edge at
all, and a service with no `clients` derives none, so nothing points at
it either. Both are servers. An authored instance that also dials out is a
relay — one hop in the middle of a chain. Clicking a shape dims everything
it is not connected to, which is how one instance's reach stays readable in
a picture too dense to trace by eye.

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
`http.Handler`, a `Server` and a small JSON contract; a caller supplies a
`Load` function producing its own graph as that shape, plus a `Legend` saying
what each `Kind` means. This inventory's `topology.Graph` maps onto it in one
file inside `internal/apps/conf`, which is where the Container/Shape/Edge
vocabulary belongs and where it stays — `webgraph` never imports `topology`,
or anything else about this design. The next place `dgs` wants a topology or
dependency graph — GPX's own stop graph, one day, or something in `cred` —
imports `webgraph` and writes the same one file, rather than choosing a
library or building a page again.

### What the picture shows at each size

A whole inventory drawn at once is unreadable: a hundred ports and eighty
lines fitted to one screen is a grey mass, whatever the colours are. So the
picture has three levels, and which one is drawn follows how far in the
reader has zoomed.

- **Overview.** Each machine is one shape carrying its name, and the lines
  between machines are one line per pair, labelled with how many connections
  it stands for. The owner boxes stay, so the small picture still says whose
  machines these are.
- **Medium.** Everything is open — processes, ports, every line — with the
  second line of each label and the words on the lines left off.
- **Full.** Everything, as authored.

The levels are `webgraph`'s, not this app's: a group carries `collapse`,
meaning *this box stands for its contents when the picture is small*, and
this file sets it on the node boxes and on the credential box someone with no
device file gets instead of one. Which level of its own hierarchy is worth seeing first
is the caller's decision, and the only thing the page needs to be told.

The overview is laid out on its own — the shut boxes and the lines between
them, run through the same force-directed layout — rather than being the
detailed picture seen from further away, which would be exactly as tangled.
Both geometries are kept, so moving between levels moves shapes rather than
re-running a layout, and the zoom is carried across the switch so nothing
changes size on screen: the reader keeps looking at what they were looking
at, in more or less detail. A button in the header pins a level for a reader
who wants the small picture on a big screen.

### Two views of the same graph

A force-directed picture says where things sit, and with eighty lines in it
that is most of what it says: the lines themselves stop being followable. So
the page draws the same graph two ways, and a button in the header switches
between them.

- **Network.** The picture as a picture, at the three levels above.
- **Ring.** One machine per place on a circle, ordered by the box it belongs
  to so one owner's machines are neighbours, with the owner's name written
  outside the ring against its stretch of it. Every line is a chord across
  the middle, and a line standing for several connections is drawn as thickly
  as the number it stands for. The ring is always the machine-level picture:
  a hundred ports on a circle is the tangle it exists to answer.

A ring cannot say where anything is, and it does not try to. What it says is
who reaches whom, and clicking one machine — which dims everything it is not
connected to, the same as in the network view — is how a single machine's
reach is read off it.

Secret values never reach the graph or its API. A node's tooltip names what a
credential is for, never its value.

## The check report

`dgs conf --check` is everything the four tabs would have told you about the
inventory being wrong, written to stdout without opening anything. It is an
[App report](../../tui.md#app-reports): a boolean flag on the app's own
command, declared in the registry beside it.

It runs, in this order:

1. Every inventory file that would not parse, and every manifest with no
   template. These come first because everything below is computed from what did
   parse, so a rule failing underneath may be a consequence rather than a
   fault of its own.
2. [`validate`](inventory.md#validation), rule by rule.
3. The secrets store against the paths the inventory implies, in both
   directions, each named individually — a missing path is `secret sync`'s to
   generate, and an orphaned one is a person's to remove or rename. Equal
   counts of the two say so, because that pattern is a rename rather than a
   pair of unrelated faults.

A clean inventory is one line naming what was read. A report that printed
the whole inventory when nothing is wrong is a report nobody reads the top
of. Anything found is one line each and a non-zero exit, so it fits a
pre-commit hook or a shell loop over several roots.

The count is the error's own message rather than a line of the report: the
report is stdout and the error is stderr, and printing it in both puts the
same sentence in two places.

`dgs conf --targets` is the other report: what the generator root holds,
grouped by node, per
[export.md](export.md#targets-and-selectors).

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
2. **The six views, read-only.** Node, user, service, instance, port,
   secret, values unread, cross-linked. No graph yet. Every index but Secrets is a
   tree, folds with the File Explorer's keys, and hides its line numbers, per
   [`scroll-lists.md`](../../scroll-lists.md#line-numbers).
3. **Revealing a secret.** `Enter` on a secret view, the same warning
   `export`'s preview already carries.
4. **Editing.** `e` on a secret view, the three rules above.
5. **The graph.** `internal/webgraph` — the embedded page, the vendored
   library, the JSON contract — and the mapping from this inventory onto it.
   Force-directed, grouped by node, keyed by kind.
