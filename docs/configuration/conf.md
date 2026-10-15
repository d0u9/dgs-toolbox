# Conf configuration

`<config dir>/conf/config.json`:

```json
{
  "root": "~/confgen",
  "secrets": "~/confgen-secrets",
  "services": "~/confgen-deploy",
  "install_root": "/srv/example",
  "label_prefix": "example",
  "tool": "example",
  "export": {
    "dir": "~/Exports"
  }
}
```

| Key | Meaning | Default |
| --- | --- | --- |
| `root` | The directory holding `services/` and the inventory files. | empty — the TUI page opens with no root and asks for one |
| `secrets` | The root of the secrets tree, one file per credential. | empty — an instance needing a secret refuses to render |
| `services` | A directory of deploy service definitions, one `<service>.yaml` each, consulted before the built-in ones when a bundle is built. The same as `rhumb deploy build --services`. | empty — only the built-in definitions |
| `install_root` | Where a Linux host bundle is installed when its instance writes no `deploy.dir`: `<install_root>/<service>`. A macOS bundle with no `deploy.dir` stays where it was unpacked. The same as `rhumb deploy build --install-root`. | empty — `/srv/rhumb` |
| `label_prefix` | The start of the name the service manager knows an instance by, `<label_prefix>.<node>.<instance>`: the systemd unit and the launchd label and plist. Letters, digits, `-` and `_`. The same as `rhumb deploy build --label-prefix`. | empty — `rhumb` |
| `tool` | Tool name in ctl comments, shim ownership markers and the capitalized `<Tool>Bundle` plist key. Letters, digits, `-` and `_`. Use the same name for build and bundle-gc. | empty — `dgs` |
| `export.dir` | Where the destination form opens, for a folder or an archive. | empty — the home directory |

Paths follow the same rules as [`cred`](cred.md#paths): a leading `~` is the
home directory, `$NAME` and `${NAME}` are environment variables, and the
result must be absolute.

What `dgs conf export` does with these is in
[`apps/conf/export.md`](https://github.com/d0u9/rhumb/blob/master/docs/export.md).

Device interfaces, addresses and connection endpoints are described in
[Device network inventory](../apps/conf/network-inventory.md) and the
[rhumb reference](https://github.com/d0u9/rhumb/blob/master/docs/inventory.md#device-network-inventory).
They live in the generator root, not this `config.json`. Node `host` is an
optional explicit binding; a node without it has no declared network facts.
Inline node `networks`/`reaches` are not accepted.

| Inventory key | Meaning | Default |
| --- | --- | --- |
| Node `host` | Host ID in `hosts.yaml`; excludes inline `networks`/`reaches` | absent — no host network facts |
| Host `interfaces` | Stable interface IDs and their facts | empty |
| Interface `mac` | Hardware address, normalized to colon form | empty |
| Interface `addresses` | Named IP declarations | empty |
| Address `network`, `ip` | Declared network scope and unscoped literal IP | required |
| Address `dhcp_reservation` | Explicit MAC–IPv4 reservation | `false` |
| Address `names` | Interface DNS names pointing at this IP; first name is the device CNAME target | empty |
| Host `names` | Device DNS name → local `{interface, address}` or `{endpoint: network}` refs; at most one per network | empty |
| `names.yaml` → `names` | Alias → DNS target, included in each network where the target exists | optional file, empty map |
| Host `endpoints` | Explicit connection targets by network | empty |
| Endpoint `from` / `address` | Exact interface/address reference or literal IP/domain; choose one | none selected |
| Host `reaches` | Additional outbound network scopes | empty |

`names "<network>"` exposes `Name`, `Type`, `Value`, `Source` records, including
CNAMEs. `flatnames "<network>"` resolves them to IP records. Names are ASCII,
case-insensitive, with an optional final dot. Duplicate names, missing targets,
cycles and chains beyond 8 CNAME hops fail validation. Published names use A/AAAA for an IP endpoint and CNAME for a domain endpoint.
External endpoint domains are permitted terminals; `flatnames` cannot resolve
them without IP declarations. See [DNS layers](../apps/conf/network-inventory.md#dns-layers).

Public devices can reference their existing endpoint in host `names`:
`host-a.dev.example.test: [{endpoint: internet}]`. Published services then point
to that device name, which points to the domain or IP in `endpoints.internet`.
Each name reference chooses one form; endpoint refs and interface refs cannot
both select the same network for one name.

A host supports multiple interfaces/MACs and named addresses. Every reservation
row identifies `host/interface/address`. There is one inventory format; there is
no conversion tool, and loading never rewrites configuration. Existing machine
moves edit the current Host source fields.


`services`, `install_root`, `label_prefix` and `tool` are read by every way to a
bundle: `dgs conf bundle <export-dir>`, which is `rhumb deploy build`;
`dgs conf export --bundle`; and the inspect page's Bundle format. On the first
two, `--services`, `--install-root`, `--label-prefix` and `--tool` override them for one
run; `--services` on `dgs conf inspect` does too.

`dgs conf bundle-gc` finds launchd entries under `label_prefix`, or its own
`--label-prefix`: run it with the prefix the bundles were built with. A bundle
of the same instance built under another prefix is rebuilt only when replacing
was confirmed — `--overwrite`, or Replace on the inspect page — and a macOS
bundle never is: it is installed where it is, so `ctl uninstall` it first.

Bundle ownership defaults to `dgs`, independently of the label prefix and
install root. There is no compatibility with old `rhumb-bundle` shim markers
or `RhumbBundle` plist keys. On each machine, in this order:

1. Run `ctl unlink` with the **old** ctl, or delete its old shims by hand.
   The new `ctl link` refuses to replace a shim with another tool's marker.
2. Replace the bundle with one rebuilt under the new tool name.
3. Run the new `ctl link` once. It writes new shims and, on macOS, renames
   the bundle key in existing plists; nothing else in them changes, and the
   running service is left alone.

New unlink and bundle-gc recognize only the configured tool. Do the same when
changing `tool` later.

### DNS JSON export

`dgs conf dns <network>` prints `{network, records}` to stdout. Records are
sorted by name and use `name`, `type`, `value`, `source`. Services keep their
domains in `ports.published`; a node's host endpoint determines the target,
including existing proxy-ingress selection. Multiple published service names
can point at one domain or IP. Identical records requested by several ports
are merged. A published name equal to the canonical access domain produces no
new record, avoiding a self-CNAME. Invalid inventory emits no JSON.

This is a desired-record export for your external DNS tool. It does not call
Cloudflare or other providers, query external DNS, infer zone ownership, or
specify TTL/proxy settings. Ordinary aliases still need an authored target.
