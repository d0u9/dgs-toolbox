# Configuration: Inventory

The inventory is the set of facts `dgs conf` renders from: which machines exist,
which services run on them, who is allowed to reach what, and which chains relay
through which. [`export.md`](export.md) describes the renderer and what it
writes. This page describes what it reads.

## Why it exists

Before the inventory, a service's values file carried three different kinds of
information at once:

```yaml
port: 1080                    # the shape of this service's configuration
server: 203.0.113.30          # where the upstream is
secret_server: exit-jp        # which credential opens it
```

Only the first line belongs to the service. The other two are facts about the
world, copied by hand into every values file that needs them, and copied again
into the server on the other end. Three things follow from that, and they are
what this page removes:

- **An address is written in many places.** A machine changing address means
  finding every values file that names it.
- **A credential is written twice and must match.** Adding a person means one
  edit on the server and one on their device, with the password typed into both.
- **Nothing knows what runs where.** Which instances share a machine can only be
  guessed from their names.

The inventory states each of these once. The renderer derives the rest.

## The model

| Term | What it is |
| --- | --- |
| **node** | A machine or device: a server, a home server, a laptop, a phone. |
| **network** | A named network that nodes belong to, such as `home` or `internet`. |
| **service** | A kind of software, with templates and defaults: `shadowsocks-rust`, `hysteria2`, `microbin`. |
| **role** | One form of a service — `server`, or a named client program — selecting a template, a defaults file and an output name. |
| **instance** | One service running on one node, and one rendered configuration file. |
| **port** | A named listening port of an instance. Every port an instance listens on is named; an instance that listens on nothing has none. |
| **route** | An ordered list of hops, from where traffic enters to where it leaves. |
| **hop** | One step of a route, written `<instance>:<port>`. |
| **user** | A logical identity: a person, not a Linux account and not a password. |
| **grant** | A credential for one party to reach one port. Derived, never written. |
| **secret** | One credential value, one file. |

Two terms describe things the reader never writes. A **grant** is derived from a
user's access list or from a route's hops; a **principal** is whatever holds one
— a node, an upstream instance, or a user. They are named here because the
secrets layout and the rendered user tables are organised by them.

`connection` is deliberately absent. In an earlier design a connection was
something written by hand for every client reaching every server, which is the
N×M table this model exists to avoid. What remains of it is the route, which is
written once per chain, and the grant, which is derived.

## Fixed names and example names

Everything on this page is written in terms of one worked inventory, and most of
the words in it are that inventory's, not the model's. An implementation that
recognises `shadowsocks-rust`, `server` or `internet` has hard-coded someone's
data.

**Fixed.** `dgs` knows these and nothing else:

| What | The names |
| --- | --- |
| Files | `services/`, `nodes/`, `users.yaml`, `routes.yaml`, `networks.yaml`, `confgen.yaml`, `defaults.yaml` |
| Node keys | `id`, `networks`, `reaches`, `owner`, `client_role`, `instances` |
| Instance keys | `id`, `service`, `role`, `ports`, `bind`, `values` |
| User keys | `username`, `devices`, `client_role`, `access` |
| Route keys | `hops` |
| Network keys | `networks`, `universal` |
| Manifest keys | `secret`, `roles`, and a role's `auth`, `reached_by`, `template`, `defaults`, `output`, `rotation`, `combine_own` |
| Enumerated values | `devices: unmanaged`; `auth: per-principal`, `shared`, `none`; `defaults: document`, `element`; `client_role: none`; `rotation: disruptive` |
| Reserved words | `own`, as a port name and as a secrets path segment |
| Secrets path segments | `node`, `instance`, `user`, `own`, and the `.previous` suffix |
| Selector fields | `node`, `user`, `service`, `role`, `instance`, `route` |
| The loopback address | `127.0.0.1`, for two hops on one node |

**An example.** Every one of these is a name the inventory's author chose, and
the model attaches no meaning to any particular spelling:

| What | As written here |
| --- | --- |
| Service names | `shadowsocks-rust`, `hysteria2`, `microbin`, `httpproxy` |
| Role names | `server`, `ss-rust`, `shadowrocket`, `sing-box`, `link` |
| Network names | `home`, `internet` |
| Port names | `main`, `alt`, `proxy`, `local`, `web`, `socks`, `http` |
| Node, instance, user and route identifiers | `us-sfo-dgo-linux-01`, `ss-sfo01`, `doug`, `jp` |
| Names of an instance's own secrets | `auth_password`, `server_password` |
| Everything inside `values` | `masquerade`, `fast_open`, `public_path` — a template's own, not dgs's |
| Output file names | `config.json`, `share.txt` |

`server` is in the second table and not the first. No rule keys off it: a role
authenticates because it says `auth: per-principal`, and is reached because it
says `reached_by`, not because of what it is called. The same is true of
`internet`, which is the universal network only because `networks.yaml` says so.

## Files

```text
~/confgen/                    conf.root
├── services/<service>/       confgen.yaml, templates, defaults
├── nodes/*.yaml              a machine, its networks, its instances
├── users.yaml                people and what they may reach
├── routes.yaml               chains
└── networks.yaml             network preference order

~/confgen-secrets/            conf.secrets
```

The secrets tree is a separate root rather than a directory inside `conf.root`.
A directory that is not under the repository cannot be committed by accident,
which a `.gitignore` entry only promises.

Everything except the secrets tree belongs in version control. Reviewing a
change to `users.yaml` is the audit: it says, in one diff, who gained or lost
access to what. The secret values are random bytes with no history worth
keeping — see [Secrets](#secrets).

## Nodes

A node is a machine or a device. Servers and phones are the same kind of thing
here, which is why the word is `node` rather than `host`.

```yaml
# nodes/us-sfo-dgo-linux-01.yaml

id: us-sfo-dgo-linux-01

networks:
  internet: 203.0.113.10

instances:
  - id: ss-sfo01
    service: shadowsocks-rust
    role: server
    ports:
      main: 38250
      alt: 49217

  - id: hy2-sfo01
    service: hysteria2
    role: server
    ports:
      main: 443
```

Instances live in the node file rather than in a directory of their own. The
question asked most often is "what runs on this machine", the unit of an export
is a machine, and a node file answers both by being read top to bottom. A node
carrying dozens of instances can be split into a directory later; that is a
layout change and not a model change.

A machine that both serves and reaches out is not a special kind of node. The
home server below answers requests on one instance, relays through a client
instance beside it, and hosts a service of its own:

```yaml
# nodes/home-server.yaml

id: home-server

networks:
  home: 192.168.1.10

instances:
  - id: http-home
    service: httpproxy
    role: server
    bind: "0.0.0.0"
    ports:
      proxy: 8118

  - id: ss-home
    service: shadowsocks-rust
    role: ss-rust
    bind: 127.0.0.1
    ports:
      local: 1080

  - id: bin-home
    service: microbin
    role: server
    bind: "0.0.0.0"
    ports:
      web: 8080
```

It has no address on `internet`, and does not need one: it reaches out through
NAT, and nothing connects to it from outside. `networks` says where a node can
be reached, not where it can reach — see
[below](#networks-and-how-an-address-is-chosen).

A node hosting no services needs only three lines:

```yaml
# nodes/macbook.yaml

id: macbook
owner: doug
reaches: [home]
```

`owner` names the user whose devices this is. It appears on client devices and
nowhere else. A node that hosts services has no owner.

A device has no `networks` at all: nothing connects *to* a phone, so it has no
address to advertise. `reaches` is how it says it can use the home network when
it is there.

### Networks and how an address is chosen

```yaml
# networks.yaml

networks: [home, internet]
universal: internet
```

`networks` is a preference order, most preferred first. `universal` names the one
network every node can reach without saying so, and may be omitted, in which case
nothing is implicit and every node states what it reaches.

Neither name is known to `dgs`. `home` and `internet` are this inventory's
names, and the rule that one network is reachable from everywhere is expressed
by `universal` naming it rather than by the renderer recognising a word.

### Reached, and reaching

A node says two different things about networks, and NAT is why they cannot be
one field:

- **`networks`** maps a network to this node's address on it. It says where
  others can reach this node.
- **`reaches`** lists networks this node can open a connection on. It says
  nothing about being reachable.

A node reaches every network it has an address on, and every node reaches the
`universal` network, so `reaches` is written only for a network a node can use
but has no address on: a phone on the home LAN, or a laptop there. A home server
behind NAT writes neither — its `home` address covers the LAN, and the universal
network is implicit.

### Choosing an address

The address at either end of a hop is never written down. It is chosen from the
two nodes involved:

| The two ends | The address used |
| --- | --- |
| The same node | `127.0.0.1` and the downstream port |
| The downstream has an address on a network the upstream reaches | That address, on the first such network in preference order |
| Neither | An error naming both nodes and what each one reaches |

The rule is one-directional on purpose. A client reaching a server needs the
server to be reachable and the client only to be able to reach; requiring both
to be addressable would rule out every machine behind NAT, which is most of
them.

A machine changing address is one line in one node file.

A mobile device is not a special case, because "at home" and "away" are not two
addresses for one route — they are two routes. A phone at home reaches the
internet through the home server; away, it reaches a server directly. Those are
different chains, the person picks one in their client, and each renders its own
configuration. The model states which chains exist; which one to use at a given
moment is a decision no file can make.

## Instances and ports

An instance is one service running on one node, and one rendered configuration
file. Its identifier is global: hops name instances with no node to qualify
them.

The identifier is free text and nothing parses it. It is written in two places
only — a route's hops, and a selector — and there are few routes, so a longer
name costs almost nothing while an ambiguous one costs a rename of every secret
path under it.

### Naming

The convention is `<service>-<site><index>`, where the site and index are the
part of the node's name that tells it apart from its neighbours:

```text
us-sfo-dgo-linux-01   →   ss-sfo01, hy2-sfo01, bin-sfo01
jp-tyo-dgo-linux-01   →   hy2-tyo01
home-server           →   http-home, ss-home, bin-home
```

A node with no index gives instances with no index, and a site that grows a
second machine gets its index from the node that already carries one.

Two instances of one service on one node — which is rare, since several ports of
one service are one instance with several ports — take a purpose suffix:
`ss-sfo01-pub` beside `ss-sfo01-fam`.

The site codes are IATA, which are unique worldwide: no two airports share one,
and airport and city codes are assigned from the same namespace, so `sfo` and
`nrt` cannot mean two places. That uniqueness is worth having, and it holds only
while one scheme is used throughout. Three things break it:

- **Mixing schemes.** An invented abbreviation for a place with no airport is
  registered nowhere, so two of them will eventually collide. A site near an
  airport takes that code with a suffix — `sfo-home` — rather than a new code.
- **Cloud region names.** `us-east-1`, `nyc3` and `ap-northeast` mean different
  places at different providers and are not comparable between them. They are
  not site codes.
- **Reading, not collision.** `SJC` and `SJO` are distinct codes for San José in
  California and in Costa Rica, and city names repeat across countries where the
  codes do not.

Node names keep their country prefix, which is redundant for uniqueness and
worth its three characters for reading a list of machines. Instance names drop
it, since the site code is already unique.

None of this is enforced, and it cannot be: no checker knows which scheme a name
came from. What is enforced is that instance identifiers are unique, so the cost
of getting a name wrong is an error that names both instances, not a
configuration that renders two things over each other.

### Ports

**Every port an instance listens on is named**, including the only one:

```yaml
  - id: hy2-tyo01
    service: hysteria2
    role: server
    ports:
      main: 443
```

There is no scalar `port:` shorthand. The alternative — a shorthand whose secret
paths are one level shallower — makes adding a second port to an existing
instance a migration of every secret file under it, where a silently missed file
is regenerated as a new random value and breaks the peer that still holds the
old one. One extra line per instance buys one shape everywhere: in hops, in
secret paths, in validation and in the renderer.

An instance that listens on nothing has no `ports` at all. A phone client that
runs as a system tunnel has no local port to declare, and inventing one would be
a number in a file that nothing reads. The argument for naming ports is about
secret paths splitting when a second port appears, and an instance with no ports
has no secret paths under it to split.

`bind` is optional and defaults from the role's defaults file. A server binds
every interface; a client's local listener binds loopback.

A port named `own` is rejected; see [Secrets](#secrets).

### An instance's own values

`id`, `service`, `role`, `bind` and `ports` are what dgs itself needs: enough to
place an instance, find its template, and resolve every address and secret
around it. They are not enough to configure a service — Hysteria2 needs a
masquerade target, Shadowsocks a `fast_open` flag, an nginx server block
whatever an nginx server block needs — and none of that is a network fact, a
listening port, or shared by every instance of a role. `values` is where it
goes:

```yaml
  - id: bin-sfo01
    service: microbin
    role: server
    bind: 127.0.0.1
    ports:
      web: 8080
    values:
      public_path: https://clip.matrix-au.org/
      upload_enabled: true
```

`values` is opaque: dgs parses it as YAML and nothing more. It does not
validate its shape, does not know what any key means, and does not merge it
with anything — a template reads it by name, the same way it reads `defaults`.
This is deliberate, not a gap left for later: a masquerade target is Hysteria2's
concept, not the inventory's, and adding a typed field for every service's own
parameters would mean this page growing a section per service forever. What
belongs in the inventory is what routing, address resolution and secrets need to
work; what one instance of one service is configured to do belongs to that
service, expressed however its template wants it.

**A value that does not vary between instances of a role belongs in the role's
`defaults.yaml` instead.** A masquerade target chosen once and reused by every
Hysteria2 server is a default, not a value — `values` is for what a second
instance of the same role would need to say differently, MicroBin's public path
being the clear case: two MicroBin instances cannot share one.

An authored override on a client node carries `values` the same way it carries
`ports` and `bind`; see [what is derived](#what-is-derived).

## Users

```yaml
# users.yaml

users:
  doug:
    access: [jp, sfo, home-sfo, bin, nas]

  friend-a:
    username: yak
    devices: unmanaged
    client_role: link
    access: [jp, sfo]
```

`access` lists routes. `username` is the account name the services see, and
defaults to the user's own identifier, so it is written only where the two
differ.

### Managed and unmanaged devices

By default a user's devices are **managed**: each is a node file, and each holds
its own credential. Losing one device means deleting one secret and re-rendering
the servers it reached; the person's other devices keep working.

A user marked `devices: unmanaged` has no node files at all. One credential is
issued per route entry, and what the person copies it onto is not modelled —
because it cannot be. This is the case of handing a configuration to someone
else: how many devices they use it on is not a fact the inventory can hold, and
inventing node files for devices nobody tracks would make it lie.

The difference shows up in three places:

| | Managed | Unmanaged |
| --- | --- | --- |
| Secret | One per device | One per person |
| Rendered account name | `doug-macbook` | `yak` |
| Export | One bundle per device | One bundle per person |

An unmanaged user declares no networks, and reaches only the `universal` one. A
route granted to an unmanaged user whose entry has no address there is an error.

An unmanaged user also has no node to carry a `client_role`, so it is written on
the user instead, and means the same thing. `link` is usually the answer: which
program someone else runs is not a fact the inventory can hold, and a share URI
is the one form nearly every program imports.

## Routes

```yaml
# routes.yaml

routes:
  jp:
    hops: [hy2-tyo01:main]

  sfo:
    hops: [ss-sfo01:main]

  home-sfo:
    hops: [http-home:proxy, ss-home:local, ss-sfo01:alt]

  bin:
    hops: [bin-sfo01:web]
```

Each adjacent pair of hops is one edge. `home-sfo` says traffic enters the home
server's HTTP proxy, passes to the Shadowsocks client on the same machine, and
leaves through the `alt` port in San Francisco. Two of those hops share a node
and one does not, and nothing in the file says so — the address rules above
settle it.

A route is also the unit a person is granted. That is deliberate: inserting a
relay into the middle of a chain changes the route's hops and changes nothing in
`users.yaml`.

A one-hop route is an ordinary route. Reaching a service directly is not a
special form.

### Naming

A route's name says what the person is choosing when they pick it in their
client, because that is the only place a route name is ever read by someone who
is not editing these files.

Two things therefore stay out of the name:

- **The machine.** A route named `sfo` may enter `ss-sfo01` today and `ss-sfo02`
  after that machine is replaced. Putting the instance in the name throws away
  the indirection that lets a chain be rebuilt without touching `users.yaml`.
- **The protocol**, unless the protocol is the choice. A line that happens to run
  over Hysteria2 is an implementation detail, and a route called `hy2` stops
  meaning anything once a second one exists. A line deliberately offered
  *alongside* the same exit over another protocol — one for speed, one for a bad
  network — is a real choice, and then it belongs in the name: `sfo-ss` beside
  `sfo-hy2`.

Where two things vary, the name carries both. `home-sfo` names an exit and the
fact that it passes through the home server, which are independent of each
other.

Proxy exits and internal services share one namespace — `jp` beside `nas` —
because the person picks from one list and does not care which kind a name is.
That is one axis used deliberately, not two axes mixed by accident, which is
what a `sfo` beside a `hy2` would be.

**A route is optional.** An instance nothing relays through and nobody is
granted — a web service reached from a browser, a port open only on the home
network — is a complete, valid instance with no route naming it. Nothing warns
about it.

### One upstream per instance

An instance forwards to one place, so a non-terminal hop must have the same
successor in every route that passes through it. Two routes sending `ss-home`
to different upstreams cannot both be rendered, and the error says so by name.

That constraint *is* rule-based routing — choosing an upstream per request by
domain or region — and it is not supported. When it is, a route will describe
which upstreams an instance can reach rather than which one it does, and the
rule set itself will stay in the instance's own configuration rather than
becoming an inventory concept. The error message names this so the reader knows
it is a boundary and not a mistake.

## What is derived

Nothing below is written by hand.

**Client instances.** A managed user's device and each route they are granted
produce an instance named `<node>-<route>`; an unmanaged user produces
`<user>-<route>`. Its service is the entry hop's service, and its values come
from its own role's defaults.

Its role is not the entry hop's role — an entry hop is a `server`, and what
reaches it is not another server.

### Which role a client derives as

Roles are named after the client program, because the program is what differs:
Shadowrocket and a `shadowsocks-rust` command line read configurations with
nothing in common, while an iPhone and an iPad running the same app read the
same one. A role named after a platform would split two devices that need one
file and join two that need different ones.

```yaml
roles:
  server:
    auth: per-principal
    reached_by: ss-rust
  ss-rust:
    auth: none
    template: templates/ss-rust.json.tmpl
    output: config.json
  shadowrocket:
    auth: none
    template: templates/shadowrocket.conf.tmpl
    output: shadowrocket.conf
  link:
    auth: none
    template: templates/link.tmpl
    output: share.txt
```

A node says which one it runs, and every derived client on it takes that role:

```yaml
# nodes/phone.yaml
client_role: shadowrocket
```

It is a property of the device rather than of each instance, because a device
runs one client program for everything it proxies. A device using different
programs for different services cannot be expressed; when that happens
`client_role` becomes a map from service to role, and nothing else changes.

The role is resolved in this order:

1. The node's `client_role`, or for an unmanaged user, the user's.
2. The entry hop's role's `reached_by`.
3. Neither: nothing is derived.

A `client_role` naming a role a service does not have is an error naming the
service and listing the roles it does have. It does **not** fall back to
`reached_by`: falling back would hand a phone a configuration written for a
command-line client, and the failure would happen on the device rather than
here. Services a device derives nothing from — one whose entry role has no
`reached_by` — are not consulted, so they cannot raise this error.

**A role with no `reached_by` derives nothing.** MicroBin is reached from a
browser: there is no client configuration to render, and a route entering it
still produces a grant and its shared secret, but no file for the person's
device. The address and the password reach them some other way, which is the
truth of how such a service is used and not something to invent a file for.

### When the client program is unknown

Two answers, and both already exist in the shapes above.

A **`link` role** renders the protocol's own share URI — `ss://`, `hysteria2://`
— to a text file. Nearly every client imports one, so one role per service
covers every program nobody has written a template for. It is the sensible
default for an unmanaged user, whose client program is not a thing the inventory
can know.

**`client_role: none`** says this device renders nothing. The grant is still
derived and the server still carries the account; only the file is absent, for a
machine configured by hand.

A client instance's ports are its own local listeners — the SOCKS and HTTP ports
an application on that device connects to. Nothing reaches a phone from outside,
so they are never hop targets, and they carry no secrets. Their names are read
by the template, which needs to know which listener is which; the address and
port at the far end are not written here, because the route derives them.

A derived instance is a default, not a fixed result. Authoring an instance with
the same identifier pins what it names and leaves the rest derived:

```yaml
# nodes/phone.yaml

id: phone
owner: doug
reaches: [home]

instances:
  - id: phone-sfo
    ports:
      socks: 10080
      http: 18080
```

Everything not written stays derived: the service, the role, `bind`, and every
value the role's defaults supply. An override that replaced the whole instance
would mean copying a client configuration out in full to change one port, which
is the copying this page exists to remove.

`service` and `role` may not be written in an override. Both follow from the
route's entry hop and its `reached_by`, so a line naming them is either
redundant or a contradiction.

Written and derived instances share one namespace, so the uniqueness check
covers both.

**Grants.** One per (principal, port) pair: one for each managed device granted
a route, one for each unmanaged user granted a route, and one for each
non-terminal hop reaching the hop after it.

**Server account tables.** The principals holding a grant on a port, rendered as
accounts. People and machines are listed together, because to the server they
are the same thing:

```text
ss-sfo01, port main:
  doug-macbook
  doug-phone
  yak

ss-sfo01, port alt:
  ss-home
```

Two ports of one instance carry two independent tables and two independent sets
of credentials.

## Secrets

One credential is one file. The path is the identity:

```text
<instance>/<port>/<kind>/<name>
```

| Segment | What it says |
| --- | --- |
| `<instance>` | Which instance the credential opens |
| `<port>` | Which port of it — ports have separate account tables |
| `<kind>` | What holds it: `node`, `instance` or `user` |
| `<name>` | Which one |

```text
~/confgen-secrets/
├── ss-sfo01/
│   ├── main/node/macbook
│   ├── main/node/phone
│   ├── main/user/friend-a
│   └── alt/instance/ss-home
├── hy2-tyo01/
│   └── main/
│       ├── node/macbook
│       ├── node/phone
│       └── user/friend-a
└── bin-sfo01/
    └── own/auth_password
```

`own` is the exception, and sits at the instance level rather than under a port:
a TLS private key or an administrative password belongs to the whole instance,
not to one of its listeners. An instance may have several. `own` is therefore a
reserved port name.

The layout is grouped by what consumes the secrets, because rendering a server
reads every credential for one instance at once. Reading it the other way works
without decrypting anything: `ls` a port to see which devices can reach it, or
grep the tree for a node name to see what one device holds.

There is no index and no identifier to assign. A file's path is derived from the
same facts the renderer derives everything else from, so the set of files that
should exist is computable, which is what makes `secret sync` possible.

Nothing groups an instance's files under its node. Node membership can change —
an instance moving to a different machine is a rename, per
[naming](#naming) — and a physical directory encoding it would have to move
along, the same fragility a route name avoids by not naming a machine.
Everything scoped to one physical machine — every instance it hosts — is a
selector, `node:us-sfo-dgo-linux-01`, answered by querying the inventory, not by
where files sit.

### Editing without `dgs`

The layout needs no tool to read or change: every value is a plain file, so a
machine with no `dgs` on it still works with what is already there.

```bash
# every credential ss-sfo01 holds, without decrypting anything special
find ss-sfo01 -type f -exec sh -c 'echo "== $1 =="; cat "$1"' _ {} \;

# change one
printf 'new-value' > ss-sfo01/main/node/macbook

# who can reach it, with nothing decrypted
ls -R ss-sfo01
```

This is not a fallback bolted on afterward — it is what one file per credential
buys, the same reason age and SOPS-based setups converge on it: a value
readable and writable with `cat` and a redirect outlives any tool built to read
it. Whatever `dgs` grows to make this more convenient, `find`, `cat` and
`printf` keep working.

### Viewing and editing several at once

Seeing every credential an instance holds today means opening each file in
turn; filling in several at once during a migration means the same, once per
value. `dgs conf secret show <instance>` and `dgs conf secret edit <instance>`
are for that:

- **`show`** reads everything under an instance — every port, every principal,
  its own secrets — and prints it as one structured block, generated on
  demand and never written anywhere.
- **`edit`** does the same, into a temporary file opened in `$EDITOR`. On save,
  each value that changed is written back to its own file, one write per
  changed leaf — editing several credentials in one pass still touches only the
  files that actually changed. A key added in the editor that names no path the
  inventory implies is refused: a name comes from `users.yaml` or a node file,
  never from typing it into an editor session, so `sync` cannot see it as an
  invented path with no history. A key removed in the editor is left alone —
  deleting a credential is `secret sync`'s and a person's own decision, the
  same way `sync` itself never deletes.

Both take a `node:`, `user:` or `route:` selector too, covering every instance
it matches — the query [above](#secrets) answers "everything on this
machine", these answer "let me see and change it."

Neither exists yet. Spawning an editor is a write from outside the read-only
reports [`tui.md`](../../tui.md#app-reports) describes, the same open question
[`export.md`](export.md#open-questions) already raises for a command-line
export — both wait on the same decision about how a file-writing invocation
fits the command model, rather than each inventing its own answer to it.

### How a service says what it needs

A role declares whether its inbound side authenticates:

```yaml
# services/shadowsocks-rust/confgen.yaml

secret:
  kind: base64
  bytes: 32

roles:
  server:
    auth: per-principal
    reached_by: ss-rust
    template: templates/server.json.tmpl
    defaults: element
    output: config.json
  ss-rust:
    auth: none
    template: templates/ss-rust.json.tmpl
    defaults: element
    output: config.json
```

| `auth` | What is generated |
| --- | --- |
| `per-principal` | One secret per principal reaching each port, and a rendered account table |
| `shared` | One `own` secret for the instance, embedded in every client that reaches it |
| `none` | Nothing |

`auth` is per role rather than per service because a client role's local
listener is usually not the service's own protocol at all: a Shadowsocks
client's loopback SOCKS port authenticates nobody, while the server role of the
same service authenticates everybody.

`secret` stays at the service level, because the format is the protocol's:
Shadowsocks 2022 needs base64 of exactly the key size, not an arbitrary
password. A service that does not declare one gets a printable random string.

### A shared identity alongside a principal's own

Some protocols give every principal its own secret and still need one more
value shared by all of them. Shadowsocks 2022's server holds one PSK for the
whole port, and each user's effective password is that PSK and their own,
joined: no client can connect from its own secret alone.

```yaml
roles:
  server:
    auth: per-principal
    combine_own: psk
```

`combine_own` names one of the role's own secrets — `<instance>/own/psk` — that
`secret sync` generates and keeps alongside the per-principal ones. Every client
deriving from this role receives it as `upstream.own`, next to `upstream.secret`
for its own principal value; see
[the render context](#the-render-context). A role with no `combine_own` gives
clients nothing beyond their own secret, which is the ordinary case.

Sync generates a missing `combine_own` file the same as any implied path. It
does not report one orphaned once a role stops declaring it: an `own/` path
sync did not compute an identity for and one it used to are, on disk alone,
the same fact, so both are left for a person to remove by hand.

### Keeping the tree in step

`dgs conf secret sync` compares the paths the inventory implies against the
files on disk. Missing paths are generated. Paths on disk that the inventory no
longer implies are **reported and never deleted**, because sync cannot tell a
rename from a removal, and the failure mode of guessing wrong is silent: a new
random value on one side and the old one still on the other.

Renaming a node or an instance is therefore an explicit move,
`dgs conf secret mv <old> <new>`, and sync says so when the number of new paths
and orphaned paths match.

Sync also reports which servers need re-rendering. **Adding a device is a change
on the server side**, not only a bundle handed to its owner: a new device means
a new credential, and every instance it reaches must be rendered again and
deployed before the device can connect.

### Rotation

A secret file may have a sibling named `<name>.previous`. A role rendering an
account table emits both values as two accounts; a client renders only the
current one. That makes a rotation three ordered steps with no disconnection:

1. Write the new value, move the old one to `.previous`, render and deploy the
   server.
2. Render and deploy the clients.
3. Delete `.previous`, render and deploy the server again.

The overlap is a second working credential, so it is a debt and not a state.
`validate` reports a `.previous` file older than seven days as an error.

A service whose template cannot emit two accounts for one principal declares
`rotation: disruptive` on the role, and rotating it says up front that the
connection will drop.

### What is not protected

The secrets tree is plaintext. Losing it loses the credentials themselves:
they are random values held nowhere else, so the recovery is a rotation of
everything, not a restore. Keeping a copy is outside what `dgs` does.

## The render context

The inventory reaches templates as a structured context, and that context is the
contract between this page and every template. Changing its shape breaks every
template at once, so it is written down here and changes to it are breaking
changes.

```yaml
node:        the node this instance runs on: id, networks
instance:    id, service, role, ports, bind, and the instance's own values
upstream:    the next hop, resolved: address, port, its secret, and — when the
             next hop's role declares combine_own — that shared secret too,
             as own — absent for a terminal instance
principals:  for a port with auth: per-principal, the account name and secret of
             everything holding a grant on it
own:         the instance's own secrets, by name
target:      service, role and instance names
```

The context is structured rather than one deep-merged mapping so that a key
present at two levels cannot silently overwrite another, and so a template
failing can be told which level it was reading.

## Validation

`dgs conf validate` checks:

1. Node identifiers are unique; instance identifiers are unique, derived ones
   included. `universal`, if written, names a network in the list.
2. Every `instance.service` names a service, and every `instance.role` names one
   of its roles. A `reached_by` names another role of the same service.
3. A `client_role`, on a node or on an unmanaged user, names a role held by
   every service its granted routes derive a client for, or is `none`. The error
   lists the roles that service has. A managed user does not carry one, and an
   unmanaged user has no node to carry one.
4. Every hop names an existing instance and an existing port on it.
5. No port is named `own`.
6. Every route named in an `access` list exists; every user named by an `owner`
   exists.
7. An instance authored on a client node overrides a derived one: its identifier
   matches an instance the user's access derives, so a misspelled route name
   cannot leave an override that nothing reads, and it sets neither `service`
   nor `role`.
8. A non-terminal hop has the same successor in every route through it. The
   error names rule-based routing as the reason this is not allowed yet.
9. No route names one instance twice.
10. Every edge resolves an address: the two ends are on one node, or the
    downstream has an address on a network the upstream reaches.
11. A route in an `access` list has an entry the granted user's devices can
    reach, on a port not bound to loopback only.
12. An unmanaged user's granted routes enter on the `universal` network.
13. Account names rendered for one port are distinct.
14. Two instances on one node do not bind the same address, port and protocol.
15. Every secret the inventory implies exists, and every file in the secrets
    tree is implied by it. Both directions are reported; neither is fixed here.
16. No `.previous` file is older than seven days.

## Boundaries

- **Rule-based routing** is not supported; see
  [One upstream per instance](#one-upstream-per-instance).
- **Per-device subsets of a user's access** are not expressible: every managed
  device of a user receives every route the user is granted. When it is wanted,
  an optional `access` on the node overrides the user's list, and nothing else
  in the model changes.
- **Deployment** is not here. An export writes files; copying them to a machine
  and restarting the service is a different problem, with different failure
  modes, and putting it behind the same command would make a transfer look like
  a render.
- **A separate access-grant entity** is not needed. It would exist to hold
  credentials for clients outside the inventory, and `devices: unmanaged`
  answers that case without a new kind of file.

## Milestones

These cover the whole of `dgs conf`, this page and [`export.md`](export.md)
together, because the inventory changes what the renderer reads and what a
target is. Each is one commit leaving a tree that builds and passes. Every
package below takes plain values and returns plain values, imports no TUI and no
configuration, and has its own tests.

### What the existing packages become

Three packages were built before the inventory existed. They are not preserved
for their own sake; what survives does so because it was never about the layout
that changed.

| Package | What happens to it |
| --- | --- |
| `internal/conf/confgen` | The manifest half stays: discovery, strict unknown-key parsing, broken services listed with a reason. `loadInstances` and the `Instance` and `RoleInstances` types go, because an instance is no longer a file in a role directory. |
| `internal/conf/render` | `funcs.go` stays almost whole — the function set is independent of where values come from. `secret` changes meaning, from walking a per-service secrets file to naming one of an instance's own secrets. `Input` becomes the render context. |
| `internal/conf/target` | The path matcher goes entirely. A selector matches fields now, and a target is an instance. What survives is behavioural: a stable sort, and a selector matching nothing being an error rather than an empty result. |

### 0. Convert the existing generator root

Before any code: rewrite the current values files as node files, `users.yaml`
and `routes.yaml`. It is an hour by hand at this size, and a converter would
cost more than it saves for a thing run once.

It comes first because real data is the fastest test of whether this model is
right. A case it cannot express is worth finding before eleven milestones are
built on it.

### 1. Trim `confgen`

The root becomes `services/`. The manifest gains `secret` and each role gains
`auth`; the `secrets` key goes. Nothing in the package knows what an instance
is any more.

### 2. `internal/conf/inventory` — reading

`nodes/*.yaml`, `users.yaml`, `routes.yaml` and `networks.yaml` parsed into one
model. Parsing only: no derivation, no validation. A file that will not parse is
listed with its error rather than dropped, as a broken service already is.

### 3. `internal/conf/derive` — derivation

Client instances, edges, resolved addresses, grants and per-port principal
tables, as a pure function from the inventory model. It touches no filesystem.

Its tests use the worked example on this page as a fixed input, and pin the
three address cases, both kinds of principal, and two ports of one instance
carrying separate tables.

This is the middle of the whole design, and the place where a test is worth
writing before the code.

### 4. `internal/conf/validate`

The [rules above](#validation), one case and one error message each, every
message naming what it read and where it read it.

### 5. `internal/conf/secretstore`

The path layout, reading, `sync`, `mv` and `.previous`. Generated values take
the shape the service declares.

Its tests run over a temporary directory, and the case that matters most is a
rename: equal counts of new and orphaned paths must produce the `secret mv`
hint rather than a silently generated value on one side of a pair.

### 6. Rework `render`

`Input` becomes the render context this page pins, and `secret` changes with it.
The rest of the function set does not move. At the end of this milestone one
target renders end to end.

### 7. Rewrite `target`

A target is an instance. A selector is `<field>:<value>` terms. `--targets`
reports grouped by node. The old matcher is deleted rather than kept working
beside the new one.

### 8. The page

The tree's top level becomes nodes, with unmanaged users as their own entries.
Tri-state selection, folding and preview keep the behaviour they have.

### 9. Export

The output gains its node directory. Render-all-then-publish does not change.

### 10. Rotation

`.previous` in the rendered account tables, and the staleness check in
`validate`.

### 11. A worked example

A complete inventory under `examples/conf/`, loaded by the test that loads every
example. Until this exists, nothing has checked that the configuration on this
page can actually be written.

### Two cautions

**Milestones 2 to 6 are one stretch.** Stopping among them leaves two ideas of
what an instance is in the tree at once. If the work has to be split, 7 and 8
are the ones to defer — the old selector and the old page can sit over an empty
target list for a while; 2 to 6 should not be interrupted.

**Do 0 and 3 first, and their tests before their code.** Real data and a pinned
derivation are the fastest way to find out whether this model is right. If they
are wrong, the other nine milestones are wasted.
