# Configuration: Export

`dgs conf export` renders service configuration files from templates, per-instance
values and secrets, and writes a chosen set of them to a folder or a zip
archive.

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

- **Per-instance boilerplate.** An instance should be a values file and nothing
  else.
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

One directory holding one subdirectory per service, named by `conf.root`; see
[configuration](#configuration). A subdirectory is a service when it holds a
`confgen.yaml`, so a `README`, a scratch folder or a service still being written
is skipped rather than half-read.

### The service manifest

`<service>/confgen.yaml` declares what the renderer cannot infer:

```yaml
secrets: shadowsocks-rust.yaml

roles:
  server:
    template: templates/server.json.tmpl
    defaults: element
    output: config.json
  client:
    template: templates/client.json.tmpl
    defaults: element
    output: config.json
```

| Key | Meaning |
| --- | --- |
| `secrets` | The secrets file this service draws on, named relative to `conf.secrets`. |
| `roles` | One entry per kind of instance the service generates. The key is the role name, and also the directory its value files are in. |
| `roles.<role>.template` | The template rendered for the role, relative to the service directory. |
| `roles.<role>.defaults` | How the role's defaults apply: `document` or `element`. See below. |
| `roles.<role>.output` | The name the rendered file is written under — the name the service expects where it runs, such as `config.json` or `server.env`. |

The role name is the directory name and the defaults file is always
`defaults.yaml` in it, so neither is declared. That is the naming unification
above spent on making the manifest shorter rather than on describing four
spellings.

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

Every `*.yaml` in a role's directory other than `defaults.yaml` is an instance,
named by its file name without the extension. Nothing registers it: adding an
instance is adding a file, and the next run lists it.

A file that is not valid YAML, or that is not a mapping, is listed as **broken**
with the parse error, rather than omitted — an instance that silently stops
appearing is worse than one that appears with a reason it cannot be rendered.

### An instance is not a machine

One machine may hold several instances of one role: two Hysteria2 services, or
several Shadowsocks clients reaching different servers. An instance is one
rendered configuration file, and the machine it ends up on is not something the
generator knows.

Two things follow, and they are the whole reason this is written down:

- **The secrets lookup key is a field in the values, never the file name.** It
  is what it is today — Hysteria2's `server`, Shadowsocks' per-entry
  `secret_server` — and the renderer does not replace it with the instance name.
  A Shadowsocks client's `servers` list carries one key per entry, so there is
  no single name for the file to supply anyway.
- **The instance name is free.** It is an identifier for the reader, not a host
  name the renderer parses. Names that say what the instance is for read best;
  nothing checks them.

### Output names do not vary

A role's `output` is one name for every instance of it, because the service
reading it expects one name wherever it runs. Two clients reaching different
servers both render to `config.json`, and are told apart by the directory the
export puts them in, not by the file name. Copying the right one to the right
machine is done by hand and deliberately so: a name that varied per instance
would mean editing a service unit to match every time.

### Targets and selectors

The unit of everything below is a target, written `<service>/<role>/<instance>`:

```text
hysteria2/server/us-sfo-dgo-linux-01
shadowsocks-rust/client/us-sfo-dgo-linux-01
```

A selector is a target with `*` matching within one segment and `**` matching
the rest:

```text
hysteria2/**                    every role and instance of one service
*/server/us-sfo-*               the server side of one machine, across services
**/us-sfo-dgo-linux-01          every instance of that name, whatever its role
```

A selector matching no target is an error naming the selector, not an empty
export.

`dgs conf --targets` writes every target found, with its status, to stdout and
returns without opening the TUI, as [App reports](../../tui.md#app-reports)
describes. It is how a reader checks what the generator root holds, and what a
selector will match, without rendering anything.

## Rendering

A target renders from four inputs:

1. the role's `defaults.yaml`,
2. the instance's values file, applied over the defaults at the level the role
   declares,
3. the service's secrets file,
4. the target itself — its service, role and instance names, as a datasource
   named `target`.

The fourth is available to templates and used by none of them today. It is not
a replacement for the secrets lookup key: as
[above](#an-instance-is-not-a-machine), that key stays in the values, because an
instance is not a machine and one file may carry several keys. It is a separate
datasource rather than a key merged into the values so that the values stay
exactly what the service's own configuration says, with nothing reserved inside
them.

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
| `secret` | The secrets entry for a lookup key. |

`secret` is new, and replaces the loop every template currently opens with: each
walks the whole secrets file with `coll.Has` to find the entry whose `servers`
list holds its key. The renderer holds the secrets, so it does the lookup, and a
key that matches nothing is an error naming the key rather than an empty
`dict` that fails further down.

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

One tree, services holding roles holding instances, with the tri-state checklist
behaviour the recipient checklist in [`cred/vault.md`](../cred/vault.md#the-flow)
already uses: `Space` toggles the item under the cursor, checking a service or a
role checks every instance under it, an instance can then be unchecked on its
own, and a parent shows `[x]`, `[-]` or `[ ]` for all, some or none of its
children.

```text
[x] hysteria2
    [x] server
        [x] us-sfo-dgo-linux-01
        [ ] jp-tyo-dgo-linux-01
    [ ] client
[ ] microbin
[x] shadowsocks-rust
    [x] server
        [x] us-sfo-dgo-linux-01
    [x] client
        [x] us-sfo-dgo-linux-01
```

One tree rather than a service picker followed by a page per service: the same
host usually wants a server here and a client there, and a flow that visits each
service in turn to collect them makes the common export the long one. Folding
uses the File Explorer's keys, as the vault contents tree does.

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

Every target becomes its own directory, whether or not it is the only one:

```text
us-sfo-dgo-linux-01/
├── hysteria2/server/us-sfo-dgo-linux-01/config.yaml
├── microbin/server/us-sfo-dgo-linux-01/server.env
└── shadowsocks-rust/client/us-sfo-dgo-linux-01/config.json
```

The layout is the selector, so the archive says where each file came from and
what it is for without a note beside it, and two instances of one role cannot
collide. The instance directory is kept in the single-target case too: a layout
that changes shape with the number of targets is one a script reading it has to
handle twice.

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

- The generator root's secrets are kept out of version control by name
  (`*secrets*/`). An export destination is not covered by that rule, so
  `conf.export.dir` defaults outside any repository, and a destination inside
  one is a warning on the confirmation dialog naming the repository.
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
| `conf.root` | The directory holding one subdirectory per service. | empty — the page opens with no root and asks for one |
| `conf.secrets` | The directory a manifest's `secrets` file is named relative to. | empty — a service naming a secrets file refuses to render |
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
