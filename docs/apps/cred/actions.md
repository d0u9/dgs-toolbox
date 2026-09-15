# Credentials: Actions

What can be done with an entry of an opened vault file — see
[`vault.md`](vault.md#actions-d). Actions are the only way decrypted content
leaves memory, so each says exactly where it puts it. A test checks that this
document names every Action `dgs` registers.

`Enter` on a file entry in CONTENTS lists the Actions for its kind. Choosing one
opens its form; continuing shows the shared confirmation dialog naming every path
to be written; confirming runs it and reports in the status bar.

| Action | Kinds | Puts the content |
| --- | --- | --- |
| [`ssh.install`](#sshinstall) | SSH private key | in `~/.ssh/keys/`, `~/.ssh` or a chosen folder, with its `.pub` and optionally a `Host` entry in `~/.ssh/config.d/` |
| [`ssh.agent`](#sshagent) | SSH private key | in the running ssh-agent |
| [`ssh.config`](#sshconfig) | text | in `~/.ssh/config.d/` |
| [`age.install`](#ageinstall) | age identity | in `new_identity_dir` |
| [`file.save`](#filesave) | SSH private key, SSH public key, age identity, text | in a chosen folder |

Other kinds — binary files, directories, unsafe entries — have no Actions yet.

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
  ```

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
