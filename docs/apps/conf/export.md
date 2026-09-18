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
└── shadowsocks-rust/
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

`services/<service>/confgen.yaml` declares what the renderer cannot infer:

```yaml
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
  shadowrocket:
    auth: none
    template: templates/shadowrocket.conf.tmpl
    output: shadowrocket.conf
  link:
    auth: none
    template: templates/link.tmpl
    output: share.txt
```

Client roles are named after the client program rather than after `client`,
because the program is what decides the file: see
[which role a client derives as](inventory.md#which-role-a-client-derives-as).

The keys in the table below are fixed. Every name around them — the service, the
roles, the templates, the output files — is this manifest author's, and no rule
keys off any particular spelling, `server` included. The two lists are in
[fixed names and example names](inventory.md#fixed-names-and-example-names).

| Key | Meaning |
| --- | --- |
| `secret` | The shape of a generated credential for this service: `kind` and its size. Omitted, a printable random string. |
| `roles` | One entry per kind of instance the service generates. The key is the role name, and also the directory its defaults file is in. |
| `roles.<role>.auth` | Whether the role's inbound side authenticates: `per-principal`, `shared` or `none`. See [inventory](inventory.md#how-a-service-says-what-it-needs). |
| `roles.<role>.reached_by` | The role a client derives as, to reach this one. Omitted, a route entering this role renders nothing for the person granted it — see [what is derived](inventory.md#what-is-derived). |
| `roles.<role>.template` | The template rendered for the role, relative to the service directory. |
| `roles.<role>.defaults` | How the role's defaults apply: `document` or `element`. See below. |
| `roles.<role>.output` | The name the rendered file is written under — the name the service expects where it runs, such as `config.json` or `server.env`. |
| `roles.<role>.rotation` | `disruptive` when this role's template cannot emit two accounts for one principal, so rotating it drops the connection instead of overlapping old and new. Omitted, it can. See [rotation](inventory.md#rotation). |
| `roles.<role>.combine_own` | The name of one of this role's own secrets that every principal reaching it also needs, alongside its own. Omitted, a principal needs nothing beyond its own secret. See [a shared identity alongside a principal's own](inventory.md#a-shared-identity-alongside-a-principals-own). |

The role name is the directory name and the defaults file is always
`defaults.yaml` in it, so neither is declared. That is the naming unification
above spent on making the manifest shorter rather than on describing four
spellings.

There is no `secrets` key naming a file. Which credentials a service draws on is
derived from the inventory, and where each one lives is derived from what it
opens; see [Secrets](inventory.md#secrets).

Roles are a map rather than the two fixed names `server` and `client` so that a
service wanting a third — a relay, a bridge, a second client profile — declares
one instead of waiting for the renderer to grow a case for it. A role is not a
kind of machine: it is a set of instances sharing a template, a defaults file
and an output name, and the name is a label on that set.

### Two kinds of defaults

`defaults` is declared because the services genuinely differ, not because they
drifted.

- **`document`** — the defaults are a whole configuration, and an instance's
  values are merged over them. Hysteria2 and MicroBin work this way: `listen`,
  the TLS paths and the MicroBin environment block are the baseline, and an
  instance overrides keys of it.
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

One machine may hold several instances of one role: two Hysteria2 services, or
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

A role's `output` is one name for every instance of it, because the service
reading it expects one name wherever it runs. Two clients reaching different
servers both render to `config.json`, and are told apart by the directory the
export puts them in, not by the file name. Copying the right one to the right
machine is done by hand and deliberately so: a name that varied per instance
would mean editing a service unit to match every time.

### Targets and selectors

The unit of everything below is a target, and a target is one instance.

A selector is one or more `<field>:<value>` terms, separated by spaces, all of
which must match. A value may contain `*`:

```text
node:host-a                     everything that machine runs
node:macbook                    everything for one device
user:doug                       everything every device of one person needs
service:hysteria2 role:server   the server side of one service
route:home-sfo                  every instance along one chain
instance:ss-sfo01                 one
node:us-sfo-*                   a machine name with a wildcard
```

A bare word with no field is an instance name.

The selector reads fields rather than a `<service>/<role>/<instance>` path
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

1. the role's `defaults.yaml`,
2. the instance's own values, applied over the defaults at the level the role
   declares,
3. what the inventory derives for it — the node, the resolved upstream, the
   principals holding a grant on each port, and the secrets those imply,
4. the target itself — its service, role and instance names, as a datasource
   named `target`.

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
| `secret` | The instance's own secret of a given name. |

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
role and instance before it reaches the page, and `required` says what was
missing and where it was looked for.

## The page

`dgs conf export` opens on the target tree.

### Selecting

One tree, nodes holding instances, with the tri-state checklist behaviour the
recipient checklist in [`cred/vault.md`](../cred/vault.md#the-flow) already
uses: `Space` toggles the item under the cursor, checking a node checks every
instance on it, an instance can then be unchecked on its own, and a parent shows
`[x]`, `[-]` or `[ ]` for all, some or none of its children.

```text
[x] us-sfo-dgo-linux-01
    [x] ss-sfo01          shadowsocks-rust / server
    [x] hy2-sfo01         hysteria2 / server
    [ ] bin-sfo01         microbin / server
[ ] jp-tyo-dgo-linux-01
[x] macbook
    [x] macbook-jp      hysteria2 / sing-box
    [x] macbook-sfo     shadowsocks-rust / ss-rust
```

Nodes rather than services at the top level, because a node is what an export is
for: the files a machine needs, or the files a person's device needs. A machine
usually wants a server of one service and a client of another, and a tree that
visits each service in turn to collect them makes the common export the long
one. Folding uses the File Explorer's keys, as the vault contents tree does.

Unmanaged users appear as their own top-level entries, holding the instances
derived for them, since they have no node.

A **broken** instance cannot be checked, and says why on the row.

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

`e` exports what is checked. The steps are the shared ones: a form for the
destination, then the confirmation dialog, then the result in the status bar.

- **Folder.** A directory, chosen with the File Explorer, defaulting to
  `conf.export.dir`.
- **Archive.** A `.zip`, named and placed the same way.

## What is written

Every target becomes its own directory, under the node it belongs to, whether or
not it is the only one:

```text
us-sfo-dgo-linux-01/
├── hysteria2/server/hy2-sfo01/config.yaml
├── microbin/server/bin-sfo01/server.env
└── shadowsocks-rust/server/ss-sfo01/config.json

macbook/
├── hysteria2/sing-box/macbook-jp/config.json
└── shadowsocks-rust/ss-rust/macbook-sfo/config.json
```

An unmanaged user's bundle is named for the user rather than a node, since there
is no node:

```text
friend-a/
├── hysteria2/link/friend-a-jp/share.txt
└── shadowsocks-rust/link/friend-a-sfo/share.txt
```

The layout says which machine each file is for, where it came from and what it
is, without a note beside it, and two instances of one role cannot collide. Both
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
- **How a command-line export fits the command model.** The TUI comes first and
  the command line follows it — see [Deferred](#deferred) — but a leaf command
  opens a TUI today, and reports are read-only, so an invocation that writes
  files is outside the current command model. That is the part still open, and
  it needs a decision in [`tui.md`](../../tui.md) rather than a flag added here.
  [`dgs conf secret show`/`edit`](inventory.md#viewing-and-editing-several-at-once)
  wait on the same decision — spawning `$EDITOR` is the same kind of
  side-effecting invocation, and neither should invent its own answer to it.

The per-instance `.sh` files are not among these: they are deleted with the
directory rename, not kept working alongside the manifest.

## Deferred

- **Exporting from the command line.** Decided, deferred, and not to be dropped:
  the TUI ships first, and a selector plus a destination on the command line
  follows it. The same export is repeated — the same hosts, for the same person —
  often enough that doing it by hand every time is the wrong end state, and it is
  what makes the renderer testable without driving a TUI. The page prints the
  equivalent command line after an export, so the second export is one line
  rather than the same tree walked again. What is still open is how a
  file-writing invocation fits the command model; see
  [Open questions](#open-questions).
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
4. **The page.** The tree, tri-state selection, the status bar's counts.
5. **Preview.** `Enter` on an instance, with errors shown as they come back.
6. **Export.** The destination form, the confirmation dialog, render-all-then-publish,
   folder and zip.
7. **Encrypted archives.** As deferred above.

The first of these were built before the inventory existed, against a target
that was a path and a secret that was a lookup key in a values file. The list
above is left as the record of that, and the work that revisits it is
[`inventory.md`](inventory.md#milestones), which covers both pages: discovery
gains the node files, rendering takes the render context in place of the
per-service secrets file, and selectors match fields in place of path segments.
The page, the preview and the export keep their shape; what changes under them
is the tree's top level, from services to nodes.
