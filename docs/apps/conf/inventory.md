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
| **service** | One program a node deploys, with its template, defaults and output name: `ssserver`, `sslocal`, `hysteria2`, `microbin`. |
| **export** | One way of handing a credential to a person: a share URI, a JSON configuration, a QR code. Deployed nowhere. |
| **instance** | One service running on one node, and one rendered configuration file. |
| **port** | A named listening port of an instance. Every port an instance listens on is named; an instance that listens on nothing has none. |
| **route** | An ordered list of hops, from where traffic enters to where it leaves. |
| **hop** | One step of a route, written `<instance>:<port>`. |
| **user** | A logical identity: a person, not a Linux account and not a password. |
| **credential** | One of a person's identities. A device names which one it uses; two naming the same one share a secret. |
| **group** | The directory a node's file sits in: whose machines these are — a person, or whoever hosts them. |
| **grant** | A credential for one party to reach one port. Derived, never written. |
| **secret** | One credential value, one file. |

Two terms describe things the reader never writes. A **grant** is derived from a
user's access list or from a route's hops; a **principal** is whatever holds one
— one of a person's credentials, or an upstream instance. They are named here
because the secrets layout and the rendered account tables are organised by
them.

A principal is never a device. How many credentials someone keeps is theirs to
decide: one password across a laptop and a phone is as real as one per machine,
and a model that made the device the identity could write down only the second.
A device names one of its owner's credentials instead, so both are one shape.

`connection` is deliberately absent. In an earlier design a connection was
something written by hand for every client reaching every server, which is the
N×M table this model exists to avoid. What remains of it is the route, which is
written once per chain, and the grant, which is derived.

## Fixed names and example names

Everything on this page is written in terms of one worked inventory, and most of
the words in it are that inventory's, not the model's. An implementation that
recognises `ssserver`, `sslocal` or `internet` has hard-coded someone's
data.

**Fixed.** `dgs` knows these and nothing else:

| What | The names |
| --- | --- |
| Files | `services/`, `services/<service>/exports/`, `nodes/`, `users.yaml`, `routes.yaml`, `networks.yaml`, `confgen.yaml`, `defaults.yaml` |
| Node keys | `id`, `networks`, `reaches`, `owner`, `export`, `profiles`, `credential`, `instances` |
| Profile keys | `export`, `values`, `access` |
| Instance keys | `id`, `service`, `ports`, `process`, `bind`, `self`, `values` |
| Port keys | `port`, `protocol`, `self`, when a port is written as a mapping rather than a bare number |
| User keys | `username`, `devices`, `export`, `credentials`, `access` |
| Credential keys | `note`, `access` |
| Route keys | `hops` |
| Network keys | `networks`, `universal` |
| Service manifest keys | `secret`, `auth`, `template`, `defaults`, `output`, `rotation`, `self`, `upstream` |
| Self declaration keys | `set`, `fields`, `kind`, `bytes` |
| Export manifest keys | `template`, `defaults`, `output`, `upstream` |
| Enumerated values | `devices: none`; `auth: per-principal`, `none`; `defaults: document`, `element`; `export: none`; `rotation: disruptive`; `protocol: tcp`, `udp`; `kind: opaque`; `upstream: shared` |
| Reserved words | `self`, as a port name and as a secrets path segment |
| Defaulted names | `default`, the credential a person has when they declare none and a device uses when it names none |
| Secrets path segments | `self`, and the `.previous` suffix |
| Selector fields | `node`, `user`, `service`, `export`, `profile`, `instance`, `route` |
| The loopback address | `127.0.0.1`, for two hops on one node |

**An example.** Every one of these is a name the inventory's author chose, and
the model attaches no meaning to any particular spelling:

| What | As written here |
| --- | --- |
| Service names | `ssserver`, `sslocal`, `hy2client`, `hysteria2`, `microbin`, `httpproxy` |
| Export names | `link`, `json`, `shadowrocket` |
| Network names | `home`, `internet` |
| Port names | `main`, `alt`, `proxy`, `local`, `web`, `socks`, `http` |
| Node, instance, user and route identifiers | `us-sfo-dgo-linux-01`, `ss-sfo01`, `doug`, `jp` |
| Names of an instance's own secrets | `auth_password`, `server_password`, `psk`, `tls_key` |
| Keys and fields of one of them | `main`, `backup`, `username`, `password` |
| Everything inside `values` | `masquerade`, `fast_open`, `public_path` — a template's own, not dgs's |
| Output file names | `config.json`, `share.txt` |

`ssserver` is in the second table and not the first. No rule keys off it: a
service authenticates because it says `auth: per-principal`, and is reached
because something names it, not because of what it is called. The same is true
of `internet`, which is the universal network only because `networks.yaml` says
so.

## Files

```text
~/confgen/                    conf.root
├── services/<service>/       a program a node deploys
│   └── exports/<export>/     a way of handing that service's credential over
├── nodes/<group>/*.yaml      a machine, its networks, its instances
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

### Starting a generator root

```bash
dgs conf init [<dir>] [--secrets <dir>] [--gitignore]
```

`<dir>` is the generator root to create, and defaults to `conf.root`. The
secrets root is a separate root, so it comes from `--secrets` or `conf.secrets`
rather than from the same path, and is skipped when neither gives one.
`--gitignore` writes one into the secrets root as well, for the case where it
does sit somewhere a repository can see.

**It writes a scaffold, not an example.** Each file is a valid, empty one
carrying the comments that say what belongs in it, so `dgs conf --check` is
clean the moment it finishes and nothing has to be deleted before real content
goes in. A worked inventory to copy from is [`examples/conf/`](../../../examples/conf)
instead — an example written into someone's own root is a machine that does
not exist, and deleting it is a step the scaffold should not create.

**It never overwrites.** A root half-built by hand, or built by an earlier
version of this command, gains what it is missing and keeps what it has; the
report says which files were which. Running it twice is the same as running it
once.

`nodes/` and `services/` get a `README.md` each rather than being left empty,
since an empty directory does not survive version control, and the file
explaining what goes in the directory is the one that should be sitting there
when someone opens it.

## Nodes

A node is a machine or a device. Servers and phones are the same kind of thing
here, which is why the word is `node` rather than `host`.

Node files sit one level down, in a directory naming whose machines these are:

```text
nodes/
├── digitalocean/us-sfo-dgo-linux-01.yaml
├── digitalocean/mn-uln-dgo-linux-01.yaml
├── home/server.yaml
├── home/nas.yaml
└── doug/phone.yaml
```

The directory is the **group**. A group naming a user in `users.yaml` is that
person's devices, and fills `owner` for every file in it, so the owner is not
repeated in each one. A group naming no user is whoever the machines belong to
— a provider, a household, a company — and `dgs` reads nothing further into it.
Only one level is read: a directory is a group, never a path. A file written
straight into `nodes/` has no group, which is a group of one rather than a
special case.

```yaml
# nodes/digitalocean/us-sfo-dgo-linux-01.yaml

id: us-sfo-dgo-linux-01

networks:
  internet: 203.0.113.10

instances:
  - id: ss-sfo01
    service: ssserver
    process: ss-main
    ports:
      users: 38250

  - id: ss-sfo01-relay
    service: ssserver
    process: ss-main
    ports:
      relays: 49217

  - id: hy2-sfo01
    service: hysteria2
    ports:
      users: {port: 443, protocol: udp}
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
# nodes/home/server.yaml

id: home-server

networks:
  home: 192.168.1.10

instances:
  - id: http-home
    service: httpproxy
    bind: "0.0.0.0"
    ports:
      proxy: 8118

  - id: ss-home
    service: ssserver
    bind: 127.0.0.1
    ports:
      local: 1080

  - id: bin-home
    service: microbin
    bind: "0.0.0.0"
    ports:
      web: 8080
```

It has no address on `internet`, and does not need one: it reaches out through
NAT, and nothing connects to it from outside. `networks` says where a node can
be reached, not where it can reach — see
[below](#networks-and-how-an-address-is-chosen).

A node hosting no services needs only two lines:

```yaml
# nodes/doug/macbook.yaml

id: doug-macbook
reaches: [home]
```

`owner` is the group, so it is not written. A node kept somewhere else may name
one anyway, and an `owner` written wins over the directory. A node that hosts
services has no owner either way.

`credential` names which of its owner's credentials this device authenticates
with, and defaults to `default` — see [Users](#users). Two devices naming the
same one hold one secret between them.

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

`networks` is where others reach this node, which is not always what its own
interfaces hold. A machine published through a forwarded port writes the address
others dial, not the private one it is configured with; the field answers "where
do I send a packet for this node", and a reader copying an interface address into
it has answered a different question.

A node sitting on two networks writes both, and that is the whole of an edge
host in front of a private subnet:

```yaml
id: edge-fra
networks:
  internet: 203.0.113.40
  backend: 10.0.0.1
```

Backends on that subnet have a `backend` address and no `internet` one. They
still reach the internet, because reaching and being reached are separate
fields and `universal` covers the first — they fetch updates and send mail as
before. Nothing outside can open a connection to them, because nothing outside
has an address to dial. The separation that exists for NAT does this second job
unchanged, which is why a private subnet needs nothing new here.

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

**A port may say its transport.** A bare number is TCP, which is what a reader
assumes when nothing says otherwise. A port on anything else is written as a
mapping instead, and the other ports of the same instance are untouched:

```yaml
  - id: hy2-sfo01
    service: hysteria2
    ports:
      users: {port: 443, protocol: udp}

  - id: caddy-sfo01
    service: caddy
    ports:
      https: 443
```

Two ports collide only when they share an address, a number **and** a
transport. A QUIC service on 443/udp beside a web server on 443/tcp is two real
listeners, and a check that knew only the number would call one machine bound
twice.

**Name a port after what arrives on it**, not after its position. A port is the
unit an account table belongs to, so `users` and `relays` say what a reader
needs — which of an instance's two tables this is — where `main` and `alt` say
only which was written first:

```yaml
  - id: ss-sfo01
    service: ssserver
    ports:
      users: 38250
      relays: 52146
```

**Several ports of one program are one instance.** An instance is one rendered
configuration file, and one program reads one file, so the ports a single
program listens on belong together. Each port still has its own principals, its
own grants and its own account table; what they share is the file they are
written into, and whichever of the instance's own secrets each of them hands
out — see [a secret several people hold](#a-secret-several-people-hold):

```yaml
  - id: ss-sfo01
    service: ssserver
    ports:
      users:  {port: 38250, self: [psk.main]}
      relays: {port: 52146, self: [psk.backup]}
```

**Two instances are for two files.** A second server of the same service, with
its own configuration and its own values, is a second instance; `process` says
two of them are served by one running program, which is what a delivery step
that assembles one file out of two needs to know:

```yaml
  - id: ss-sfo01
    service: ssserver
    process: ss-multi
    ports:
      users: 38250

  - id: ss-sfo01-legacy
    service: ssserver
    process: ss-multi
    ports:
      users: 38260
```

An instance naming no `process` is a process of its own, which is the ordinary
case. Combining what two instances render into one program's configuration file
is a question for delivery, not for this model — and with ports of one program
in one instance, it is a question that arises far less often than it did.

`bind` is optional and defaults from the service's defaults file. A server binds
every interface; a client's local listener binds loopback.

A port named `self` is rejected; see [Secrets](#secrets).

### The name a port is published at

**A port reached from a browser says the name it answers to**, as a bare
hostname:

```yaml
  - id: vault-fra
    service: vaultwarden
    bind: 127.0.0.1
    ports:
      web: {port: 8222, published: vault.example.com}
```

It is written on the port rather than on the route because three different
readers need it, and only one of them is the proxy. A reverse proxy in front
matches its site block on it. **The service behind it needs the same string in
its own configuration** — Vaultwarden builds absolute URLs, a cookie domain and
a WebAuthn relying-party identifier out of it, and a value that disagrees with
what the proxy serves breaks sign-in rather than the page. And a person types it
into a browser.

A route could carry it instead, and then the second reader could not have it. An
instance's render context is a local view — its node, itself, its upstream, its
principals, its own secrets — and a value living on another hop of a route that
merely ends here is a backwards lookup. Reaching it would mean handing every
instance the route table, which is the one shape [the render
context](#the-render-context) is arranged to avoid.

A bare hostname, not a URL: the proxy's site block wants the name alone, and a
template that needs `https://vault.example.com` writes the scheme itself, where
the other direction would have every template take a URL apart.

A template reads its own port's name with `published "<port>"`, the way it
reads an account table with `principals "<port>"`. A proxy reads the names of
the ports it fronts from [`downstreams`](#the-render-context) instead, because
those belong to other instances.

A client reads the name of the port it dials as `(upstream).published`. The
services on a node can answer to a name other than the node's — one shared, or
one each — while the node keeps its own address in `networks`, and an exported
link or configuration writes the service's name rather than the node's.
`(upstream).address` is still the node's address chosen by the rule above, so
which of the two a file carries is the template's choice, usually
`or (upstream).published (upstream).address`.

The name is given only on an edge resolved on the `universal` network, which is
where a name is taken to resolve. An edge between two ends on one node, or one
resolved on a private network such as `home`, was chosen an address that
network needs — loopback, a LAN address — and a public name would send the
client out and back in, or nowhere. On those edges `(upstream).published` is
absent, and `or (upstream).published (upstream).address` falls back to the
address the rule chose. An inventory with no `universal` network gives no
names this way.

`published` stands on its own. A service reached from a browser on its own
address, with no proxy anywhere, still has a name it answers to and still has to
render it into its own configuration. It is a property of the port, which is why
a port is where it is written.

**One name per port.** Two ports may share a name — it is a DNS name, and two
services on one machine answering to it on different ports, Shadowsocks on
38250/tcp and Hysteria2 on 443/udp, is an ordinary deployment, since a client
dialing the name dials the number too. Sharing is broken in three cases, and
the check reports each wherever it is written: ports on two nodes, because a
name reaches one machine; a port a proxy fronts, because the proxy tells its
downstreams apart by name alone; and two ports on one number and transport,
because nobody dialing the name can tell them apart.

### An instance's own values

`id`, `service`, `bind` and `ports` are what dgs itself needs: enough to
place an instance, find its template, and resolve every address and secret
around it. They are not enough to configure a service — Hysteria2 needs a
masquerade target, Shadowsocks a `fast_open` flag, an nginx server block
whatever an nginx server block needs — and none of that is a network fact, a
listening port, or shared by every instance of a service. `values` is where it
goes:

```yaml
  - id: bin-sfo01
    service: microbin
    bind: 127.0.0.1
    ports:
      web: 8080
    values:
      public_path: https://paste.example.com/
      upload_enabled: true
```

`values` is opaque: dgs parses it as YAML and nothing more. It does not
validate its shape and does not know what any key means. Where it reaches the
template follows from the service's defaults kind, and from nothing else: with
`document` defaults it merges over the whole defaults document, key by key,
and the instance wins; with `element` defaults it reaches the template
unmerged, and the template does the merging it wants. Either way a template
also reads it by name, as `(instance).values`. See
[two kinds of defaults](export.md#two-kinds-of-defaults).

**Only `values` merges.** `id`, `service`, `bind` and `ports` are dgs's own and
stay out of the document: they are read through the instance function, so a
service's settings have one place a node file may write them, and a document
never grows a key no service declared.
This is deliberate, not a gap left for later: a masquerade target is Hysteria2's
concept, not the inventory's, and adding a typed field for every service's own
parameters would mean this page growing a section per service forever. What
belongs in the inventory is what routing, address resolution and secrets need to
work; what one instance of one service is configured to do belongs to that
service, expressed however its template wants it.

**A value that does not vary between instances of a service belongs in the
service's `defaults.yaml` instead.** A masquerade target chosen once and reused by every
Hysteria2 server is a default, not a value — `values` is for what a second
instance of the same service would need to say differently, MicroBin's public path
being the clear case: two MicroBin instances cannot share one.

An authored override on a client node carries `values` the same way it carries
`ports` and `bind`; see [what is derived](#what-is-derived).

## Users

```yaml
# users.yaml

users:
  doug:
    access: [jp, sfo, home-sfo, bin, nas]
    credentials:
      default:
        note: the laptop, the phone and the iPad
      work:
        note: the office laptop — revocable on its own
        access: [sfo]

  friend-a:
    username: yak
    devices: none
    export: link
    credentials:
      default:
    access: [jp, sfo]
```

`access` lists routes. `username` is the account name the services see, and
defaults to the user's own identifier, so it is written only where the two
differ.

### Credentials belong to the person

`credentials` names this person's credentials. Each is a mapping: `note` says
where it is used — free text `dgs` never reads — and `access` narrows it to
some of the person's routes. A person who declares none has one, called
`default`.

**The value is always a mapping, even where only the note is written.** A
credential that is one string today and a mapping the moment it narrows would
be two shapes for one thing, and every reader would have to know both. One
shape costs the common case a line and buys a key that can grow.

**How many credentials someone keeps is theirs to decide, not the service's.**
One password across a laptop and a phone is as real as one per machine: the NAS
has one `doug` in its account table either way, and a model that made the device
the identity could write down only the second. So the credential is declared on
the person, and a device names which one it uses:

```yaml
# nodes/doug/phone.yaml
id: doug-phone
# credential: default, so it is not written
```

```yaml
# nodes/doug/work-laptop.yaml
id: doug-work-laptop
credential: work
```

| | Account | Secret | Files |
| --- | --- | --- | --- |
| `default` | `doug-default` | `ss-sfo01/users/doug/default` | one per device naming it, per route it opens |
| `work` | `doug-work` | `ss-sfo01/users/doug/work` | one per device naming it, per route it opens |

Two devices naming one credential are **one principal**: one row in the
server's table, one file on disk, one password. They still render a
configuration each, because their local ports and client programs differ — they
share a secret, not a file.

**A credential is an account because it is declared, not because a device names
it.** Writing one down is the decision; a device only chooses which of them it
carries. So a credential no device names is still an account, with its own row
in the server's table and its own secret — it is one the person carries
themselves, onto whatever machine is at hand:

```yaml
  doug:
    access: [sfo]
    credentials:
      default: whatever machine is at hand
      mbp: the laptop, revocable on its own
```

```yaml
# nodes/doug/mbp.yaml
id: mbp
credential: mbp
```

Here `mbp` is carried by that laptop and `default` by the person. Both are
accounts on the port `sfo` enters; the first renders a file for the laptop, and
the second one named for the person, since there is no device to name it for.
A file the person carries is dialed from a machine this inventory does not
model, so it resolves on the `universal` network, and a route granted to
someone in this position whose entry has no address there is an error.

The alternative would be a credential that exists in `users.yaml` and nowhere
else: no account, no secret, no file, and nothing saying why. Declaring one is
the deliberate act, and deleting one is the other.

A device naming a credential its owner does not keep is an error listing the
ones they do.

**The account name is the username and the credential, always both.** A table
of `doug-default` and `doug-work` says what it is; one where the first
credential is bare and the rest are suffixed would rename the first the day a
second appears. `default` is a name like any other — it is written out in
account names, in secret paths and in the picture — and it is simply the one
you get without choosing.

**Keeping one credential per device is still available**, by declaring one per
device and naming them. That is the safer default in the sense that losing a
machine costs one deletion, and it is not the model's business to insist on it:
a person who uses one password everywhere is describing what they actually do,
and writing two values the server then has to carry would be the inventory
inventing a fact.

**Deleting one is a deliberate step.** Removing a credential leaves its value on
disk, where `secret sync` reports it as orphaned and never deletes it — it
cannot tell a removal from a rename. That report is the reminder to delete the
file and re-render the servers it reached; until both are done, the password
still opens the port. `dgs conf --check` names it.

### A credential may open fewer routes

`access` on a credential narrows it to some of the routes its owner holds:

```yaml
  doug:
    access: [jp, sfo, home-sfo]
    credentials:
      default:
        note: the phone
      mbp:
        note: the laptop, which leaves the house
        access: [sfo]
```

`mbp` is an account on the port `sfo` enters and on no other. The laptop takes
a file for `sfo` and none for `jp` or `home-sfo`, and the account tables those
two routes enter never hold its password. Losing the laptop costs one route,
not every route its owner has.

**Access is granted to the person, and a credential chooses among it.** A
credential may name only routes its owner already holds; one naming a route
they do not is an error listing the routes they have. The alternative — a
credential reaching past its owner — would mean reading `users.yaml` twice to
answer what one person can reach, and the `access` line would stop being the
answer.

**Omitted, a credential opens every route its owner holds.** That is what a
credential without an `access` means, so adding a route to a person adds it to
each of their credentials, and only the ones that say otherwise stay behind.

**A device carries one credential and nothing else.** A route its credential
does not open is a route that device cannot take, however the owner's own
`access` reads, so nothing is rendered for the pair.

**Narrowing is not the same as a second person.** Two people are two account
names, two sets of files and two lines in the picture. One person with a
narrowed credential is one identity whose password for one line is revocable
on its own — which is what someone means when they say the laptop should not
be able to reach the house.

### A person with no device file

`devices: none` says this inventory has no node file for this person, and that
it is deliberate:

```yaml
  friend-a:
    username: yak
    devices: none
    export: link
    credentials:
      default:
    access: [jp, sfo]
```

It says only that, and it decides nothing. Credentials are declared the same
way as anybody else's, and since none of them is named by a device, every one
is [carried by the person](#credentials-belong-to-the-person) — the same rule
as everybody, with nothing left on the other side of it. What the person copies
one onto is not modelled, because it cannot be: how many machines someone else
uses a configuration on is not a fact the inventory can hold, and inventing
node files for machines nobody tracks would make it lie.

**The key exists to tell two silences apart.** A person with no node file and a
person whose node files were put in the wrong directory look identical. `devices:
none` makes the first one something written down, so validation can name the
second — and writing it beside a node file that *is* theirs is a contradiction
validation reports.

With no node to carry an `export`, it is written on the user instead and means
the same thing. `link` is usually the answer: which program someone else runs
is not a fact the inventory can hold, and a share URI is the one form nearly
every program imports.

## Routes

```yaml
# routes.yaml

routes:
  jp:
    hops: [hy2-tyo01:users]

  sfo:
    hops: [ss-sfo01:users]

  home-sfo:
    hops: [http-home:proxy, ss-home:local, ss-sfo01-relay:relays]

  bin:
    hops: [bin-sfo01:web]
```

Each adjacent pair of hops is one edge. `home-sfo` says traffic enters the home
server's HTTP proxy, passes to the Shadowsocks client on the same machine, and
leaves through the relay port in San Francisco. Two of those hops share a node
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

**Unless the service fans out.** A reverse proxy — Caddy, nginx, Apache — is one
entrance in front of many services, so it has as many successors as there are
routes through it. A service says so once, in its manifest, with
[`downstreams: many`](#a-service-that-fans-out), and rule 8 stops applying to
its instances:

```yaml
routes:
  vault:
    hops: [caddy-fra:https, vault-fra:web]

  bin:
    hops: [caddy-fra:https, bin-fra:web]
```

This is not the rule-based routing above, and the difference is not one of
degree. A proxy picks its upstream by the name the request arrived at, and every
route through it names exactly one — the value is
[the downstream port's `published`](#the-name-a-port-is-published-at), written in
the inventory and readable before anything runs. Rule-based routing picks an
upstream per request from a rule set this inventory does not hold, and one route
can then leave by several exits. The first is static fan-out and is derived; the
second is a decision at runtime, and it stays unsupported.

The hops stay a flat list either way. A route is still one chain from entrance
to exit; what changed is that two chains may share their first hop.

## What is derived

Nothing below is written by hand.

**Grants.** Every credential a person declares is a grant on the entry port of
every route it opens — declaring one is what makes it an account. That is every
route the person is granted, unless the credential's own `access` narrows it. A
device names which one it carries; it does not bring one into being.

**Export instances.** A device and each route its credential opens produce a
file named `<node>-<route>-<service>-<export>`; a credential no device of
theirs names produces `<username>-<credential>-<route>-<service>-<export>`,
one per such credential, because the person carries it themselves. The service
is in the name because an export's name is unique only within its service. Its
values come from its own export's defaults. A device with
[profiles](#a-device-with-several-profiles) produces one file per profile
instead, named `<node>-<route>-<service>-<export>-<profile>`.

It is not a deployment. Nothing runs a share URI, and the program that reads a
JSON configuration runs on a machine this inventory does not model.

### Which export a person receives

**A service is a program a node deploys. An export is a way of handing that
service's credential to a person.** They are separate kinds of thing, and an
export sits inside the service it writes out:

```text
services/     ssserver  sslocal  hy2client  hysteria2  microbin  caddy
services/ssserver/exports/     link  json  shadowrocket
services/hysteria2/exports/    link  shadowrocket
```

A QR code settles the first half: nothing runs one, so a way of writing a
credential out is not a program. The nesting settles the second: an export
renders one service's `upstream` and nothing else, so it belongs to that
service rather than beside it. Two services each offering a `link` are two
exports, and the pair `<service>/<export>` is what names one.

**An export is named after the format, not after the program that reads it,
and not after the protocol** — the directory above it already says which
service this is. `json` is the JSON a shadowsocks-rust client reads. `sslocal`
is the program, and it is a *service*, because the home server deploys one:

```yaml
# nodes/home/server.yaml — a deployment
  - id: ss-home
    service: sslocal
    bind: 127.0.0.1
    ports:
      local: 1080
```

Those two are not the same file and should not share a template. `ss-home` is
a hop: `http-home` dials into its `local` port, so that port's number and
address are a contract other instances depend on. The file written for a
laptop is a leaf: nothing dials into it, and its local port means something
only to the applications on that machine. They will diverge, and each should
be free to.

**The directories a service holds are the ways it may be written out.** It
does not list them as well: a list could name a directory that is not there,
or miss one that is, and a reader would have no way to tell which of the two
is the truth.

```yaml
# services/ssserver/confgen.yaml
auth: per-principal
self:
  psk: {set: true, kind: base64, bytes: 32}
template: templates/config.json.tmpl
output: config.json
```

```yaml
# services/ssserver/exports/link/confgen.yaml
template: templates/share.txt.tmpl
defaults: element
output: share.txt
```

**An export manifest has three keys and no more.** No `ports`, since nothing
listens; no `auth`, since nothing connects to a file; no `self`, since what
it carries belongs to the service it reaches and arrives through
`upstream`. The manifest being half the size of a service's is the sign the
split is along the right seam.

**Every one of them is written.** Which form someone wants is a question asked
when the files are handed over, not when the inventory is written. So all of
them render, a bundle holds each, and a
[selector](export.md#targets-and-selectors) narrows to the ones this hand-over
needs:

```bash
dgs conf export user:jane                                # every way
dgs conf export user:jane export:link                 # one of them
dgs conf export user:jane export:link export:json     # two of them
```

They share a credential — the account is the person's, not the file's — so
several forms cost several renderings and no extra secret.

**A device narrows to one of them** when it is known to run one program:

```yaml
# nodes/doug/phone.yaml
export: json
```

This is a property of the device rather than of each instance, because a device
runs one client program for everything it proxies. It does not say which
service it narrows *within*, and does not have to: a device granted both
Shadowsocks and Hysteria2 routes takes each service's own `json`. A name only
one of them offers narrows that one and leaves the other at everything it
offers.

**A service's exports and a device's `export` answer two different
questions.** The service's directories say which ways exist, which is a fact it
knows. The device says which one it can read, which is a fact only it knows — a
manifest that guessed would hand a phone a configuration written for a
command-line client.

So the resolution is:

1. **A service with no `exports/` writes nothing**, whatever the device says.
   MicroBin is reached from a browser: there is no file to hand over, and a
   route entering it still produces a grant, but no file. The address and the
   password reach the person some other way, which is the truth of how such a
   service is used and not something to invent a file for. `export` does not
   bring a file back — it chooses among the ways, never whether any exists.
2. Otherwise the node's `export`, or, for a file the person carries rather
   than a device, the user's, narrowing to that one way.
3. Otherwise every way the service names.

An `export` naming something none of the services this device reaches offers is
an error listing the services it does reach. It does **not** fall back to the
full list: the failure would then happen on the device rather than here.

### A device with several profiles

One device may run several programs that each want the same credential
written differently: sing-box and a browser's SOCKS listener on one laptop,
each on its own local port. `export` cannot say that — it picks one way for
the whole device — so a device may declare **profiles**, the uses it is put
to, by name:

```yaml
# nodes/doug/macbook.yaml
id: doug-macbook
profiles:
  singbox:
    export: singbox        # a format of its own: its own template
  browser:
    export: json
    values:
      local_port: 1080     # the same format, one value changed
    access: [sfo]
```

A device with profiles is written out **once per profile**, and each profile
narrows in the device's place: its `export` chooses among the ways the
services it reaches offer, exactly as a device's `export` does, and unwritten
takes every one. The file name gains the profile —
`doug-macbook-sfo-ssserver-json-browser`. A device without profiles is
written out once, as before.

**Two ways to make a profile different, and both are meant.**

- **A different format is a different export.** When the program reads a
  different file altogether — sing-box's configuration is not sslocal's
  `config.json` — the difference is a template, and a template lives in the
  service's `exports/<name>/` with its own `defaults.yaml`. The profile only
  names it. Every device with the same use names the same export, and the
  settings are written once.
- **The same format with a value changed is `values`.** A second listener on
  another port is not a second format, and copying a template to change one
  number is the copying this page exists to remove. `values` reach the
  template as the file's own values, as an authored override's do.

An instance authored on the device with a profile's file name still pins it,
and its `values` win over the profile's key by key. The profile says what the
use needs; the override is the exception for one route.

`access` narrows a profile to some of the routes the device's credential
opens, as a credential's `access` narrows its owner's. Unwritten, every one.

**Profiles are files, not accounts.** Every profile carries the device's one
credential, so the server's table holds one row for the device however many
profiles it has, and revoking the device revokes all of them. A use that must
be revocable on its own is a separate credential, and a credential is chosen
by a device, not by a profile.

A device with profiles does not also write `export`: each profile says its
own, and a device-wide one beside them would be read by nothing. A profile
with `export: none` is an error rather than a way of switching one off — a
profile writing nothing is a profile to delete. Profiles belong to devices; a
node nobody owns runs services and has nothing written out for it.

`dgs conf export node:doug-macbook profile:singbox` hands over one use.

### When the client program is unknown

Two answers, and both already exist in the shapes above.

A **link export** writes the protocol's own share URI — `ss://`,
`hysteria2://` — to a text file. Nearly every client imports one, so one of
them per protocol covers every program nobody has written a template for. It is
what a credential carried by the person rather than by a device gets, since the
program it will be pasted into is not a thing the inventory can know, and it
costs nothing to write beside the others.

**`export: none`** says this device receives nothing. The grant is still
derived and the server still carries the account; only the file is absent, for a
machine configured by hand.

An export instance's ports are its own local listeners — the SOCKS and HTTP
ports an application on that device connects to. Nothing reaches a phone from outside,
so they are never hop targets, and they carry no secrets. Their names are read
by the template, which needs to know which listener is which; the address and
port at the far end are not written here, because the route derives them.

A derived instance is a default, not a fixed result. Authoring an instance with
the same identifier pins what it names and leaves the rest derived:

```yaml
# nodes/doug/phone.yaml

id: doug-phone
reaches: [home]

instances:
  - id: doug-phone-sfo-ssserver-json
    ports:
      socks: 10080
      http: 18080
```

Everything not written stays derived: the export, `bind`, and every value the
export's defaults supply. An override that replaced the whole instance
would mean copying a client configuration out in full to change one port, which
is the copying this page exists to remove.

`service` may not be written in an override. An export instance has none: it
follows from the route's entry hop and its `exports`, so a line naming it is
either redundant or a contradiction.

Written and derived instances share one namespace, so the uniqueness check
covers both.

**Grants.** One per (principal, port) pair: one for each credential granted a
route, and one for each non-terminal hop reaching the hop after it.

**One principal on one port is one credential**, whatever brings it there.
Two routes a person is granted that enter the same port give them two client
configurations and one account: a server's table has no way to tell which
route a connection came over, and nothing downstream could act on it if it
had. A second account on the same port therefore means a second credential,
not a second route — and declaring one is a line in `users.yaml`.

Two entries in `users.yaml` for one person are still the answer when the
accounts are meant to be independent of each other rather than one person's:
a separate access list, separate grants, and nothing tying them together.
[Rule 13](#validation) requires their rendered names to differ — a port cannot
hold two accounts called `doug-default`, and a server could not tell them apart
if it did.

**Server account tables.** The principals holding a grant on a port, rendered as
accounts. People and machines are listed together, because to the server they
are the same thing:

```text
ss-sfo01, port users:
  doug-default
  doug-work
  yak-default

ss-sfo01-relay, port relays:
  ss-home
```

Two ports of one instance carry two independent tables and two independent sets
of credentials.

## Secrets

One credential is one file. The path is the identity:

```text
<instance>/<port>/<group>/<name>
```

| Segment | What it says |
| --- | --- |
| `<instance>` | Which instance the credential opens |
| `<port>` | Which port of it — ports have separate account tables |
| `<group>` | Whose it is: a person, or an instance relaying through |
| `<name>` | Which of theirs: a credential, or `default` for a relay's single one |

```text
~/confgen-secrets/
├── ss-sfo01/
│   ├── users/doug/default
│   ├── users/doug/work
│   ├── users/friend-a/default
│   ├── relays/ss-home/default
│   ├── self/psk/main
│   └── self/psk/backup
├── hy2-tyo01/
│   ├── users/
│   │   ├── doug/default
│   │   └── friend-a/default
│   └── self/tls_key
└── bin-sfo01/
    └── self/auth_password
```

**Every credential under a port is a `<group>/<name>` pair**, whatever holds
it. Sorting by the kind of holder instead — a `node/` beside a `user/` — draws
a distinction a reader of the tree never needs, and leaves `node/phone` unable
to say whose phone it is. Grouping by the holder answers that. A relaying
instance is its own group: what connects is the instance, not the machine under
it and not whoever hosts that machine.

`self` is the exception, and sits at the instance level rather than under a
port: an instance's own secrets belong to the instance, and which of its
listeners hands one out is the port's business rather than the file's. An
instance may have several, and one of them may be a set of values or a record
of fields, so a path under `self` is one, two or three segments deep — see
[a service's own secrets](#a-services-own-secrets). `self` is therefore a
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
printf 'new-value' > ss-sfo01/users/doug/default

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

Neither exists yet, and neither is a Cobra subcommand.
[`inspect.md`](inspect.md#editing-a-secret) is what implements them: `show` is a
secret view read, `edit` is `e` on it, so the write happens inside a leaf
command's page behind the usual confirmation dialog rather than from an
invocation outside the read-only reports [`tui.md`](../../tui.md#app-reports)
describes. What remains open in [`export.md`](export.md#open-questions) is the
command line alone.

### How a service says what it needs

A service declares whether its inbound side authenticates:

```yaml
# services/ssserver/confgen.yaml

secret:
  kind: base64
  bytes: 32

auth: per-principal
template: templates/config.json.tmpl
defaults: element
output: config.json
```

```yaml
# services/sslocal/confgen.yaml

auth: none
template: templates/config.json.tmpl
defaults: element
output: config.json
```

| `auth` | What a grant on one of this service's ports implies |
| --- | --- |
| `per-principal` | One secret per principal reaching each port, and a rendered account table |
| `none` | Nothing |

There is no third value. Declaring a secret and handing it out are two separate
things: [`self`](#a-services-own-secrets) declares one, and
[a port's own `self` list](#a-secret-several-people-hold) says it travels to
whatever is granted on that port. Either can be used without the other — MicroBin's admin password
goes to nobody, and a service can hand out a secret whose principals have none
of their own — so `auth` keeps only the question it was always answering: does
something reaching this service get a credential of its own.

`auth` is per service, and a client service usually declares `none`: its local
listener is not the protocol at all — a Shadowsocks client's loopback SOCKS
port authenticates nobody, while `ssserver` authenticates everybody.

`secret`'s shape is the protocol's:
Shadowsocks 2022 needs base64 of exactly the key size, not an arbitrary
password. A service that does not declare one gets a printable random string.

### What a service needs from its upstream

`auth` and `self` describe the inbound side. `upstream` describes the other
one: what this program needs from the hop it dials, beyond the address, port
and account every template is given.

```yaml
# services/sslocal/confgen.yaml

auth: none
upstream:
  shared: {}
```

| Name | What it holds |
| --- | --- |
| `shared` | The secrets the upstream port hands to everything granted on it, in the order that port writes them |

Shadowsocks 2022 is the case it exists for. The password a client sends is
the server's PSK for that port and the client's own, joined, so `sslocal`
cannot render a working configuration from its own credential alone. The
order is the port's, because a protocol taking the two in the other order
authenticates nothing.

**The declaration is the consumer's.** A secret belongs to the instance it is
filed under, and it reaches another instance because the program dialling says
it needs it — never because the program listening happens to publish it. This
is the whole of the rule, and it is what keeps a web service's upload password
out of the reverse proxy in front of it: the proxy forwards, it does not
authenticate, so it declares nothing and is handed nothing, however much the
port it reaches hands to the people granted on it.

Reading it the other way round — every upstream port's `self` list travelling
to whatever dials it — makes two different facts one key. A port's `self` list
says what the *people* granted there hold; `upstream` says what a *program*
needs to connect. They coincide only while the person and the next hop are the
same party, which is exactly what a proxy in front of a service stops being
true.

An export declares it the same way and for the same reason: the share URI a
person imports carries the same two-part password its client configuration
would, and is as unusable without the server's half.

A name `dgs` does not understand is an error rather than something skipped.
Silently dropping a credential a program needs renders a file that looks
complete and does not authenticate, which is the worst of the ways this can
fail. Growing the list is adding a name — an upstream's certificate
fingerprint, its SNI — not changing the shape of the key.

### A service that fans out

A service that is one entrance in front of many says so once:

```yaml
# services/caddy/confgen.yaml

auth: none
downstreams: many
template: templates/Caddyfile.tmpl
defaults: document
output: Caddyfile
```

| `downstreams` | What an instance of this service may dial |
| --- | --- |
| `one` | One upstream, the same in every route through it. The default, and not written |
| `many` | One upstream per route through it, told apart by the `published` name of the port each route reaches |

`many` is the only thing that relaxes
[rule 8](#one-upstream-per-instance), so a service that has not asked for it
behaves exactly as before. What an instance of such a service is handed is
[`downstreams`](#the-render-context), the resolved far end of every route
through it.

It is a declaration about shape, not about credentials. A proxy declares no
`upstream`, and the reasoning is the section above: it forwards, it does not
authenticate, so nothing a port hands to the people granted on it reaches the
proxy in front. A fan-out service needing a secret from each of its downstreams
would be a new key and is not one of these.

### A service's own secrets

`auth` describes the inbound side: what a principal needs to reach this
service. A service also has credentials of its own, belonging to the instance rather than
to anything reaching it — an administrative password, an upload password, a
TLS private key. They are independent of `auth`, and a service declares them by
name:

```yaml
# services/microbin/confgen.yaml

auth: none
self:
  auth_password: {}
  admin_password: {}
  upload_password: {}
```

Each name is one `<instance>/self/<name>` file, one per instance of the service,
generated by `secret sync`. MicroBin authenticates nobody in the inventory's
sense — nothing routes through it, no principal holds a grant on it — and still
holds three passwords, which is why the two are separate keys.

An empty declaration is one generated value in the shape the service's `secret`
block declares. Three keys change that, and no more:

| Key | What it makes the name |
| --- | --- |
| `kind`, `bytes` | The shape of this name's value, where it differs from the service's `secret` block. `kind: opaque` means a value `dgs` never generates — a private key, a certificate chain, a vendor's keyfile — reported as missing until someone writes it. |
| `set: true` | A family of values under one name, whose keys each instance declares. One file per key: `<instance>/self/<name>/<key>`. |
| `fields` | One credential made of several generated parts, each with its own shape. One file per field: `<instance>/self/<name>/<field>`, or `<instance>/self/<name>/<key>/<field>` when the name is also a set. |

```yaml
# services/tuic/confgen.yaml

self:
  account:
    set: true
    fields:
      uuid:     {kind: uuid}
      password: {kind: base64, bytes: 32}
  tls_key: {kind: opaque}
```

```text
tuic-sfo01/self/account/main/uuid
tuic-sfo01/self/account/main/password
tuic-sfo01/self/account/backup/uuid
tuic-sfo01/self/account/backup/password
tuic-sfo01/self/tls_key
```

An instance says which of these it holds, and the keys of each set. Naming
a value on a port is already saying the instance has it, so the ordinary
case is writing nothing and letting the ports speak:

```yaml
  - id: ss-sfo01
    service: ssserver
    ports:
      users:  {port: 38250, self: [psk.main, psk.backup]}
      relays: {port: 52146, self: [psk.main]}
```

Writing `self` is for the two cases the ports do not cover. A key no port
hands out — one the instance keeps to itself — is written with its name:

```yaml
    self:
      psk: [main, backup, internal]
```

And an instance holding fewer of its service's own secrets than the service
declares narrows by writing the ones it has. A paste bin on the home network
with no administrative interface should not be handed the two passwords
guarding one:

```yaml
  - id: bin-nas
    service: microbin
    ports:
      web: {port: 8080, self: [upload_password]}
    self:
      upload_password:
```

Unwritten means every name the service declares, which is the ordinary case;
written, it is the whole list, so `self: {}` means none of them.

**The tree stops there: a name, a key, a field.** Everything deeper is
structure, and structure is configuration. Which values a port hands out, in
what order, what each one is called, what it is for — those are facts a diff
should show and a reviewer should read, so they live in the node file and in
`values`, where version control keeps them. The secrets tree holds the leaves
alone, because that is what keeps one credential one file: readable with `cat`,
writable with `printf`, rotatable on its own, and computable by `sync`. A value
that genuinely has no shape `dgs` can generate is `kind: opaque`, one file, not
a structure the tree has to learn.

**The manifest and the instance are the only places these are named.** The
manifest names the secrets and their shape; the instance names the keys of each
`set`. Together they are what makes the tree computable in both directions: a
path they imply with no file is generated, and a file under `self/` they do not
imply is reported orphaned like any other. Without them, the two are the same
fact on disk, and `sync` can only leave both alone.

**A value that has to be edited after it is generated is not a secret.** A
random administrative *password* is exactly what is wanted; a random
administrative *username* is a string you will replace by hand the first time
you see it, on every machine, forever. `kind: opaque` is the one exception,
and it is an exception about generation rather than about editing: the value
belongs in the tree, and nothing but a certificate authority or a vendor can
produce it. It is configuration, and belongs in the
service's `defaults.yaml` where a change to it is a diff someone can read. The
test is not whether a value is sensitive — a username is — but whether
generating it produces the value you want.

### A secret several people hold

A port says which of its instance's own secrets everything granted on it also
holds:

```yaml
    ports:
      users:  {port: 38250, self: [psk.main, psk.backup]}
      relays: {port: 52146, self: [psk.main]}
```

A port with no `self` hands out nothing beyond each principal's own secret,
which is the ordinary case. What the list means follows from `auth`, which is
the whole of the difference:

- With `auth: per-principal`, each principal's own secret and these arrive
  together. Shadowsocks 2022's server holds a PSK for the port, and each user's
  effective password is that PSK and their own, joined — no client can connect
  from its own secret alone.
- With `auth: none` there is no secret of their own, so these arrive alone: one
  upload password, known by whoever is granted the route.

**The list is ordered**, because the protocols that take more than one are
ordered: a Shadowsocks 2022 relay chain is a sequence, and rendering it in a
different order than the peer holds produces a configuration that authenticates
nothing. It is written in the node file rather than derived, so the order is a
line someone can read and a diff someone can review.

Naming the values per port, rather than marking one of them in the service
manifest, is what lets one program serve two ports on different credentials —
which is the case the manifest could not express, since a service's manifest
describes every instance of it at once and the difference is between two ports
of one. Two ports handing out the same value say so by naming it twice.

A port's list names `<name>.<key>` for a `set` name and `<name>` for the
others. It never names a field: a field is half a credential, and half a
credential is not a thing to hand over.

**Who holds one of these is who is granted the port** — which the model already
knows, so the blast radius of a rotation is something a view can name rather
than something to remember.

This list says who holds the value, and nothing else. It does not decide
whether the program dialling that port is given it: that is the dialling
service's own declaration, in
[`upstream`](#what-a-service-needs-from-its-upstream). The two coincide while
the person and the next hop are the same party, and a reverse proxy in front of
a web service is where they stop coinciding — the people granted there still
hold the upload password, and the proxy, which only forwards, is handed
nothing.

### Keeping the tree in step

`dgs conf secret sync` compares the paths the inventory implies against the
files on disk. Two things imply a path: a `per-principal` service implies one per
(principal, port) grant, and a service's [`self` declarations](#a-services-own-secrets)
imply, per instance, one path per name — per key for a `set` name, per field
for a name with `fields`, and one per key and field for a name with both. A
`kind: opaque` name is never generated: it is reported as missing until someone
writes it. A name whose keys and fields are only partly on disk is reported as
incomplete rather than completed, since the values that are there may have been
written by hand and the ones that are not may be on their way. Missing paths are generated. Paths on disk
that the inventory no longer implies are **reported and never deleted**,
because sync cannot tell a rename from a removal, and the failure mode of
guessing wrong is silent: a new random value on one side and the old one still
on the other.

Renaming a node or an instance is therefore an explicit move,
`dgs conf secret mv <old> <new>`, and sync says so when the number of new paths
and orphaned paths match. `mv` is not implemented yet: sync names the orphans
and the hint, and moving the files is done by hand until it is.

Sync names what it is about to write before writing it, and asks — the values
are credentials, and a server has to be rendered again and deployed before
anything can connect with a new one. `--yes` is what a script passes.

Sync also reports which servers need re-rendering. **Adding a credential is a change
on the server side**, not only a bundle handed to its owner: every instance it
reaches must be rendered again and deployed before anything can connect with
it. Adding a device that names a credential already in use is not — it renders
a file and touches no server.

### Rotation

A secret file may have a sibling named `<name>.previous`. A service rendering an
account table emits both values as two accounts; a client renders only the
current one. That makes a rotation three ordered steps with no disconnection:

1. Write the new value, move the old one to `.previous`, render and deploy the
   server.
2. Render and deploy the clients.
3. Delete `.previous`, render and deploy the server again.

The overlap is a second working credential, so it is a debt and not a state.
`validate` reports a `.previous` file older than seven days as an error.

A service whose template cannot emit two accounts for one principal declares
`rotation: disruptive`, and rotating it says up front that the
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
instance:    id, service, ports (each with its number — the transport is the
             model's, not a template's — the name it is published at, when it
             has one, and the self values it hands out, in the order written),
             bind, and the instance's own values
upstream:    the next hop, resolved: address, port, the name that hop's port
             is published at when it has one and the edge resolved on the
             universal network, the account name this
             instance connects as, and its secret; plus, for a target whose
             manifest declares it needs them, that hop's port's self values
             as shared, an ordered list — absent for a terminal instance,
             and shared absent from every target that did not declare it.
             See what a service needs from its upstream
downstreams: for an instance whose service declares downstreams: many, the hop
             that follows it in each route through it, resolved: address, port,
             and the published name that route arrived at. Ordered by route
             name, so a rendered file does not change because a route was
             added above another. Absent for every other instance
principals:  for a port with auth: per-principal, the account name and secret of
             everything holding a grant on it
self:        the instance's own secrets, by name: a value, a map of fields, a
             map of keys, or a map of keys of maps of fields, as the service
             declares
target:      service and instance names
```

An export instance's `instance` carries its `export` and, for one of a
device's [profiles](#a-device-with-several-profiles), its `profile`, beside
the values a profile or an override gives it. A service whose defaults apply
per `element` reaches those values unmerged, as `(instance).values`, and it
is for its template to lay them over `defaults`.

The context is structured rather than one deep-merged mapping so that a key
present at two levels cannot silently overwrite another, and so a template
failing can be told which level it was reading.

## Validation

`dgs conf validate` checks:

1. Node identifiers are unique; instance identifiers are unique, derived ones
   included. `universal`, if written, names a network in the list.
2. Every `instance.service` names a service. Every export a service holds
   declares a template, since one that renders nothing writes nothing. An
   instance's `self` names only the service's
   own `set` names, and a name the service declares `set` has its keys listed
   there. A port's `self` names `<name>.<key>` for a `set` name and `<name>`
   for every other, never a field, and every name and key it uses exists.
3. An `export`, on a node or on a user, is one of the ways
   offered by the services its granted routes enter, or is `none`. The same
   name under two of those services is not a conflict: the device takes each
   service's own. The error lists the services it reaches. A device reaching
   none is not consulted.
4. Every hop names an existing instance and an existing port on it.
5. No port is named `self`.
6. Every route named in an `access` list exists; every user named by an `owner`
   exists; a device's `credential` is one its owner declares, and the error
   lists the ones they do. A credential's `access` names only routes its owner
   holds, and the error lists those. `devices: none` is not written beside a
   node file owned by that person.
7. An instance authored on a client node overrides a derived one: its identifier
   matches an instance the user's access derives, so a misspelled route name
   cannot leave an override that nothing reads, and it does not set
   `service`.
8. A non-terminal hop has the same successor in every route through it, unless
   its service declares `downstreams: many`. The error names rule-based routing
   as the reason this is not allowed yet, and distinguishes it from fan-out so a
   reader knows which of the two they have written.
9. No route names one instance twice.
10. Every edge resolves an address: the two ends are on one node, or the
    downstream has an address on a network the upstream reaches.
11. A route in an `access` list has an entry the granted user's devices can
    reach, on a port not bound to loopback only.
12. The granted routes of a user who keeps a credential no device of theirs
    names enter on the `universal` network — that file is dialed from a machine
    this inventory does not model.
13. Account names rendered for one port are distinct.
14. Two instances on one node do not bind the same address, port and protocol.
15. Every secret the inventory implies exists, and every file in the secrets
    tree is implied by it. Both directions are reported; neither is fixed here.
16. No `.previous` file is older than seven days.
17. Every port a `downstreams: many` instance reaches declares `published`.
    Without it the proxy has nothing to tell one downstream from another, and
    the error names the instance and the route.
18. Two ports declaring the same `published` name are on one node, neither is
    fronted by a `downstreams: many` instance, and they differ in number or
    transport. The error names both.
19. A target declaring `upstream: shared` reaches a port whose `self` list
    hands something out. The declaration is the consumer's, so nothing about
    the port it dials makes it true, and a port handing out none renders an
    empty list into a credential built half from it — a file that looks
    complete and authenticates nothing.
20. A device with `profiles` has an owner and writes no `export` of its own.
    Each profile's name holds no slash or space, since it ends a file name;
    its `export` is one of the ways the services it reaches offer, and not
    `none`; its `access` names only routes the device's credential opens, and
    the error lists those.

## Boundaries

- **Rule-based routing** is not supported; see
  [One upstream per instance](#one-upstream-per-instance). Static fan-out, where
  each route through a proxy has one upstream, is a different thing and is
  supported.
- **A second name for one port** is not expressed. `published` is one hostname,
  which is what a service that builds absolute URLs out of it can actually have;
  a backend genuinely indifferent to its name cannot be offered under two. A
  list would make which one goes into that configuration a guess, so the case
  waits for something that names the canonical one.
- **Fan-out by path** — `/api` to one backend and `/` to another — is not
  expressed. A port is published at a name, not at a name and a prefix.
- **Network preference is global.** `networks` orders every edge the same way. An
  edge cannot be told to prefer a different network from the one the order picks.
- **Which parties may reach a port** is not expressed, and no check reports it.
  The inventory says an edge resolves and that a granted person can reach a
  route's entrance; it never says who *cannot* arrive. Two things do say it, and
  both are topology rather than a rule: a backend on a network no user's node has
  an address on, and a backend on the proxy's own node bound to loopback. A
  firewall is neither, and it is not modelled here — `validate` passing is not a
  statement that a port is unreachable from anywhere else.
- **Per-credential subsets of a person's access** are not expressed. Every
  credential someone keeps reaches every route they are granted. Scoping one to
  a subset would mean a credential's entry carrying an access list beside its
  note; nothing needs it yet.
- **Deployment** is not here. An export writes files; copying them to a machine
  and restarting the service is a different problem, with different failure
  modes, and putting it behind the same command would make a transfer look like
  a render.
- **A separate access-grant entity** is not needed. It would exist to hold
  credentials for clients outside the inventory, and a credential no device
  names answers that case without a new kind of file: it is an account and a
  file, carried by the person.
- **A virtual network** — WireGuard and the like — needs nothing new. The
  tunnel's server is an instance with a port and a grant per peer, and the
  address space it creates is a `network` like any other: peers list an address
  on it, and address resolution picks it over the internet by preference order.
  What is not built is drawing one in the picture, where a network cuts across
  the node and group boxes rather than nesting inside them.

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
| `internal/conf/confgen` | The manifest half stays: discovery, strict unknown-key parsing, broken services listed with a reason. `loadInstances` and the `Instance` and `RoleInstances` types go, because an instance is no longer a file in a service directory, and `Role` goes because a service is one program. |
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

The root becomes `services/`. The manifest gains `secret` and `auth`; the
`secrets` key goes. Nothing in the package knows what an instance
is any more.

### 2. `internal/conf/inventory` — reading

`nodes/<group>/*.yaml`, `users.yaml`, `routes.yaml` and `networks.yaml` parsed
into one model. A node's group is its directory, and fills `owner` when the
directory names a user. Parsing only: no derivation, no validation. A file that will not parse is
listed with its error rather than dropped, as a broken service already is.

### 3. `internal/conf/derive` — derivation

Client instances, edges, resolved addresses, grants and per-port principal
tables, as a pure function from the inventory model. It touches no filesystem.

Its tests use the worked example on this page as a fixed input, and pin the
three address cases, both kinds of principal, two ports of one instance
carrying separate tables, and two devices naming one credential being one
principal with one secret between them.

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

The tree's top level becomes groups, then nodes, with a person who has no
device file appearing as a group holding one entry per credential.
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
