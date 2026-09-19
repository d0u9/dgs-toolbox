# Configuration: Export

`dgs conf export` renders service configuration files from templates, the
[inventory](inventory.md) and secrets, and writes a chosen set of them to a
folder or a zip archive.

It is the generator side of what [`dgs cred`](../cred/vault.md) protects: the
templates and the values are in version control, the secrets they draw on are
not, and the rendered result is plaintext that leaves the machine only when
someone asks for it.

## Why it exists

The configuration it renders is kept as one directory per service, with one
value file per instance:

```text
05-confgen/
├── hysteria2/
│   ├── templates/server.yaml.tmpl
│   └── server/
│       ├── default.yaml
│       ├── us-sfo-dgo-linux-01.yaml
│       └── us-sfo-dgo-linux-01.sh
├── microbin/
└── ssserver/
```

Every instance has a `.sh` beside its values that calls the renderer with the
paths worked out from its own name. The scripts are copies of one another:
adding an instance means copying a script and editing it.

Three things follow, and this page addresses each:

- **Per-instance boilerplate.** An instance should be a few lines of values and
  nothing else.
- **Per-service difference.** Services differ in ways that matter — see
  [defaults](#two-kinds-of-defaults) — so the differences become a declaration
  read by one renderer rather than a script per instance.
- **Getting the result out.** Rendering prints to stdout, so collecting the
  files for a machine, or for a person, is done by hand.

### Names get unified

The directories are spelled four ways across three services — `server/` beside
`servers/`, `default.yaml` beside `defaults.yaml` — and the spellings are
accidents rather than decisions. They are unified as part of this work, and
nothing here is designed to keep an old name working: these files have one
consumer, who would rather rewrite them than carry the difference. A design that
finds a cleaner shape should take it and rename what it must.

## What it reads

### The generator root

`conf.root` holds the services and the inventory:

```text
~/confgen/
├── services/<service>/       confgen.yaml, templates, defaults
├── nodes/*.yaml
├── users.yaml
├── routes.yaml
└── networks.yaml
```

A subdirectory of `services/` is a service when it holds a `confgen.yaml`, so a
`README`, a scratch folder or a service still being written is skipped rather
than half-read.

The four files beside it are the inventory: the machines, the people, and the
chains between them. [`inventory.md`](inventory.md) describes them, and this page
assumes them.

### The service manifest

`services/<service>/confgen.yaml` declares what the renderer cannot infer. A
service is one program, so a manifest describes one template, one output name
and one inbound contract:

```yaml
# services/ssserver/confgen.yaml
secret:
  kind: base64
  bytes: 32

auth: per-principal
self:
  psk: {set: true}
template: templates/config.json.tmpl
defaults: element
output: config.json
```

```yaml
# services/ssserver/exports/link/confgen.yaml
template: templates/share.txt.tmpl
defaults: element
output: share.txt
upstream:
  shared: {}
```

`ssserver` and `sslocal` are two services rather than two forms of one: they
read configurations with nothing in common. An **export** is not a service at
all — nothing runs a QR code — so it lives in its service's own `exports/` and
its manifest has a template, a defaults kind, an output name, and
`upstream` when the credential it writes is partly the server's.
The directories a service holds are the ways it offers; nothing lists them a
second time. See
[which export a person receives](inventory.md#which-export-a-person-receives).

The keys in the table below are fixed. Every name around them — the services,
the templates, the output files — is this manifest author's, and no rule keys
off any particular spelling. The two lists are in
[fixed names and example names](inventory.md#fixed-names-and-example-names).

| Key | Meaning |
| --- | --- |
| `secret` | The shape of a generated credential for this service: `kind` and its size. Omitted, a printable random string. |
| `auth` | Whether this service's inbound side authenticates each principal separately: `per-principal` or `none`. Its own credentials are `self`'s business, not this key's. See [inventory](inventory.md#how-a-service-says-what-it-needs). |
| `template` | The template rendered for this service, relative to the service directory. |
| `defaults` | How `defaults.yaml` applies: `document` or `element`. See below. |
| `output` | The name the rendered file is written under — the name the program expects where it runs, such as `config.json` or `server.env`. |
| `rotation` | `disruptive` when this service's template cannot emit two accounts for one principal, so rotating it drops the connection instead of overlapping old and new. Omitted, it can. See [rotation](inventory.md#rotation). |
| `self` | This service's own secrets — credentials belonging to the instance rather than to anything reaching it — declared by name, each with its shape (`kind`, `bytes`), and optionally `set: true` for a family of values whose keys an instance declares, or `fields` for a credential made of several generated parts. One file per leaf, under `<instance>/self/`. It is the only place they are named, so `secret sync` generates what is declared and reports what is not. See [a service's own secrets](inventory.md#a-services-own-secrets). |
| `downstreams` | `many` when one instance of this service is the entrance for several routes — a reverse proxy in front of many web services — or omitted for the ordinary `one`. It is the only thing that relaxes the rule that a non-terminal hop has the same successor in every route through it, and it says nothing about credentials: a proxy forwards and does not authenticate, so declaring it hands the service nothing. See [a service that fans out](inventory.md#a-service-that-fans-out). |
| `upstream` | What this service needs from the hop it dials, beyond the address, port and account every template is given, declared by name. The one name today is `shared`: the secrets that hop's port hands to everything granted on it, in the port's own order. A secret crosses from one instance to another because the program dialling declares it needs it, never because the one listening publishes it, so a service that writes nothing here is handed nothing. An export takes the same key for the same reason. A name `dgs` does not understand is an error. See [what a service needs from its upstream](inventory.md#what-a-service-needs-from-its-upstream). |

The defaults file is always `defaults.yaml` in the service directory, so it is
not declared.

There is no `secrets` key naming a file. Which credentials a service draws on is
derived from the inventory, and where each one lives is derived from what it
opens; see [Secrets](inventory.md#secrets).

Services are directories rather than two fixed names `server` and `client` so
that a protocol wanting a third — a relay, a bridge, a second client program —
gets one by adding a directory rather than by waiting for the renderer to grow
a case for it. Exports are directories for the same reason: a QR code is one
more directory, not a new case in the renderer.

### Two kinds of defaults

`defaults` is declared because the services genuinely differ, not because they
drifted.

- **`document`** — the defaults are a whole configuration, and an instance's
  `values` are merged over them, key by key, the instance winning. Hysteria2
  and MicroBin work this way: the TLS paths, the masquerade target and the
  MicroBin environment block are the baseline, and an instance overrides keys
  of it. What merges is `values` alone: `id`, `service`, `bind` and `ports`
  are dgs's own, are reached through the `instance` function, and are not keys
  of the document. So the dot a template reads is the service's settings and
  nothing else, and `values` is the one place a node file writes them. See
  [an instance's own values](inventory.md#an-instances-own-values).
- **`element`** — the defaults are one entry of a list, and apply to *every*
  entry of that list in an instance. Shadowsocks works this way: an instance's
  `servers` is a list of services on different ports, and each one starts from
  the same baseline entry. Merging at the document level here would put the
  baseline's one-entry `servers` list against the instance's list rather than
  into each of its entries.

The existing Shadowsocks templates do the element-level merge themselves,
reading the defaults through a second datasource. Whether the renderer performs
it instead and hands the template a finished list is
[open](#open-questions).

Unknown keys are an error, as everywhere else `dgs` reads configuration, so a
misspelled key cannot leave a setting quietly at its default.

### Instances

An instance is an entry in a node file's `instances` list, or one derived from a
user's access — see
[What is derived](inventory.md#what-is-derived). Nothing registers it: adding an
instance is adding a few lines to the node it runs on, and the next run lists
it.

A node file that is not valid YAML, or an instance entry that is not a mapping,
is listed as **broken** with the parse error, rather than omitted — an instance
that silently stops appearing is worse than one that appears with a reason it
cannot be rendered.

### An instance is one file on one machine

One machine may hold several instances of one service: two Hysteria2 servers, or
several Shadowsocks clients reaching different servers. An instance is one
rendered configuration file, and the node it belongs to is the file it is
written in.

Two things follow:

- **No values file names a secret.** Which credential a rendered file carries
  follows from what it connects to and who it connects as, both of which the
  inventory states. A key typed into a values file and matched by hand against a
  server is the thing this replaces.
- **The instance name is free.** It is an identifier for the reader, not a host
  name the renderer parses. It must be unique across the inventory, because hops
  name instances with nothing to qualify them; beyond that nothing checks it.

### Output names do not vary

A service's `output` is one name for every instance of it, because the program
reading it expects one name wherever it runs. Two clients reaching different
servers both render to `config.json`, and are told apart by the directory the
export puts them in, not by the file name. Copying the right one to the right
machine is done by hand and deliberately so: a name that varied per instance
would mean editing a service unit to match every time.

### Targets and selectors

The unit of everything below is a target, and a target is one instance.

A selector is one or more `<field>:<value>` terms, separated by spaces. **Terms
naming one field are alternatives, and different fields narrow each other.** A
value may contain `*`:

```text
node:host-a                          everything that machine runs
user:doug                            everything every device of one person needs
service:hysteria2                    one service
route:home-sfo                       every instance along one chain
instance:ss-sfo01                    one
node:us-sfo-*                        a machine name with a wildcard
user:jane export:link                one person, one way
export:link export:json              either way, whoever holds it
node:laptop profile:singbox          one use of one device
```

A bare word with no field is an instance name.

`profile:` picks among the files a device with
[profiles](inventory.md#a-device-with-several-profiles) is written out as, and
matches nothing else: a machine's instances, a device without profiles and a
file a person carries have no profile.

Alternatives within a field are what make picking several ways of handing one
credential over possible — a Shadowsocks server is written out as a share URI
and as a `config.json` alike, and a hand-over sometimes wants one, sometimes
both. It is also what makes a shell's own brace expansion do what it looks like
it does, since `export:{link,json}` expands to exactly those two terms:

```bash
dgs conf export user:jane export:{link,json}
```

The selector reads fields rather than a `<service>/<instance>` path
because the machine is a field. A path glob could only pick out one machine's
instances when the machine's name had been written into each instance's name,
and `user:` and `route:` could not be expressed at all — while those two are the
exports performed most often, one per person and one per chain.

A selector matching no target is an error naming the selector, not an empty
export.

`dgs conf --targets` writes every target found, with its status, to stdout and
returns without opening the TUI, as [App reports](../../tui.md#app-reports)
describes. It is how a reader checks what the generator root holds, and what a
selector will match, without rendering anything.

## Rendering

A target renders from four inputs:

1. the service's `defaults.yaml`,
2. the instance's own values, applied over the defaults at the level the
   service declares,
3. what the inventory derives for it — the node, the resolved upstream, the
   principals holding a grant on each port, and the secrets those imply,
4. the target itself — its service and instance names, as a datasource named
   `target`.

The third is the whole of [`inventory.md`](inventory.md) arriving as data, and
its shape is pinned there as
[the render context](inventory.md#the-render-context): it is the contract
between the inventory and every template, so changing it is a breaking change.

The context is a set of datasources rather than one deep-merged mapping so that
the values stay exactly what the service's own configuration says, with nothing
reserved inside them, and so a template that fails can be told which level it
was reading.

Secrets are read at render time and held only as long as the render. They are
not written anywhere except the output the export publishes.

### The template language

The existing templates are gomplate. Across all three services they use ten
functions: `coll.Append`, `coll.Has`, `coll.Merge`, `coll.Omit`, `coll.Slice`,
`data.ToJSON`, `data.ToYAML`, `dict`, `ds` and `required`.

`dgs` installs one binary and may not call a `gomplate` on `PATH`, so gomplate
is compiled in or it is not used. gomplate does work as a library —
`gomplate.NewRenderer` with the datasources given as `RenderOptions`, which
renders these templates unchanged — but importing the whole of it brings its
datasource backends with it. Measured against a stripped 1.6 MB empty program,
with `dgs` itself at 24.6 MB today:

| What is imported | Binary | Added |
| --- | --- | --- |
| `gomplate/v4` | 84.3 MB | +82.7 MB |
| `gomplate/v4/coll` and `/data` | 18.7 MB | +17.1 MB |
| `gomplate/v4/coll`, marshalling YAML and JSON directly | 3.6 MB | +2.0 MB |
| nothing | 1.6 MB | 0 |

The first line pulls in 155 modules — the AWS, Azure and Google Cloud SDKs,
`k8s.io/client-go`, Vault, Consul and OpenTelemetry — for datasource backends
this reads none of, since every datasource here is a local file. Most of the
second line is `cuelang.org/go`, behind `data`'s CUE parser.

**The last line is the one taken: gomplate is not a dependency.** The functions
are small — `coll.Merge` is a twenty-five line recursive map merge where the
override wins and a list replaces a list rather than merging into it — and
`Masterminds/sprig`, the obvious alternative at +5.1 MB, does not carry a
`toYaml` at all, so that would be paid for and then written anyway.

Not inheriting gomplate's function set means not inheriting its spelling either.
`coll.Merge` is a function named `coll` returning a struct with a `Merge`
method, a namespace worked around rather than a namespace; with the names free,
they are flat:

| Function | What |
| --- | --- |
| `merge` | Merge maps, later argument losing, lists replaced whole. |
| `omit`, `pick` | A map without, or with only, the named keys. |
| `has` | Whether a map has a key or a list holds a value. |
| `append`, `slice`, `dict` | Building a list or a map in a template. |
| `toYAML`, `toJSON` | `yaml.Marshal` and `json.Marshal`, which `dgs` already has. |
| `required` | The value, or an error naming what is missing. |
| `b64` | base64url without padding, which is what a share URI's userinfo is. |
| `join` | Values joined by a separator, which is how the protocols taking more than one credential take them: a Shadowsocks 2022 password is the server's PSK and the user's own, joined by a colon — `join ":" (upstream).shared` beside `(upstream).secret`, for a manifest that declared it needs them. |
| `secret` | The instance's own secret of a given name, narrowed by further arguments: a key for a `set` name, a field for a name with `fields`, both for a name with both. Given fewer arguments than the name has levels, it returns the map of what is under it. |

| `published` | The name one of this instance's own ports answers to, by port name, or empty. A service behind a reverse proxy renders the same string the proxy matches its site block on — `DOMAIN=https://{{ published "web" }}` — so the two cannot disagree. See [the name a port is published at](inventory.md#the-name-a-port-is-published-at). |

Beside them, one accessor per datasource — `defaults`, `node`, `instance`,
`upstream`, `downstreams`, `principals` and `target` — which is how a template reads
[the render context](inventory.md#the-render-context). They are functions
rather than fields of one value so that a template naming a level that does
not exist for it, an `upstream` on a terminal instance, fails where it is
written.

`secret` is new, and replaces the loop every template currently opens with: each
walks the whole secrets file with `coll.Has` to find the entry whose `servers`
list holds its key. That loop goes away entirely. A per-principal credential
arrives in the context already matched to the principal that holds it, and
`secret` is left for an instance's own secrets — a TLS key, an administrative
password — named rather than searched for. A name that matches nothing is an
error naming it rather than an empty `dict` that fails further down.

`merge`'s behaviour is pinned by tests rather than by a dependency, the
list-replaces-list rule above especially: element-level defaults exist precisely
because of it.

The cost this accepts is error messages. `text/template`'s own are poor —
`executing at <.foo>: nil pointer evaluating interface {}.foo` says nothing
about which instance failed — so every render error is wrapped with its service,
service and instance before it reaches the page, and `required` says what was
missing and where it was looked for.

## The page

Exporting from the TUI happens on the inspect page's Nodes and Users tabs,
per [`inspect.md`](inspect.md#marking-and-exporting). No separate page is
needed: those two tabs are already the target tree, one by machine and one
by person, so a page of its own would draw the same tree again.

### Selecting

Every row on Nodes and Users carries a checkbox, and marking follows the
tri-state behaviour the recipient checklist in
[`cred/vault.md`](../cred/vault.md#the-flow) uses:

- `Space` marks the row under the cursor and every instance under it: a group,
  a node, a person, a device or credential, or one instance. Pressing it again
  on a fully marked row unmarks the row.
- An instance can then be unmarked on its own. A parent shows `[x]`, `[-]` or
  `[ ]` when all, some or none of its instances are marked.
- `a` marks every instance on the tab, or clears every mark when the tab is
  already fully marked.

```text
▾ [-] us-sfo-dgo-linux-01
  ├─ [x] ss-sfo01                ssserver
  └─ [ ] bin-sfo01               microbin
▾ [x] doug
  ▸ [x] macbook                  doug's device, 2 files
```

The box sits beside the name, after the row's indent, branch and fold marker,
so it keeps the tree's depth.

What is marked is one set of instances, not one per tab. A device marked
under its node on Nodes shows marked under its owner on Users, since they are
the same entry seen from two directions. So nodes and people can be mixed in
one export. The status bar's centre counts the marked instances.

Folded rows keep their mark. A broken target can be marked, and the export
refuses it by name, the same way the command line does.

### Previewing

`Enter` on an instance renders that one target and shows the result, scrollable,
without writing anything. It is the same rendering the export performs, so a
template that cannot find a secret says so here —

```text
missing MicroBin auth password
```

— rather than in the middle of an export, or after the archive has been handed
to someone.

Rendered output is plaintext secrets on the terminal, and stays in its
scrollback. The preview says so, for the same reason opening a vault file does.

### Exporting

`x` exports every marked instance. With nothing marked, it exports what the
row under the cursor stands for: one node, one person, one device, or one
instance. On Services, an instance row exports that one deployment. So
exporting a single thing needs no marking.

The steps are the shared ones: a form, then the confirmation dialog, then the
result in the status bar.

- **Format.** Folder or Zip, and Show when every file is for one person.
- **Destination.** A directory chosen with the File Explorer: `Enter` on the
  row opens it, starting at `conf.export.dir`. Zip writes `conf-export.zip`
  in that directory.
- **Replace files already there.** Off by default. When it is off and a file
  would be replaced, the form says so and does not go on. This matches the
  command line's `--overwrite`.

`n` renders every target before anything is written. The confirmation then
lists each file and marks those it `(overwrites)`, the same plan the command
line prints. A successful export clears the marks.

### Showing one person's files

When every selected file is for one person, Show comes first in the form, and
it is the default. A person's files are the ones pasted into a client, and a
share link is shorter to copy than to write to disk and open again.

Show writes nothing. `n` renders the files and puts them on screen one at a
time. `Tab` moves to the next file and `↑` `↓` scroll. `c` copies the file on
screen to the clipboard through the terminal (OSC 52), up to the 64 KiB a
terminal clipboard takes. The view says the text is plaintext with every
credential in it, and that it stays in the terminal's scrollback.

Show is not offered for a server's files, or for files of more than one
person. Those go to a folder or an archive.

## What is written

A file whose output name ends in `.json` is indented two spaces per level,
keeping its keys in the order the template wrote them. This applies to the
page, Show and the command line alike. A template writes JSON the way that is
easiest to template; an export is read by a person first. Output that does not
parse is written unchanged, so the server that reads it reports the error.

Every target becomes its own directory, under the node it belongs to, whether or
not it is the only one:

```text
us-sfo-dgo-linux-01/
├── hysteria2/hy2-sfo01/config.yaml
├── microbin/bin-sfo01/server.env
└── ssserver/ss-sfo01/config.json

macbook/
├── hysteria2-link/macbook-jp-hysteria2-link/share.txt
├── ssserver-link/macbook-sfo-ssserver-link/share.txt
└── ssserver-json/macbook-sfo-ssserver-json/config.json
```

A deployment is filed under the service it runs; a file written for a person,
under the service and the way it was written. Both halves are needed, since an
export's name is unique only within its service and two services each offering
a `link` would otherwise share a directory. A device gets one directory per way
the services it reaches offer, unless it narrows with its own `export`.

The bundle of someone with no device file is named for the person rather than a
node, since there is none, and holds one directory per credential they keep:

```text
friend-a/
├── hysteria2-link/yak-default-jp-hysteria2-link/share.txt
├── ssserver-link/yak-default-sfo-ssserver-link/share.txt
└── ssserver-json/yak-default-sfo-ssserver-json/config.json
```

The layout says which machine each file is for, where it came from and what it
is, without a note beside it, and two instances of one service cannot collide. Both
the node directory and the instance directory are kept in the single-target case
too: a layout that changes shape with the number of targets is one a script
reading it has to handle twice.

An export renders every target first, and publishes only once all of them have
rendered. A failure anywhere reports the target and the error and writes
nothing — a half-written folder of configuration is the one outcome worth
ruling out, since what is missing from it is not visible in it.

Publication is the toolbox's existing one: a checked temporary file linked to
its final name, as [`cred/vault.md`](../cred/vault.md#publication) describes, so
an export cannot silently replace a folder or an archive that is already there.

Exporting the same selection twice is the normal case, though — a machine's
configuration changes and is sent again — so a destination already holding
these files is a question, not a failure. The plan names every file and marks
the ones already on disk, and the confirmation asks about replacing them; a
replaced file is written through the same temporary file, renamed over the old
one, so a reader sees either the old file or the new one. Answering no writes
nothing at all, including the files that were not in the way: a bundle written
half from this export and half from the last one is not a thing anyone asked
for.

`--yes` answers that question only when nothing is in the way. Replacing a file
from a script is `--overwrite`, because `--yes` means "do not ask me what I
already told you" and not "whatever is there, discard it".

### Writing to stdout

`--to -` writes to stdout instead of into a directory. The one command wanted
most often is a share URI on its way to a clipboard or a QR encoder, and a
directory holding one file, made to be read once and deleted, is a detour
through the disk that the credential did not need to take.

```bash
dgs conf export user:jane export:link --to - | pbcopy
```

**Stdout carries the rendered bytes and nothing else.** The plan and the
plaintext warning go to stderr, so a program reading the pipe reads the file a
directory would have held, byte for byte, whatever its format.

**One file, or an error.** `--to -` with a selector matching several files
fails, naming each of them. Two rendered files concatenated are no longer
either of them — a `config.json` following a `share.txt` parses as neither —
and separators that made them readable to a person would take the pipe away
from every program. The error names what matched, because narrowing the
selector is what the caller does next.

`--format yaml` is the other half: every file on one stream, each keeping the
path it would have had in a bundle.

```yaml
files:
  - path: jane/ssserver-link/jane-default-sfo-01-ssserver-link/share.txt
    content: |
      ss://...
  - path: doug/ssserver-json/doug-default-sfo-01-ssserver-json/config.json
    content: |
      {"server": "..."}
```

It is a document rather than a concatenation: `yq -r '.files[0].content'` hands
back the file, and nothing is lost by the file next to it. A rendered file that
is not valid UTF-8 cannot be a YAML scalar, so it carries `encoding: base64`
and says how to read itself back rather than being written out mangled.

The two are separate spellings because they answer different questions. `--to
-` alone is "give me this file"; `--format yaml` is "give me these files and
their names". One flag meaning both would make the shape of the output depend
on how many targets a selector happened to match, which is the thing a script
cannot handle.

`--format` needs `--to -`; against a directory it is an error, since a bundle
already carries paths in its layout. `--overwrite` against `--to -` is an error
too: it replaces files a destination already holds, and stdout holds none.
Neither `--yes` nor the confirmation applies, because nothing is replaced and
nothing is left behind — the pipe is the answer.

## Secrets leaving the machine

Everything an export writes is plaintext. It is the same material the vault
holds encrypted, in the form a server reads it.

- The secrets tree is a root of its own, outside `conf.root` and so outside the
  repository the inventory lives in; a directory that is not under the
  repository cannot be committed by accident, which a `.gitignore` entry only
  promises. An export destination is not covered by that, so `conf.export.dir`
  defaults outside any repository, and a destination inside one is a warning on
  the confirmation dialog naming the repository.
- The confirmation dialog names every file to be written and says that the
  result is plaintext.
- A plain `.zip` sent to someone is the secret in transit. The recipients and
  sealing `dgs cred` already has answer this: an archive encrypted to a host or
  a group, published as `.zip.age`, is the same export with `seal` in front of
  the writer. It is [deferred](#deferred) to its own milestone, not to another
  design.

`dgs` does not claim more than the process boundary allows here either: a
rendered file is on disk, and what the reader then does with it — a copy, a
backup, a chat window — is outside what this page can promise.

## Configuration

These keys are the design's. They belong in `docs/configuration/conf.md` and in
the index table there, written in the change that implements them, per the rule
in [`AGENTS.md`](../../../AGENTS.md). They are listed here so the design reads on
its own, and are not yet a reference.

| Key | Meaning | Default |
| --- | --- | --- |
| `conf.root` | The directory holding `services/` and the inventory. | empty — the page opens with no root and asks for one |
| `conf.secrets` | The root of the secrets tree. | empty — an instance needing a secret refuses to render |
| `conf.export.dir` | Where the destination form opens, for a folder or an archive. | empty — the home directory |

Paths follow the rules in
[`configuration/index.md`](../../configuration/index.md): `~`, `$NAME`, absolute
after expansion.

## Open questions

- **Where the element-level defaults merge happens.** Now that the renderer owns
  the function set, it can merge each list entry against the defaults itself and
  hand the template a finished list, or keep handing the defaults over for the
  template to merge as the Shadowsocks templates do today. The first makes the
  templates shorter and the manifest's `defaults: element` meaningful to the
  renderer; the second keeps the renderer from knowing which key holds the list.
- **What `(unmanaged user)` labels.** `--targets` groups by node and falls back
  to the user's key for targets with no node, marking those groups
  `(unmanaged user)`. The group is right — a credential no device names has no
  machine to sit under — but the words read as a statement about the person,
  and a person with a node file and a person-carried credential appears twice,
  once under their device and once under their own key with that label. A
  reader asks whether `doug` is managed or not, which is not the question the
  line answers. Whether to reword it, to say what the group is instead of what
  the person is, or to leave the grouping and the wording alone, is open.
- **How a command-line export fits the command model.** The TUI comes first and
  the command line follows it — see [Deferred](#deferred) — but a leaf command
  opens a TUI today, and reports are read-only, so an invocation that writes
  files is outside the current command model. That is the part still open, and
  it needs a decision in [`tui.md`](../../tui.md) rather than a flag added here.

  It is open only for the command line now.
  [`dgs conf secret show`/`edit`](inventory.md#viewing-and-editing-several-at-once)
  no longer waits on it: [`inspect.md`](inspect.md#editing-a-secret) settles that
  half by keeping the write inside a leaf command's page, behind the same
  confirmation dialog every other write in `dgs` uses, rather than adding a
  side-effecting Cobra subcommand. An export writing from its own page is the
  same shape; what stays undecided is an invocation that writes without opening
  one.

The per-instance `.sh` files are not among these: they are deleted with the
directory rename, not kept working alongside the manifest.

## Deferred

- **Encrypted archives.** `--encrypt-to <host|group>`, writing `.zip.age`
  through `internal/cred/seal` and the recipient folder, so an export can be
  handed over without the plaintext leaving the vault's model. Deferred to keep
  the first pass to rendering and writing, not because the design is unclear.
- **Checking every target.** Rendering all targets and reporting only the
  failures answers "did that secrets change break a host?", which today is
  answered by exporting and reading. It is the preview with no output and no
  selection, and is worth its own report once rendering exists.
- **Installing to a host.** Copying an export to the machine it is for, over
  SSH, with the service reload that follows. It is a different problem — reach,
  permissions, restart — and putting it behind the same key would make a
  transfer look like a render.

## Milestones

1. **Manifests and discovery.** Read the generator root, parse manifests, list
   targets, report broken ones. A package of its own, with tests over a
   constructed root.
2. **Rendering.** One target to bytes, in a package that knows nothing of the
   TUI: the function set above, the two kinds of defaults, the secrets lookup,
   and render errors that name their target.
3. **Selectors.** Matching, and the `--targets` report.
4. **The page.** Tri-state marking on the inspect page's Nodes and Users tabs,
   and the marked count in the status bar. Built.
5. **Preview.** `Enter` on an instance, with errors shown as they come back.
6. **Export.** `x` on the inspect page: the form, the confirmation dialog,
   render-all-then-publish, folder and zip. Built.
7. **Encrypted archives.** As deferred above.

The first of these were built before the inventory existed, against a target
that was a path and a secret that was a lookup key in a values file. The list
above is left as the record of that, and the work that revisits it is
[`inventory.md`](inventory.md#milestones), which covers both pages: discovery
gains the node files, rendering takes the render context in place of the
per-service secrets file, and selectors match fields in place of path segments.
The page, the preview and the export keep their shape; what changes under them
is the tree's top level, from services to nodes.
