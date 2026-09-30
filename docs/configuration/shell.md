# Shell configuration

What the shell itself reads: where the configuration lives, and what the top
bar shows. The design behind it is in [`tui.md`](../tui.md).

`<config dir>/shell/config.json`:

```json
{
  "top_bar": {
    "disk": true,
    "network": true,
    "cpu": true,
    "time": true
  }
}
```

## The configuration directory

The configuration is a directory, found by `--config` / `-c`, then
`DGS_TOOLBOX_CONFIG`, then `$XDG_CONFIG_HOME/dgs-toolbox/`
(`~/.config/dgs-toolbox/` when that variable is unset). That last one is the
only default location. `--config` deliberately bypasses the environment
variable, so a run can point at a whole configuration without changing the
shell it was launched from. Naming a file rather than a directory is refused.

Every command has a folder of its own in it, holding its `config.json` and
whatever else it reads from disk:

```text
<config dir>/
  shell/config.json
  capture/config.json
  capture/recipes/ workflows/ mappings/ templates/
  photo/config.json
  box/config.json
  doc/config.json
  geo/config.json
  geo/gpx/tiles.json
  conf/config.json
  cred/config.json
```

The commands are independent, so their configuration is too:

- A command reads only its own folder. Nothing in one command's file affects
  another.
- A file that is refused — an unknown key, a bad value — refuses only its own
  command, which says which file and why when it is started. The other
  commands run as usual. The shell's own file is the exception: the shell
  reads it before any command, so a refused `shell/config.json` stops `dgs`.
- A command that one day leaves the toolbox takes its folder with it, and its
  file does not change.

The layout is not configurable: `--config` moves the whole thing, and a
directory that genuinely belongs elsewhere is a symlink. See
[Capture](capture.md#what-sits-beside-the-configuration-file) for what it reads
from its folder.

A missing directory or file leaves every setting at its default. An unknown key
is rejected: the file is refused with the name of the key rather than applied
in part.

A `dgs-config.json` or `credentials.json` directly in the configuration
directory is the single-file layout this one replaced. It is not read, and
`dgs` refuses to start until it is removed, so settings someone wrote never
quietly stop applying.

## Exporting the defaults

`dgs --export-config [dir]` writes a complete default `config.json` for every
command into `dir` — the working directory when it is omitted — and prints
each path. A file that already exists is kept and reported as kept, so
exporting into a configuration in use adds only what it lacks.

## The top bar

Each key removes one fixed-width cell and its separator when set to `false`.
All four default to `true`; omitting a key preserves that default. The
breadcrumb is structural and cannot be disabled.

| Key | What it shows |
| --- | --- |
| `top_bar.disk` | disk read and write rates |
| `top_bar.network` | network up and down rates |
| `top_bar.cpu` | CPU utilization |
| `top_bar.time` | the local clock |

With `disk`, `network` and `cpu` all `false`, the shell does not start the
system-counter sampler at all.

## Web servers

Every command that serves pages has a `web` key: a list of servers, each named
by its use. A command may come to serve more than one page, so it is a list
from the start rather than one host and port.

```json
"web": [
  {"name": "browse", "port": 8766}
]
```

| Key | Meaning |
| --- | --- |
| `web[].name` | Which of the command's servers the entry is for. Required. A name the command does not have, or a name listed twice, is refused. |
| `web[].host` | The address to listen on. Only a server that may be reached from another machine accepts it; for the others a host is refused. Empty uses the server's default. |
| `web[].port` | The port to listen on. `0` uses the server's default. |

A server with no entry listens at its defaults. The servers each command has
are listed in its own document.

## Local caches

A command with a discardable local cache has a `cache.dir` key. Empty is
`dgs/<command>` under the operating system's user cache directory
(`~/Library/Caches/dgs/<command>` on macOS).
