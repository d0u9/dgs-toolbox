# Configuration: Node migration (proposal)

This design covers moving
the configuration represented by one node to a replacement machine, including
changes to its name and network addresses. It does not say that a generated
file has been installed or that DNS has been changed.

The [inventory](inventory.md) is the source of node, instance, route, network
and published-name facts. [Export](export.md) renders files from those facts;
it does not deploy them. A migration should use those existing readers and
renderers, not keep a second set of rules for how addresses or services depend
on one another.

## The question it answers

For a move such as `network-4` to `network-8`, the operator needs to know:

1. Which inventory files must change, and what exactly will be written?
2. Which rendered files change, on which nodes, and which instances need to be
   installed or restarted there?
3. Which names might need a DNS update, what old and new addresses do they
   suggest, and what cannot be decided from the inventory?
4. In what order can the new machine be prepared, checked, and cut over, with
   a way back if a step fails?

A node's address is written in `nodes/<group>/*.yaml`; route edges and
container port mappings are derived. A port's `published` name is a service
name, not necessarily the node's hostname or a DNS record managed here.

## Proposed command shape

The `dgs conf` TUI has a **Migrate** tab. It starts with **New migration** and
the saved plans in `<config_dir>/conf/migrations/`, plus **Open migration file**
through the shared File Explorer. Its path input and root navigation can reach
any accessible directory. A new migration selects a
source node, then opens a
two-column table grouped into Node, Networks, Instances, Published, Routes, and
Secrets. Secret rows show implied relative paths before and after the proposed
changes. They are read-only because the paths are derived from instance IDs,
grants, and service declarations; a path change belongs in the report's manual
secret-move checklist, not as an independent inventory edit.
Shared anchored section dividers separate the groups without becoming selectable
rows. Each indented editable row names a node ID, network name, address, authored instance ID,
published hostname, or route name, with its current and proposed value indented
under it. Up/Down or a mouse click selects a row; section headings are not
editable. Enter edits the proposed value with the
shared text control, Enter applies that edit, and Esc cancels it. `s` saves the
plan; the first save opens File Explorer to select any destination directory,
then asks for a filename. `S` opens the same Save As flow for a new location.
An opened plan saves back to its original path with `s`, including `.yml` files.
Existing plans open into the same table and can be edited and saved again. `r` validates
the whole proposal and opens the report; Esc returns to the table, where every
value remains editable; clicking a row also returns from the report to that
row. `p` from the report opens File Explorer for a destination directory, then
asks for a report filename and creates a
report file without overwriting an existing one. Leaving the tab never writes
inventory, secrets or DNS. The plan file is versioned YAML containing the
source node and each row's original and proposed values. Opening checks every
original value against the current inventory; a stale plan is refused. Saving
an open plan also refuses to replace a file changed outside this session.
Unsaved edits are marked in the status bar; leaving the table asks for a
second Esc before discarding them.

The TUI gives the migration table one third of the workspace width and the
detail or report pane two thirds, with the table capped at 90 cells. The report
pane scrolls independently of the table. `[` and `]` still switch tabs when
no text editor is open.

`dgs conf migrate node` opens a terminal wizard. It asks for the source node,
the new node ID, each network's new name and address, each authored instance's
new ID, each published port's hostname, and each route touching that node's
new name. Enter keeps the shown
value. EOF cancels. The wizard validates the proposed inventory in memory and
prints an upgrade report with changed references, rendered target impact,
secret-path changes, a per-node deployment/handoff list, and an operator
checklist. It can save that report to a
path supplied at the final prompt, creating the file only if it does not
already exist. It does not modify inventory or secrets. Supplying `--node`
keeps the noninteractive read-only preview; `--network`, `--instance` and
`--route` can be repeated with explicit `from` and `to` fields. `--published`
names an old instance and port plus the desired hostname. The report flags
published-name changes for DNS review; it never edits external DNS.

An instance rename updates its authored ID and route-hop references in the
preview. A route rename updates the route key and typed access lists in users,
credentials and node profiles. Values, deploy settings and templates remain
opaque, and the report marks them for manual review. A changed instance ID
can change secret paths; the report lists those before anyone edits files.

The first interface is a CLI action, separate from `conf inspect` and
`conf export`:

```text
dgs conf migrate node \
  --node 'from=network-4,to=network-8' \
  --network 'from=home,address=10.0.1.8'
```

When the network itself is renamed as well, name that separately from its new
address:

```text
dgs conf migrate node \
  --node 'from=a-node-group-02-01,to=a-node-group-03-01' \
  --network 'from=network-4,to=network-8,address=10.0.1.8' \
  --network 'from=tailnet,address=10.0.2.8'
```

Each `--network` is one complete change, so several networks cannot be paired
in the wrong order. `from` is required; `to` defaults to `from` for an
address-only change, and `address` may be omitted for a name-only change.
Network renames are inventory-wide: `networks.yaml`, every node's
`networks` and `reaches`, and every credential's `reaches` must agree. It is
never inferred from the node ID, since another node or credential may also
use that network. The preview lists each typed reference it would change.

The first milestone implements only the read-only route-edge preview. Later,
without `--apply`, it only prints a full plan. `--apply` prints the same plan and
asks before changing local inventory files; `--yes` answers that prompt for a
script. Each `from` names an existing inventory network on the old node.
Leaving a network
out retains its address. Explicit address removal, adding a network, and moving
instances between two concurrently existing nodes are outside the first
version; they require a separate design because they can change reachability
or ownership, not just an address. The old ID must identify exactly one valid
node and the new ID must not already be in use.

No new `dgs-config.json` key is needed. The command reads `conf.root`; it reads
`conf.secrets` only when comparing rendered output. Its plan remains useful if
the secrets root is unavailable, but it must mark render comparisons that it
could not perform.

The command changes the node's `id`, its named `networks` addresses, and
structured references that the inventory schema explicitly defines. It does
not search and replace arbitrary text. In particular, an instance ID, route
name, credential, service `values`, `deploy` value, template, filename and
`published` name are not inferred from a node ID. If any of them contains
`network-4` or an old address, the plan lists the exact file and field for review.
An instance keeps its ID unless a separate instance rename is requested. A
node filename need not equal its ID; leaving its path intact avoids an
unnecessary second change. The plan still shows that path clearly.

## Plan calculation

1. Load the current inventory and service manifests. Report broken files and
   validation failures before planning. Capture a digest of every file the
   plan may edit so an old preview cannot silently apply to a changed tree.
2. Apply the requested edits to an in-memory copy of the inventory. Derive and
   validate both snapshots with the same code used by `conf --check`. Refuse a
   target state with broken references or unresolved edges.
3. Compare stable facts by identity: authored instances and their nodes;
   derived client targets; route edges and resolved address/network/port;
   grants and implied secret paths; and container port mappings. Show each
   change with its reason, such as `route home: network-4 -> network-8, home address
   10.0.1.4 -> 10.0.1.8`.
4. Enumerate all export targets in both snapshots. Render both versions when
   the required secrets are available and compare output paths, modes and
   bytes. The authoritative deployment list is the union of targets with
   changed output, targets removed by the migration, and targets newly
   created. A changed derived target ID may change its export path even when
   its bytes are equal, so path changes count. Secrets must never appear in
   plan output or a textual diff; show the output path and a digest only.
5. When rendering is impossible, derive a conservative candidate list from
   changed target identity, node facts, resolved edges, grants, mappings and
   the instances they feed. Mark it `render not compared`, not `unchanged`.
   Templates can read opaque `values` and `deploy` fields, so dependency facts
   alone are not proof that a rendered file is identical.

The report groups changed outputs by destination node and service/instance.
It distinguishes server runtime configuration, container deployment files,
and client or link exports. It also lists unchanged targets, but only when a
render comparison proves them unchanged. A service with no deployment template
still appears when its runtime configuration changes; the operator must know
where and how that service is installed.

### DNS review

For every `published` name on a port hosted by the changed node, show the name,
port and hosting instance, plus the old and new addresses for each changed
network. Include the node's own old/new ID as a hostname *candidate* only if
it is a DNS name in the inventory; an arbitrary node ID is not evidence of a
DNS record. Also scan typed configuration values and template files for exact
old ID/address occurrences and list them as manual review items, never as
automatically understood DNS records.

The inventory has no DNS zone, record type, provider, TTL, CNAME chain,
split-horizon view, proxy/CDN state or current authoritative answer. The
command therefore does not write DNS. It cannot claim that a `published` name
points directly to either node, or that changing an A/AAAA record is needed.
The checklist says `inspect this name's authoritative records; update the
relevant view if it resolves to the old endpoint; verify from the networks
clients use`. If the old address is private, its `home` network context is
shown rather than treating it as a public DNS target. A name hosted by a
reverse proxy may need a change to the proxy's record, while its backend's
`published` name stays the same; the plan must identify the actual ingress
node from the route and label the DNS step accordingly.

## Execution and rollback

`--apply` edits only local inventory files. Before writing, it rechecks the
captured file digests and rebuilds the plan. It writes complete replacement
files in their existing directories, then atomically renames them into place.
Because several file renames are not one filesystem transaction, it records
which edits succeeded and retains exact backups until the resulting inventory
loads and validates. On failure, it restores the original bytes and reports
any restore failure with paths. Re-running the same migration after success
should report that the requested target state is already present; it must not
create another replacement or alter unrelated fields.

The command does not move or generate secrets automatically. A changed node ID
can change derived client target IDs and export paths; whether it changes any
secret path must be measured by comparing `secretstore.ImpliedPaths` before and
after. If paths differ, list the exact old/new paths and stop before applying
until a separate, explicit secret move has a defined implementation. The
inventory design currently notes that `dgs conf secret mv` is not implemented.

The human rollout checklist should separate preparation from cutover:

1. Prepare `network-8` with the intended addresses and services; keep `network-4`
   available while verifying the replacement.
2. Apply the inventory edit, run `dgs conf --check`, and export the changed
   targets. Review the rendered output and copy/install it on each listed node.
3. Restart or reload each listed service using its own deployment procedure;
   test its listening ports and the route edges that changed.
4. Inspect and update the listed DNS records where necessary, then verify
   answers and service reachability from the relevant networks. DNS caching
   means the old endpoint may still receive traffic during cutover.
5. Remove `network-4` only after the new endpoint works and old traffic has
   drained. The command never performs this step.

Rollback means restoring the saved inventory bytes, exporting the old targets
again, redeploying those files, and reverting any separately changed DNS.
The report must name the old target paths and node/service pairs required for
that rollback. A local inventory rollback alone does not roll back deployed
machines or DNS.

## Example of the report

```text
Node migration: network-4 -> network-8
Inventory: nodes/home/network-4.yaml
Address: home 10.0.1.4 -> 10.0.1.8

Local edits
  nodes/home/network-4.yaml: id, networks.home

Export and deploy
  network-8: microbin/bin-home: configuration and compose.yaml changed
  laptop: ssclient/laptop-home: upstream address changed
  alex-phone: ssclient/phone-home: upstream address changed

DNS review
  bin.example.net: published by bin-home:web; inspect its public DNS target
  network-4.example.net: possible node hostname; confirm whether a record exists

Manual review
  services/microbin/deploy/defaults.yaml: old address appears in an opaque value

Checks after installation
  Verify bin-home:web from home and internet as applicable
  Verify client routes whose resolved edge changed
```

The names and paths in this example are illustrative. Each real report names
only evidence found in that generator root, and says when a check is a
candidate rather than a known change.

## Acceptance cases for implementation

- An IP-only move changes a remote client's resolved edge and rendered file,
  but leaves an unrelated service off the deployment list.
- A same-node hop retains loopback when the node's address changes.
- A changed container bind mapping appears in the deployment list even when
  the runtime configuration does not change.
- A node rename changes derived client target paths; unchanged instance IDs
  and unchanged secret paths remain untouched.
- A `published` name behind a proxy yields a DNS review item with the ingress
  node, without claiming that the backend's address is its DNS answer.
- An opaque template value containing the old IP is flagged for review and
  never rewritten by a global text replacement.
- A broken target state, missing secret required for comparison, stale plan,
  failed write, and repeated apply each have an explicit, tested outcome.

## Proposed milestones

Implementation status: the read-only CLI preview and interactive report wizard
are implemented. The preview compares each target with the existing export renderer, including
deployment files, and reports changed export paths, content or modes. It also
compares implied secret paths. If `conf.secrets` is not configured, the preview
marks all render comparisons unavailable. With a configured secrets root,
each target is rendered separately; only a target whose render fails is marked
unavailable. It lists literal old node IDs, network names and
changed addresses found in service files for manual review; it does not rewrite
those opaque values. It also flags authored instance IDs containing an old
node or network name. An `export path` line means only the bundle location
changes; it explicitly says whether the rendered bytes changed. The wizard
adds a per-node deployment/handoff list and a basic ordered checklist.
Authoritative DNS review and guarded local apply are still planned.

Service reference matches are labelled `注释` when the line begins with `#`
after whitespace, and `配置/模板内容` otherwise. This label describes the line
in the source file; it does not decide whether a comment should be updated.

1. **Read-only node plan.** Accept the old/new node IDs and repeatable per-network
   addresses. Load and validate both inventory snapshots, show the exact
   structured edits, including an explicit network rename and its references,
   and changed route edges; reject collisions or
   unreachable target states. No files are written. Acceptance: an IP-only
   move names the affected edges; a same-node hop stays on loopback.
2. **Service and export impact.** Compare old/new targets, secret paths,
   container mappings and rendered output where secrets are available. Group
   changed files by destination node, service and instance, with a reason and
   a `render not compared` marker when applicable. Acceptance: an unrelated
   service is absent, while a changed mapping or derived target path appears.
3. **DNS and manual review.** List `published` names on affected ingress
   nodes, old/new address candidates and exact old ID/IP occurrences in
   opaque fields or templates. Distinguish confirmed inventory facts from
   DNS checks requiring an authoritative lookup. Acceptance: a proxied
   backend does not produce an asserted DNS change to its private address.
4. **Guarded local apply and rollback.** Add `--apply`, stale-plan checks,
   same-directory replacement files, exact backups, post-write validation
   and restoration after partial failure. Block when implied secret paths
   would change until secret moves have their own implementation. Acceptance:
   repeated apply is harmless, a changed input invalidates a preview, and
   injected write failure restores the original inventory.
5. **Operator checklist.** Print preparation, per-node export/install/restart,
   DNS review, route checks and rollback work in rollout order. Reuse `conf
   export` selectors or name exact targets; do not run deployment or DNS
   commands. Acceptance: the checklist names every changed target and no
   unchanged target as required work, and says when render comparison was
   unavailable.

Each milestone has filesystem tests using a small inventory with local and
remote hops, a container mapping and a published service name. Stop after
the requested milestone is verified; applying local edits belongs only to
milestone 4.

## Decisions to confirm before implementation

- Should the first version support only replacing one node, or also moving a
  subset of instances between two nodes that both remain active?
- Should DNS stay as a review checklist, or should a later design introduce a
  DNS record inventory with ownership and authoritative readback?
- Should a node file and its instance directory be renamed when their basename
  equals the old node ID? The proposed first version keeps paths stable.
- What is the desired cutover policy when a node rename changes secret paths?
  The first version blocks apply and reports them until secret moves exist.
