# Configuration: Device network inventory

Status: implemented. This is the only network inventory format. There is no
historical-format reader, upgrade path or configuration-conversion tool. Actual
inventory edits are a separate operator task; loading never rewrites files.

The [configuration reference](../../configuration/conf.md) lists the keys and
defaults. Rhumb owns parsing, reference resolution, validation and derivation;
dgs displays their results. The [implementation plan](network-inventory-plan.md)
records the delivery boundaries and acceptance checks.

## Purpose

Keep device network facts independent of deployment. A host may have multiple
interfaces and MACs, each interface may have multiple named IPs, and devices
without deployed services belong in the same inventory. IP-only and domain-only
connection targets are both valid. Private interface IPs and public ingress are
different facts.

## Ownership and layers

| Object | Owns | Boundary |
| --- | --- | --- |
| Network | Scope, preference, optional subnet and gateway | Not physical wiring or a new routing-table model |
| Host | Stable identity of a physical machine, VM or appliance | Not identified by a MAC |
| Interface | Local stable identifier, optional MAC | Not necessarily an OS interface name |
| Address | Named literal IP and network scope | Never a domain name |
| Endpoint | Explicit connection target in one scope | Reference to an address, or literal IP/domain; not a local ownership claim |
| Node | Deployment identity, optional host reference, runtime and instances | No authored network addresses or MACs |
| Instance | Service, protocol, ports, listeners and publication | Existing transport/application contracts remain |

DHCP and DNS are application protocols consuming these facts. Reservations
associate interface identities with IPv4 addresses; device DNS declarations
associate names with IPs. They do not maintain another copy of an IP.
`links.yaml` still represents sessions between instances, not L2 wiring.

## Files

```text
conf/
├── networks.yaml
├── hosts.yaml
├── names.yaml              # optional DNS aliases
├── nodes/<group>/*.yaml
├── services/
├── routes.yaml
├── links.yaml
└── users.yaml
```

Optional `names.yaml` holds alias-to-target names without network fields.
There is no separate `dhcp.yaml` or backend-specific `dns.yaml`. Product-specific settings such as
lease durations and DNS forwarding remain with the appropriate service instance.

## Configuration

```yaml
# hosts.yaml
hosts:
  host-a:
    interfaces:
      wired:
        mac: "02:00:00:00:00:11"
        addresses:
          home-v4:
            network: home
            ip: 10.0.0.10
            dhcp_reservation: true
            names: [host-a.home.test]
          extra-v4:
            network: home
            ip: 10.0.0.12
      wifi:
        mac: "02:00:00:00:00:12"
        addresses:
          home-v4:
            network: home
            ip: 10.0.0.11
            dhcp_reservation: true
            names: [host-a-wifi.home.test]
    endpoints:
      home:
        from: {interface: wired, address: home-v4}
      internet:
        address: host-a.example.net
    reaches: [backend]
  host-b:
    endpoints:
      internet:
        address: host-b.example.net
```

```yaml
# nodes/example/node-1.yaml
id: node-1
host: host-a
# Existing runtime, instances and deployment settings follow.
```

`home`, `internet` and `backend` must be declared network scopes. Nodes without
network facts may omit `host`; they cannot author `networks` or `reaches`.
Host/interface/address keys start with an ASCII letter or digit and continue
with letters, digits, hyphens or underscores.

Omitted maps/lists are empty. MAC is optional, reservations default to false,
and names default to empty. A host can contain only endpoints, only interface
facts, or MAC-only interfaces. No endpoint or interface address is selected
implicitly. Host binding is explicit and one-to-one, even when identifiers match.

## References and behavior

References have fixed depth: Node → Host; Endpoint → Interface → Address.
Endpoint `from` and literal `address` are mutually exclusive. A reference must
name both the interface and address in the same host and network scope.
Endpoints cannot reference other endpoints or DNS records.

An address edit changes the one owned IP; its endpoint, DNS and reservation
consumers follow. Switching an endpoint reference changes neither interface IP.
Literal endpoints do not follow string matches. No DNS lookup occurs during
loading, checking or rendering. Interface facts do not automatically publish
endpoints or infer additional reachability.

The existing network preference, explicit `reaches`, `universal`, same-node,
container and link rules continue to choose connection targets. Effective
`Node.Networks` and `Node.Reaches` are computed Go values, not YAML fields.
Invalid references produce diagnostics and no partial effective address set.

## Independent consumers

- DHCP rows require explicit reservation opt-in, a valid MAC and IPv4. One
  MAC/network reserves at most one IPv4; other static IPs remain valid. Rows
  always identify `host/interface/address`, are address-ordered and retain typed
  source identities. Invalid inventory emits no reservation rows.
- Address `names` requests interface DNS records for that particular IP and scope. A host
  identifier is not automatically a DNS name, and names are not attached to all
  interfaces. Service `ports.published` remains with services and preserves
  proxy-ingress derivation. Domain endpoints generate CNAMEs for published service names; endpoints alone
  do not request records.
- Local bind/publication behavior stays with existing instance/deployment rules.
  Public ingress does not prove local IP ownership; new interface data does not
  automatically change container bindings or widen listeners.

IP ownership is counted per address declaration and scope. MAC ownership is
counted per participating interface and scope, so several IPs on one interface
are not duplicate MAC owners. References never create another owner. Subnet,
reference and DNS conflict checks report the authored sources.

## DNS layers

The DNS chain is service name → device name → interface name → IP. Interface
names stay in address `names`; host-level `names` maps a device DNS name to a
list of local `{interface, address}` references or `{endpoint: network}`
references for public access endpoints:

```yaml
# Inside hosts.host-a:
names:
  host-a.dev.example.com:
    - {interface: wired, address: home-v4}
```

Each device name selects at most one target per network. An endpoint reference
uses the endpoint's network and gives a stable device name without inventing
interface facts. Domain endpoints produce CNAMEs, IP endpoints A/AAAA; local
endpoint references preserve the interface-name chain when present. Mixed
reference forms and missing endpoints are errors. Network renames update
endpoint-name references too. It targets the first
name on that address, which must exist. A service's published name uses the
device name matching its answering host's endpoint reference; multiple matching
device names fail validation, while no match uses the endpoint IP. Proxy routes
use the proxy host. Domain endpoints generate CNAMEs for published service names; IP endpoints
generate A/AAAA. No additional host-level name is required.

```yaml
# Optional generator-root names.yaml:
names:
  nas.example.com: host-a.dev.example.com
```

Aliases follow targets into every network containing them, including alias
chains. Targets absent everywhere, duplicate names (even across record types),
missing internal targets, cycles and chains beyond 8 CNAME hops are errors.
Explicit domain endpoints can terminate outside the inventory. `flatnames`
refuses such chains without querying DNS.
DNS names compare case-insensitively with a final dot ignored.

Templates choose their backend: `names "home"` returns `.Name`, `.Type`, `.Value`
and `.Source` records (`A`, `AAAA`, `CNAME`); `flatnames "home"` resolves chains to
IP records for hosts-only backends. Container tables remain scoped to their own
node and contain fixed IP records. `dgs conf dns <network>` exports generic `{network, records}` JSON with
`name`, `type`, `value`, `source` fields. External tools choose provider-specific
settings and apply records. No live DNS queries, backend writes or historical
format adapters are part of this feature.

Inventory YAML contains one mapping document; extra documents and null roots
are rejected. Root and nested merges retain YAML override precedence while
rejecting effective null records. Both raw `names` and `flatnames` report DNS
input/graph errors instead of exporting a partial table; templates that never
read either function can still render independently.

## Existing machine-move workflow

The existing Migrate page continues to support machine moves, with IP, MAC,
endpoint and network-scope edits targeting their actual Host fields. It does not
convert configuration formats or extract network inventory from node files.
Saved plans use one current schema and record the source host. Reports are bound
to input digests; changed inputs refuse apply. Replacements are loaded and
validated in a temporary tree before writes, with existing backup and handled
rollback behavior. This is not multi-file crash atomicity.

## Completion boundary

The delivery covers the single format, reference resolution, validation, DHCP
listing, DNS integration, inspection, machine-move adaptation, examples and tests.
It does not include a configuration migration/conversion tool or compatibility
adapters. It does not apply OS settings or update real DHCP/DNS servers.

No VLAN/switch/bond topology, dynamic discovery, IP allocation, automatic
failover, DHCPv6, full dual-stack support or general DNS RRsets are introduced.
Multiple stored addresses do not imply multi-answer DNS or load balancing.
