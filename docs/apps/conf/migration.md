# Configuration: Node migration (proposal)

This design covers moving
the configuration represented by one node to a new location or machine, including
changes to its name and network addresses. It does not say that a generated
file has been installed or that DNS has been changed.

The [inventory](https://github.com/d0u9/rhumb/blob/master/docs/inventory.md) is the source of node, instance, route, network
and published-name facts. [Export](https://github.com/d0u9/rhumb/blob/master/docs/export.md) renders files from those facts;
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
4. In what order can the machine be shut down, moved, checked, and brought up, with
   a way back if a step fails?

A migration is one of two scenarios, chosen when it starts because the
inventory cannot tell them apart:

- **Relocate** — the same machine moves to a new place, for example
  `network-4` → `network-8`. Its disks, Docker volumes and bind paths come along.
- **Replace** — the node moves onto a new machine that starts empty, for
  example `network-4-linux-01` → `network-4-linux-02`. Data travels as an archive.

The scenario shapes only the report's procedure; the inventory edits are the
same. Export handoffs to people or other nodes are not services to stop on
the moved machine.

A node's address is written in `nodes/<group>/*.yaml`; route edges and
container port mappings are derived. A port's `published` name is a service
name, not necessarily the node's hostname or a DNS record managed here.

## Interface

Migration lives only in the `dgs conf` TUI; there is no command-line action.
The TUI has a **Migrate** tab. It starts with **New migration** and
the saved plans in `<config_dir>/conf/migrations/`, plus **Open migration file**
through the shared File Explorer. Its path input and root navigation can reach
any accessible directory. A new migration first asks for the scenario
(relocate or replace), then selects a source node, then opens a
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
report file without overwriting an existing one. `a` from the report asks for
confirmation; `y` applies the plan (see Execution and rollback) and appends the
result, including the backup directory, to the report. Any other key cancels.
Nothing else in the tab writes inventory or secrets, and nothing writes DNS.
The plan file is versioned YAML containing the source node, the scenario and
each row's original and proposed values. A plan saved without a scenario asks
for one when it is opened. Opening checks every
original value against the current inventory; a stale plan is refused. Saving
an open plan also refuses to replace a file changed outside this session.
Unsaved edits are marked in the status bar; leaving the table asks for a
second Esc before discarding them.

The TUI gives the migration table one third of the workspace width and the
detail or report pane two thirds, with the table capped at 90 cells. The report
pane scrolls independently of the table. Up/Down and `k`/`j` move one report
line; `g`/`G` move to its top/bottom. The mouse wheel scrolls the pane under
the pointer: table rows on the left, report lines on the right. In report mode,
`c` or `y` copies the complete report through the terminal clipboard (OSC 52);
`p` writes it to a file. `[` and `]` still switch tabs when no text editor is
open.

The report is Markdown in three parts: **Changes** (inventory edits with the
typed references they rewrite, route edges, rendered output per destination
machine, secret paths), **Needs attention** (numbered items the operator must
decide), and **Procedure**, the scenario's steps in six phases:

1. Take services offline on the old host. Each instance's container mounts
   are recorded with `docker inspect` before its generated `uninstall.sh`
   runs; jobs stop before the services they read. A compose service under a
   `profiles` entry runs only on demand and has no container, so it is not
   inspected; a mount only it declares is named for backup by hand.
2. Back up data. Every mount is archived under its path inside the
   container, which is stable across hosts. A size check comes first, since a
   bind mount may be large shared storage the operator chooses not to carry.
3. Manual work: the numbered attention items, network configuration, apply
   in the TUI, `dgs conf --check`, and exporting the new bundle.
4. Import data. Replace only: `install.sh` creates the new container, which
   is stopped at once, and each archive is unpacked into the mount with the
   same container path. Relocate imports nothing.
5. Start services, listening services before jobs; then redeploy changed
   targets on other machines.
6. Verify containers, logs, each address a route dials, and DNS answers.

A rollback section closes the procedure. Commands are printed for the
operator to run; the TUI runs none of them. A compose service without
`container_name` is found by its `com.docker.compose.service` label.
Instances without a generated lifecycle script are listed for manual
handling.

An instance's key is `<node>/<id>`, so a node rename rewrites every route hop
and dial that names one of the node's instances, on any node; a dial written
without its node stays that way. An instance rename updates its authored ID and
route-hop and dial references in the preview. A route rename updates the route key and typed access lists in users,
credentials and node profiles. `--route from=home/samba,to=nas` keeps the
route in its scope; `to=<scope>/<name>` moves it to another scope, or to the
top level with no scope. A node rename moves the node file to `<new-id>.yaml` beside it, since a
node's id is its file's name (rule 36); its instance directory keeps its name.
It also renames the scope key of routes scoped
to it, and every grant naming one. Values, deploy settings and templates remain
opaque, and the report marks them for manual review. A changed instance ID
can change secret paths; the report lists those before anyone edits files.

Each network change is one row pair: its name and its address on the moved
node. Network renames are inventory-wide: `networks.yaml`, `hosts.yaml`, every
node's `networks` and `reaches`, and every credential's `reaches` must agree.
A rename is never inferred from the node ID, since another node or credential
may also use that network. The report lists each typed reference it would
change.

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

The read-only report now lists every affected `published` name even when its
text stays unchanged. It names the hosting service/port and the first hop of
each route using that port as the known ingress, before and after migration.
Addresses are shown with their inventory network name and labelled as ingress
or backend candidates. A proxied backend's private address is never presented
as a confirmed DNS answer. Each item asks for an authoritative lookup and
verification from the networks clients use.

## Execution and rollback

Apply edits local inventory files and copies secrets whose instance keys
change: every instance of a renamed node, and each explicitly renamed instance. Before writing, it rechecks the
captured file digests and rebuilds the plan. It writes complete replacement
files in their existing directories, then atomically renames them into place.
Because several file renames are not one filesystem transaction, it records
which edits succeeded and retains exact backups until the resulting inventory
loads and validates. On failure, it restores the original bytes and reports
any restore failure with paths. Re-running the same migration after success
should report that the requested target state is already present; it must not
create another replacement or alter unrelated fields.

The implementation saves originals with their permissions under a unique
`.dgs-migration-backup-*` directory in the generator root and reports that
path after success. It retains this directory for operator rollback. Each
replacement is staged in its destination directory. YAML comments and
unrelated typed fields survive the edit; opaque `values`, `deploy`, templates
and file basenames are left for manual review. Input files are checked again
after confirmation, and the same validated plan is rebuilt before any
replacement. A failed write or failed post-write load and validation restores
every file already replaced. This is filesystem API level recovery, not a
claim of power-loss durability across several renames.

For a node rename or an explicit instance rename, apply copies each implied
secret to the new `<node>/<id>` path before replacing inventory files. The
preview renders a renamed node's targets with the secrets still at their old
paths. It checks source bytes,
rejects a different destination value, and reads back each new file. The old
files remain for rollback; a retry accepts matching copies left by a partial
attempt. Added or removed secret paths without a one-to-one instance rename
still block apply. `dgs conf secret mv` remains unimplemented.
While the old paths are retained, `dgs conf --check` reports them as orphaned
and exits unsuccessfully. The operator must match those reports against the
reviewed copy list and investigate every other problem. Retire old paths only
after the new services work and rollback is no longer needed.

The human rollout checklist should separate preparation from cutover:

1. Review the old-node shutdown list and save the running state and configuration
   needed for rollback. Stop the listed runtime services before relocating the
   physical machine.
2. Move the machine and configure its new networks. Apply the inventory edit,
   run `dgs conf --check`, and export the changed
   targets. Review the rendered output and copy/install it on each listed node.
3. Start each instance in the new-node startup list using its deployment procedure;
   test its listening ports and the route edges that changed.
4. Inspect and update the listed DNS records where necessary, then verify
   answers and service reachability from the relevant networks. DNS caching
   means the old endpoint may still receive traffic during cutover.
5. Remove the old `network-4` logical identity only after the new endpoint works
   and old traffic has drained. The command never performs this step.

Rollback means restoring the saved inventory bytes, exporting the old targets
again, redeploying those files, and reverting any separately changed DNS.
The report must name the old target paths and node/service pairs required for
that rollback. A local inventory rollback alone does not roll back deployed
machines or DNS.

## Example of the report

An abridged relocation report:

```markdown
# Node migration: `network-4` → `network-8`

## 1. Changes
| Change | Before | After |
|---|---|---|
| Node ID | `network-4` | `network-8` |
| Address on home | `10.0.1.4` | `10.0.1.8` |

#### `network-8`
| Instance | File | Change |
|---|---|---|
| `microbin/bin-home` | `compose.yaml` | bundle path moves; **content changed** |

## 2. Needs attention
### 2.1 DNS records to inspect
**`bin.example.net`** — `microbin/bin-home` on port `web`

## 3. Procedure
### Phase 1 — Take services offline (old host)
### Phase 2 — Back up data (old host)
### Phase 3 — Manual work
### Phase 4 — Import data (new host)
### Phase 5 — Start services (new host)
### Phase 6 — Verify
### Rollback
```

The names in this example are illustrative. Each real report names only
evidence found in that generator root, and says when a check is a candidate
rather than a known change.

## Acceptance cases for implementation

- An IP-only move changes a remote client's resolved edge and rendered file,
  but leaves an unrelated service off the deployment list.
- A same-node hop retains loopback when the node's address changes.
- A changed container bind mapping appears in the deployment list even when
  the runtime configuration does not change.
- A node rename changes derived client target paths and every secret path of
  the node's instances; instance IDs remain untouched.
- A `published` name behind a proxy yields a DNS review item with the ingress
  node, without claiming that the backend's address is its DNS answer.
- An opaque template value containing the old IP is flagged for review and
  never rewritten by a global text replacement.
- A broken target state, missing secret required for comparison, stale plan,
  failed write, and repeated apply each have an explicit, tested outcome.

## Proposed milestones

Implementation status: the Migrate tab, its report, guarded local apply and
the DNS and manual review items are implemented. The report compares each
target with the existing export renderer, including deployment files, and
reports changed export paths, content or modes. If `conf.secrets` is not
configured, render comparisons and container mounts are marked unavailable.
It lists literal old node IDs, network names and changed addresses found in
service and node files for manual review, leaving out node-file lines whose
match is a typed field the plan already covers; it never rewrites them. It
also flags authored instance IDs containing an old node or network name.
The report does not query or edit external DNS, and deployment and DNS
commands remain operator work.

Text matches are labelled `注释` when the line begins with `#` after
whitespace, and `配置/模板内容` otherwise. This label describes the line in the
source file; it does not decide whether a comment should be updated.

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
4. **Guarded local apply and rollback.** Add apply, stale-plan checks,
   same-directory replacement files, exact backups, post-write validation
   and restoration after partial failure. Copy verified secrets for explicit
   instance renames and retain the old paths. Block ambiguous changes. Acceptance:
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
- Should old secret paths be retired after a verified cutover? The current
  implementation retains them for rollback and never deletes them automatically.
