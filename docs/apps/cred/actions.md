# Credentials: Actions

## CLI listing

- `dgs cred key hosts` lists recipient host names and their public-key counts.
- `dgs cred key groups` lists recipient group names and their resolved host members.

Both read `recipients` from `cred/config.json`, respect the global `--config`
option, print to stdout and return without opening a TUI, vault or private key.
An empty folder prints only the column headings. Missing configuration or an
unreadable folder fails. Folder errors return a nonzero exit status with
diagnostics on stderr after printing the loaded entries; the listing may be
incomplete. Warnings are printed on stdout after the table and do not change
the successful exit status. These commands change nothing.

## Standalone encryption and decryption

```sh
dgs cred e notes.txt -H host-a -G g-team
dgs cred d notes.txt.age -o restored.txt
```

`e` (alias `encrypt`) accepts one regular file or folder. Repeat
`-H` / `--host` and `-G` / `--group`
to select recipients from the configured recipient folder; each host contributes
all its public keys and duplicate keys are used once. Names are matched without
case sensitivity; groups use their full `g-` name. Empty selections, unknown
names, empty selected hosts/groups and recipient-folder errors refuse encryption.
Warnings (such as a public key shared by hosts) are printed but do not block
encryption or completion.
A folder becomes a gzip-compressed tar archive, using the configured
`archive_skip` rules. The command reports the skipped entry count.

`d` is the short command for `decrypt`; both long spellings remain aliases.
Like filename-oriented compression tools, output names are inferred: encryption
adds `.age` (directories add `.tar.gz.age`); decryption removes a final `.age`.
Without that suffix, decryption requires `-o`. Use `--output` (`-o`) to choose a
different file, rather than an output directory. Both commands respect the
global `--config` option. `-h` remains help; hosts use uppercase `-H`.
Missing/extra source arguments, unknown options, missing option values and empty
host/group/output values fail. Use `--` before a source name beginning with `-`:

```sh
dgs cred e -G g-team -- -notes.txt
```

Source symbolic links and special files are refused. An existing output file,
directory or symbolic link (including a dangling link) refuses the operation.
Encryption requires an existing output parent directory. Decryption may create
missing parents as described below. Neither stdin nor stdout streaming is
supported. Unknown recipient names report the corresponding listing command;
recipient-folder problems and unavailable identities include diagnostics.
They do not use the configured vault or write a recipient record. Existing
outputs are never replaced and sources are unchanged. Output inside a source
directory is refused, including through a symlinked parent. Both commands use
exclusive same-directory `.dgs-part` files and streamed SHA-256 readback before
publication. File identity and version are checked before readback and
publication, which reuses the platform exclusive-publish primitive. Pre-existing
parts and replacements created by another writer are never removed. When a selected key has an unlocked local
identity, the ciphertext is decrypted back and checked by SHA-256 before
publication; otherwise the command explicitly reports it as unverified.

`d` accepts binary or armored age files and uses unlocked identities from
`cred/config.json`. Plaintext is streamed into a `0600` temporary file; only
after complete authenticated decryption and independent SHA-256 disk readback
succeed is its final name published. Failures and ordinary Ctrl+C/SIGTERM interruptions remove this operation's
own part file; forced termination or a crash can leave an unpublished part. Newly created parent directories are `0700`. `d` explicitly requests writing plaintext, at the inferred or chosen path. Archives remain archives: decrypt a folder's ciphertext to a
`.tar.gz` file, then extract separately. No entries are automatically extracted.

The CLI has no application file-size limit. File copying, directory archiving,
encryption, decryption and verification use bounded buffers rather than loading
whole files or archives into memory. Available disk space and filesystem limits
still apply. The vault TUI retains its separate 64 MiB limit.

Configured private identities are read into memory only; the CLI never writes
extra key files or invokes an external encryption helper. Temporary raw key
buffers are cleared after parsing. Go and the operating system may retain
internal copies; this is not a guarantee against memory swapping.

Passphrase encryption and unlocking protected identities are not supported by these CLI commands; use the
vault TUI for those workflows. No secrets are accepted in command-line flags.

## Help, examples and shell completion

`dgs cred --help`, `dgs cred e --help` and `dgs cred d --help` explain the
commands without opening a workspace. See the [worked examples](../../../examples/cred/README.md).

Enable completion in the current Bash session:

```bash
source <(dgs completion bash)
```

For Zsh, initialize completion once, then load dgs completion:

```zsh
autoload -Uz compinit && compinit
source <(dgs completion zsh)
```

For future sessions, add the corresponding line(s) to your `.bashrc` or
`.zshrc`; if Zsh already initializes `compinit`, keep only the `source` line.
Alternatively, `dgs completion bash` / `dgs completion zsh` prints a script
that can be saved and sourced. No helper executable is needed.

Press Tab after `dgs cred ` for commands, after an option prefix for flags,
and in the source or `-o` value for filesystem paths. After `-H` / `--host` or
`-G` / `--group`, candidates come from the active recipient folder and are
filtered by the typed prefix. Repeated recipient flags and long command aliases
work the same way. A supplied `--config` selects the completion configuration,
just as it does when running the command. Once the source is supplied, completion
does not suggest a second source. Host/group completion never falls back to
filenames, including when configuration is missing or invalid. Completion reads
public recipient definitions only and never executes a file operation.

## Vault entry Actions

What can be done with an entry of an opened vault file — see
[`vault.md`](vault.md#actions-d). Actions are the only way decrypted content
leaves memory, so each says exactly where it puts it. A test checks that this
document names every Action `dgs` registers.

`Enter` on an entry in CONTENTS lists the Actions for its kind. Choosing one
opens its form; continuing shows the shared confirmation dialog naming every path
to be written; confirming runs it and reports in the status bar.

| Action | Kinds | Puts the content |
| --- | --- | --- |
| [`ssh.install`](#sshinstall) | SSH private key | in `~/.ssh/keys/`, `~/.ssh` or a chosen folder, with its `.pub` and optionally a `Host` entry in `~/.ssh/config.d/` |
| [`ssh.agent`](#sshagent) | SSH private key | in the running ssh-agent |
| [`ssh.config`](#sshconfig) | text | in `~/.ssh/config.d/` |
| [`age.install`](#ageinstall) | age identity | in `new_identity_dir` |
| [`file.save`](#filesave) | SSH private key, SSH public key, age identity, text, other | in a chosen folder |
| [`dir.save`](#dirsave) | directory | in a chosen folder, as a folder of its own |

Unsafe entries — an absolute or climbing name, a link, a device — have no
Actions, and `dir.save` leaves them out of the folder it writes.

## Rules every Action follows

- **Nothing is replaced.** Every file an Action would write is checked before
  anything is written; if any exists, the Action refuses and writes nothing.
- **Written through a part file.** Content goes to a temporary file in the same
  directory, is synced, read back and compared by SHA-256, then linked to its
  name, which fails rather than replace a file that appeared meanwhile.
- **Private keys are `0600`**, and a directory an Action creates for them is
  `0700`.
- **Paths** accept a leading `~`.

## `ssh.install`

Installs an SSH private key, by default for logging in to one server.

| Field | Default | |
| --- | --- | --- |
| Name | the entry's name | The key's file name, and the `Host` name to type after `ssh` when a Host entry is written. Letters, digits, `-`, `_`, `.`. |
| Location | `~/.ssh/keys` | `~/.ssh/keys`, `~/.ssh`, or **Other…**, which shows Folder. |
| Folder | — | Shown for Other…; chosen with the File Explorer. |
| Write Host entry | checked | Writes `~/.ssh/config.d/<name>.conf`. The fields below are shown only when checked. |
| Host name | — | Required with a Host entry: the server's IP address or domain. |
| User | empty | Written as `User` when given. |
| Port | empty | Written as `Port` when given; 1–65535. |
| Jump host | empty | Written as `ProxyJump` when given, such as `bastion` or `alice@jump.example.com:2222`. |
| Proxy command | empty | Written as `ProxyCommand` when given, for routes a jump host cannot take, such as `ssh gateway nc -X 5 -x 127.0.0.1:1080 %h %p` through a SOCKS proxy. Written as it is, apart from refusing line breaks; `dgs` does not run it. Give a jump host or a proxy command, not both. |
| Multiplex connections | unchecked | `ControlMaster auto`, `ControlPath ~/.ssh/cm-%C`, `ControlPersist 10m`: sessions share one connection, kept 10 minutes after the last closes. Unchecked writes `ControlMaster no` and `ControlPath none`. SSH keeps the first value it reads and `config.d/*` is read in name order, so these lines lose to a `Host *` that comes earlier, such as one in a file whose name sorts first. |
| Keep alive | unchecked | `ServerAliveInterval 60`, `ServerAliveCountMax 3`: drops a connection after about 3 minutes without replies. |
| Forward agent | unchecked | `ForwardAgent yes`: the server can use the local ssh-agent. Use only for trusted servers. |
| Compression | unchecked | `Compression yes`. |
| True color | unchecked | `SetEnv COLORTERM=truecolor`. Applies only when the server's `sshd_config` has `AcceptEnv COLORTERM`. |
| Add Include | checked | Shown only when `~/.ssh/config` does not already include `~/.ssh/config.d/*`. |

`~/.ssh/keys` is the default because SSH tries keys with default names in
`~/.ssh`, such as `id_ed25519`, for every server; a key installed in `~/.ssh`
under such a name is offered everywhere.

Writes:

- `<location>/<name>` — the private key, `0600`, creating the folder `0700`.
- `<location>/<name>.pub` — its public key, derived from it. When that file
  already exists and holds the same key it is left alone; a different key there
  refuses the Action.
- With **Write Host entry**, `~/.ssh/config.d/<name>.conf`, `0600`:

  ```sshconfig
  Host <name>
      HostName <host name>
      User <user>
      Port <port>
      IdentityFile <location>/<name>
      IdentitiesOnly yes
      ControlMaster no
      ControlPath none
  ```

  plus the lines of each chosen option above; **Multiplex connections**
  replaces the two `Control` lines. Host name, User and Jump host may not
  contain spaces, line breaks or quotes.

  `IdentitiesOnly yes` makes SSH offer only this key, so many keys in the agent
  or in `~/.ssh` do not use up a server's authentication attempts.
- With **Add Include** checked, `Include ~/.ssh/config.d/*` as the first line of
  `~/.ssh/config`, which is created `0600` when missing. `Include` must come
  before any `Host` block to apply to every host, hence the first line. The rest
  of the file is kept byte for byte.

Without a Host entry only the key and its `.pub` are written, and no host name
is asked for.

A passphrase-protected key is installed as it is, still protected.

## `ssh.agent`

Adds an SSH private key to the running ssh-agent, through `SSH_AUTH_SOCK`. No
file is written.

| Field | Default | |
| --- | --- | --- |
| Lifetime | `1h` | How long the agent keeps the key, as a Go duration; `0` for as long as the agent runs. |
| Confirm each use | unchecked | The agent asks before every use of the key, as `ssh-add -c` does. |

The key's comment in the agent is `<vault file>/<entry path>`, so `ssh-add -l`
says where it came from. Refused when `SSH_AUTH_SOCK` is not set, and for a
passphrase-protected key, which this Action does not unlock.

## `ssh.config`

Installs a text entry — a `Host` block kept in the vault beside a key — as one
file of SSH configuration.

| Field | Default | |
| --- | --- | --- |
| Name | the entry's name without an extension | The file is `~/.ssh/config.d/<name>.conf`. |
| Add Include | checked | As for `ssh.install`. |

The text is written as it is, `0600`; `dgs` does not check that it is valid SSH
configuration.

## `age.install`

Installs an age identity as one of this machine's identities.

| Field | Default | |
| --- | --- | --- |
| File | the entry's name | The file in `new_identity_dir`. |

Writes the identity, `0600`, creating the directory `0700` when missing. The
content must parse as age identities. Its public key is registered separately,
with `r` on the keys page.

## `file.save`

Saves the entry to a folder.

| Field | Default | |
| --- | --- | --- |
| Folder | the home directory | Chosen with the File Explorer. |
| Name | the entry's name | |

A private key or age identity is written `0600`; anything else keeps the mode
it had in the archive, or `0600` for a single file.

## `dir.save`

Saves a directory of an opened archive, and everything under it, to a folder.
The top row of CONTENTS stands for the whole file, so the same Action saves an
archive in one go.

| Field | Default | |
| --- | --- | --- |
| Folder | the home directory | Chosen with the File Explorer. |
| Name | the directory's name, or the vault file's name for the whole file | The folder written inside Folder. |

Writes `<folder>/<name>/`, then every file and directory below the chosen one
under it, keeping the archive's structure. The whole thing is planned first, so
an existing `<folder>/<name>` refuses the Action before anything is written.

- A file keeps the mode it had in the archive; a private key or age identity is
  `0600` whatever the archive said.
- A directory is `0700` when a private key or age identity lives anywhere below
  it, so the keys inside a saved folder are not left readable by other users of
  the machine. Any other directory keeps the archive's mode, or `0755`.
- A directory the archive lists but puts nothing in is created too.
- Unsafe entries are not written. The confirmation dialog names each one, and
  the status bar says how many were skipped, so a folder that arrives
  incomplete says so.
