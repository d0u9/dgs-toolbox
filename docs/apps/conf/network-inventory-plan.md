# Device network inventory: implementation plan

Status: implemented. Scope follows the [design](network-inventory.md): one current
format, no historical compatibility and no configuration conversion tool.
Examples and tests use synthetic data; real configuration is handled separately.

## 1. Model and readers

- `rhumb/inventory/host.go` defines interfaces, named addresses and endpoints.
- `hosts.yaml` owns device network facts; node `host` binds them explicitly.
- Node network/reachability maps are computed only. Removed inline node network
  fields and flat host network/address/MAC/name fields are rejected as unknown.
- Strict readers reject null records, unknown keys and ambiguous endpoints.
- Source identities are tuples of host/interface/address, never array offsets.

Exit: multi-interface/multi-IP, IP-only, domain-only, MAC-only and unbound-host
cases load; malformed or missing references do not expose partial addresses.

## 2. Validation and derivation

- Validate scopes, IPs, MACs, ownership, bindings and exact endpoint references.
- Count MAC ownership once per interface/network; count IP ownership once per
  address declaration. Reject multiple reservations for one MAC/network.
- Generate reservations only on explicit opt-in, using stable
  `host/interface/address` identifiers for every row.
- Keep interface names on IP declarations; host-level device names reference
  these addresses. Published service names use matching device names and retain
  existing proxy ingress selection, falling back to IP without a device name.
- Load optional `names.yaml` aliases. Derive backend-neutral records and expose
  raw `names` plus IP-only `flatnames`. Validate same-network targets, duplicate
  names, reference cardinality, cycles and the 8-hop limit.
- Preserve the distinction between local interface addresses and public ingress;
  do not infer listeners, routes, failover or DNS balancing.

Exit: conflict diagnostics identify owners; invalid reservations emit no output;
network targets, DNS and publication pass their behavioral tests.

## 3. Existing consumers

- Adapt node template values, routes/dials, links, inspection and graph data to
  resolved host facts. A bound host/node appears once; unbound hosts stay visible.
- Keep the existing machine-move workflow pointed at actual Host fields. IP edits,
  MAC edits, literal endpoints and endpoint selection remain distinct intents.
- Keep one saved-plan schema. Retain stale-report protection, staged validation,
  backup and handled rollback for ordinary machine moves.
- Do not add extraction rows, conversion actions, generated-interface adapters,
  compatibility readers or absent-host-file creation as a migration feature.

Exit: source edits change the intended data; changed inputs refuse application;
existing machine moves continue to work on the current model.

## 4. Examples, fixtures and reference

- Author all shipped examples and synthetic fixture inventories in the single
  format. Test builders accept typed current Host values, not legacy YAML.
- Update the rhumb inventory/export reference and dgs configuration reference and
  index together, including defaults.
- Keep the design and examples free of real device addresses or identities.

## Acceptance matrix

| Case | Required result |
| --- | --- |
| Two interfaces/MACs on one network | Independent IPs/reservations, explicit endpoint selection |
| One interface with multiple IPs | Valid static addresses; no duplicate-MAC error |
| One MAC/network reserves two IPs | Rejected before output |
| Different interfaces duplicate a MAC or address | Owner-specific conflict |
| Equivalent IPv6 bind/gateway spellings | Same address is recognized before conflict checking |
| Same-family wildcard and specific listener | Conflict for the same host port/protocol; TCP and UDP remain separate |
| Process members share a fixed container IP | One runtime owner; no conflict with their derived process instance |
| Process members disagree on fixed/dynamic container addresses | Rejected as inconsistent process configuration |
| Host interface records the container bridge gateway | Check and migration preview remain valid; overlapping prefixes alone are not an error |
| Independent instance conflicts with a process that has several members | One address-conflict report, regardless of member ordering |
| Node rename with a shared container process and no routes file | Preview, staged validation, apply and export agree; member references use the new node ID |
| Process member rename, alone or with a node rename | Process, route and dial references follow the renamed member; original snapshots stay unchanged; apply and export succeed |
| Equivalent endpoint IPs plus a distinct endpoint IP | Exported manifest contains normalized, deduplicated bindings and retains the distinct address |
| MAC-only interface | No invented IP, DNS record or reservation |
| IP-only device | Valid without names or reservation |
| Domain-only endpoint | Optional device name references endpoint; services CNAME to device then external domain; no invented interface or lookup |
| Private IP plus public ingress | Distinct facts; no inferred local bind |
| Endpoint switches to another interface/address | Source IPs and reservations unchanged |
| Named IP changes | References, device DNS and reservation follow |
| Service behind a proxy | DNS continues to follow ingress derivation |
| Device/service DNS conflict | Existing conflict rule identifies both sources |
| One device name across two networks | Per-network interface CNAME targets; no scope leakage |
| Generic DNS export | Deterministic JSON; domain endpoint → CNAME, IP → A/AAAA; invalid input emits nothing |
| Alias chain | Inherits target scopes; `flatnames` resolves every name to its terminal IP |
| Missing target, cycle or ninth CNAME hop | Rejected by Check; flattening produces no partial table |
| Bound host/node | One owner and one machine representation |
| Removed fields or unknown nested keys | Rejected, never adapted |
| Root merges with null records / multiple YAML documents | Rejected without silently discarding declarations |
| Invalid DNS read by a template | Render/export fails without partial output; unrelated templates still work |
| Scoped edit with shared aliases or merge values | Other hosts and comments preserved |
| Endpoint edit plus network rename | First apply succeeds; retry changes no files |
| Inputs change after reviewed report | Apply refuses without overwriting changes |
| Handled machine-move failure | Exact originals restored within existing boundary |

## Validation and stop condition

Run focused tests, then `go test ./...` in both repositories using the existing
`go.work`. Dgs native/socket tests require the appropriate execution permissions;
do not skip them to make the suite pass. Example load/render tests are included.
Review diffs and generated artifacts for private data.

Stop after the current model, consumers, examples and checks pass. Configuration
conversion, deployment and DHCP/DNS product integration are not part of this work.
