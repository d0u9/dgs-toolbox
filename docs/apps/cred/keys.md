# Credentials: Keys

`dgs cred` works with a folder of [age](https://age-encryption.org) encrypted
files. Before it can say which of those files this machine can open, or encrypt
a new one for other machines, it has to know two things: the private keys this
machine holds, and the public keys of the machines it knows about. This
document covers those two, and the `dgs cred keys` page that shows them.

The rest of `dgs cred` — browsing encrypted files, decrypting in memory,
Actions, encrypting and changing recipients — is outlined in
[`roadmap.md`](roadmap.md) and designed when it is reached.

## Terms

- **Identity** — a private key on this machine that can decrypt age files.
- **Recipient** — a public key that files can be encrypted for.
- **Host** — a named machine owning one or more recipients.
- **Group** — a named set of hosts.

## Settings

Where identities are searched for and where the recipient folder is are set in
`credentials.json`, a file of its own beside `dgs-config.json`, not inside it.
The keys are in [`configuration/cred.md`](../../configuration/cred.md).

Identities are local to one machine and differ between machines, so the list of
identity directories lives in that local file. Recipients are the same on every
machine, so they live in a separate folder that can be shared.

## Identities

Identity directories are searched recursively. A file is recognised by its
content, never by its name:

- an age X25519 key file: one or more `AGE-SECRET-KEY-1…` lines;
- an OpenSSH private key (`-----BEGIN OPENSSH PRIVATE KEY-----`), ed25519 or RSA;
- a PEM RSA private key.

Rules:

- A configured directory that does not exist is reported and skipped.
- Files larger than 128 KiB are skipped without being read.
- A symbolic link to a directory is not followed.
- A private key file readable by anyone other than its owner (wider than
  `0600`) is used, with a warning.
- A file reached through two configured directories, or through a link, is read
  once, and shown under its real path.
- A file that cannot be read is reported with its path.
- A passphrase-protected key is listed and marked as protected. Unlocking it is
  a later milestone. An OpenSSH key keeps its public key outside the
  encryption, so a protected one is still matched against recipients; a legacy
  encrypted PEM key does not, and is matched only once unlocked.
- A private key age cannot decrypt with — ECDSA, an RSA key shorter than 2048
  bits — is listed as unsupported with the reason. So is a post-quantum age
  identity.
- In an age key file, a line that is neither a key, a comment nor blank is
  listed as invalid, since age refuses such a file.
- An age plugin identity (`AGE-PLUGIN-…`) is recognised and listed as
  unsupported, neither used nor silently ignored. Plugins work by running an
  external program, which `dgs` does not do.

The public key of each identity is derived from it. An identity whose public
key matches a recipient belongs to that recipient's host; one matching no
recipient is shown as **unregistered**, with its public key, so it can be added
to the recipient folder.

## The recipient folder

```text
<recipient folder>/
  hosts/
    server-linux-01.json
    laptop-mac-01.json
  groups/
    g-servers.json
```

One file per host and one file per group. The file name, without `.json`, is
the name; it is not repeated inside the file.

- A missing `hosts/` or `groups/` is empty, not an error.
- Files not ending in `.json`, and hidden files, are ignored.
- Subdirectories of `hosts/` and `groups/` are not read, and are reported: two
  of them could otherwise hold hosts of the same name.
- Unknown fields are an error, as in `dgs-config.json`.
- The files carry no format version.

### A host

```json
{
  "comment": "the black box under the desk",
  "keys": [
    {
      "public_key": "age1ql3z7hjy54pw3hyww5ayyfg7zqgvc7w3j2elw8zmrj2kg5sfn9aqmcac8p",
      "description": "Main age key"
    },
    {
      "public_key": "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI...",
      "description": "SSH host key",
      "origin": "added",
      "added": "2026-09-15",
      "comment": "root@nas"
    }
  ]
}
```

- The top level is an object, so a host can gain fields without the file
  changing shape.
- `comment` is optional: the owner's note about the machine — whose it is,
  where it stands — which a host name does not say. It is plaintext in a folder
  that is shared, so it holds no secret. `dgs` never writes it by itself; it is
  edited on the keys page or by hand, and is kept as it is when a key is added,
  removed or the host is renamed.
- `public_key` is an age X25519 recipient (`age1…`), or an `ssh-ed25519` or
  `ssh-rsa` public key. A trailing comment on an SSH key is not part of the key.
  An RSA key shorter than 2048 bits is an error. Other SSH key types, which age
  cannot encrypt to (`ecdsa-sha2-*`, `sk-ssh-ed25519@openssh.com`), are an
  error, as are `authorized_keys` options before an SSH key. Post-quantum
  (`age1pq1…`) and plugin (`age1yubikey1…`) recipients are an error naming what
  they are.
- The same public key twice in one host is an error.
- `description` is required and must not be blank: a key is never shown as
  only a string of characters.
- `origin`, `added`, `private_key` and `comment` are optional. `dgs` fills them
  in when it adds a key, so that `description` is left for what only its owner
  knows:
  - `origin` — how the key came to be listed: `generated`, `imported`,
    `registered` or `added`; anything else is an error.
  - `added` — the date it was listed, `YYYY-MM-DD`; another format is an error.
  - `private_key` — where the private key lives on its own host, when `dgs`
    knows: the identity file it wrote or registered.
  - `comment` — the comment an SSH public key line carried, often `user@host`.
    The key itself is written without it.
- An SSH key's SHA256 fingerprint, as `ssh-keygen -l` shows it, is computed by
  `dgs` for display, not stored. An age key is short enough to show whole and has
  no fingerprint.
- A host with no keys is a warning.

### A group

```json
{
  "hosts": ["server-linux-01", "laptop-mac-01"]
}
```

Groups contain hosts only; an empty group is allowed; a group cannot contain another group. When a group
is chosen for encryption it expands to its hosts, and each host to its keys,
any of which can then be deselected one by one.

### Names

Host names follow the host-identifier convention by habit, but `dgs` does not
enforce it; any plain name is accepted within these limits:

- letters, digits, `-`, `_` and `.`; not starting with `.`;
- a host name must not start with `g-`, and a group name must;
- names are compared case-insensitively: `HostA` and `hosta` are the same name,
  and two of them are an error.

### Errors and warnings

A file with an **error** is left out, and the rest of the folder loads: the
folder is edited by hand, and one mistake should not hide every other host. The
error is shown on the keys page with its file. Errors are an invalid file name,
invalid JSON or an unknown field, and the key rules above; two names that differ
only in case are an error on both files.

A **warning** leaves the file loaded:

- a host with no keys;
- a group naming a host that does not exist (the other members stay), or naming
  the same host twice;
- the same public key under two hosts;
- a subdirectory of `hosts/` or `groups/`;
- an identity on this machine that matches no recipient.

## The keys page

`dgs cred keys` is read-only. `dgs cred` opens a picker, since more commands
will join it.

It has three tabs — **IDENTITIES**, **HOSTS**, **GROUPS** — switched with `[`
and `]` or a click. Each uses the two-column, leading-narrow skeleton: a list on
the left, the selected entry on the right.

- **Identities** — each identity as a two-row item: its path (with `~`, and
  `:line` for an age key), then its type, status and the host it matches or
  *unregistered*. The right shows the path, type, status and reason, matching
  host and key description, warnings, the full public key and an SSH key's
  fingerprint. An unregistered identity says to add its public key to
  `hosts/<name>.json`.
- **Hosts** — each host, marked `!` when it has warnings. The right shows the
  file, the groups it belongs to, its comment or that it has none, its warnings, and its keys as a list: the
  description, then the type and shortened key or fingerprint, marked when this
  machine holds the identity. The full public key of the key under the cursor is
  shown below. The key list is its own data field, reached with `Tab` or
  `Alt+→`.
- **Groups** — each group; the right shows the file, its hosts with their key
  counts, and its warnings.

A problem that belongs to no loaded entry — a file left out with an error, a
subdirectory, an identity directory that does not exist, a file that cannot be
read — is listed under a **PROBLEMS** divider at the end of the list in the tab
it concerns, and selecting it shows the problem.

When `credentials.json` does not exist, or names no recipient folder, the lists
say so and name where the file is looked for.

Keys: `↑↓`/`j k` move, `gg`/`G` jump, `c` copies the selected public key — an
identity's, or the key under the cursor in a host — and `R` reloads everything
from disk. Copying uses the terminal's OSC 52 clipboard sequence, so no external
program runs and it also works over SSH; a terminal without OSC 52 support
copies nothing.

## Creating and registering keys

The keys page is read-only apart from these actions, all of which write only
after the shared confirmation dialog. Everything else in the recipient folder is
still edited by hand.

| Key | Where | Action |
| --- | --- | --- |
| `n` | Identities | **Generate** a new age X25519 identity and register its public key. |
| `i` | Identities | **Import** an age identity file from elsewhere and register its keys. |
| `r` | Identities | **Register** the unregistered identity under the cursor, where it is. |
| `d` | Identities | **Delete** the identity under the cursor, or only unregister it; see below. |
| `d` | a host's keys | **Unregister** the key under the cursor. |
| `d` | Hosts | **Delete** the host under the cursor, with all its public keys; see below. |
| `a` | Hosts | **Add** a public key from another machine — a cloud server that never runs `dgs` — to a host. |
| `r` | Hosts | **Rename** the host under the cursor; see below. |
| `m` | Hosts | **Comment**: write, change or remove the host's note. |

### Where a new identity goes

Generated and imported identities are written to `new_identity_dir` in
`credentials.json`, `~/.config/age` by default. The directory is created with
mode `0700` when missing, and each identity file is written `0600`. When that
directory is not among `identities`, the confirmation says so, since the new
key would not be found again until it is added; `dgs` does not edit
`credentials.json` itself.

- A generated file has the layout `age-keygen` writes: `# created:` and
  `# public key:` comments above the `AGE-SECRET-KEY-1…` line.
- An imported file must be an age identity file with at least one usable key;
  its content is copied as it is. Importing SSH private keys is not offered —
  they belong in `~/.ssh`.
- An existing file is never replaced: writing goes to a temporary file in the
  same directory, which is synced, parsed back, and linked to the final name.
- Identities are not passphrase-protected. The result reminds that the private
  key exists only on this machine, so a file encrypted to it alone is lost with
  it.

### The form

A form in an overlay, then the confirmation. It is filled in one pass: the first
field opens already being edited, and `Enter` — or `Tab` and the arrows — keeps
what was typed and goes on to edit the next field, until the button at the
bottom-right is reached; `Enter` there continues. `Esc` while editing undoes that
field's edit, and otherwise goes back. Pasting into a field works as typing does.

- **Host** — required, asked every time. It starts empty, or with the host this
  machine's identities already belong to when there is exactly one. It is a
  combo: a new name is typed, and `Space` opens the hosts that already exist to
  choose one. It follows the name rules above; a name matching an existing host,
  ignoring case, adds to that host.
- **File** — for generate and import, the identity file name, `<host>.agekey` by
  default. Not `.txt`, which reads like a note that is safe to delete.
- **Description** — required, and starts empty: it is for what only the owner
  knows. What `dgs` knows is recorded in the key's other fields instead —
  `origin`, `added`, `private_key` and `comment` — and shown under the key on the
  Hosts tab.

### Adding a public key

For a machine that never runs `dgs`, its public key is pasted in. The form asks
for the host (the host under the cursor to start with, and choosable from the
existing hosts), the public key — an `age1…` recipient or an `ssh-ed25519` /
`ssh-rsa` line — and the description. The public key is a text area: it is
longer than the form is wide, so it wraps over as many rows as it needs. It is recorded with
origin `added`, the date, and an SSH line's comment. The key
is checked when continuing, and one already listed under any host is refused.

### Registering

The public key is added to `hosts/<host>.json`, created when missing. Refused:

- the host file has an error, since rewriting it would lose what could not be read;
- the key is already listed, under this host or another.

The host file is rewritten through a temporary file and renamed into place,
keeping its other keys and all their fields in order. Keys are written in
canonical form; an SSH key's comment moves to its `comment` field when `dgs`
adds the key.

### Renaming

`r` on a host asks for a new name in a one-field form, then confirms. The name
follows the host name rules above. A name another host already has is refused:
keys are not merged here. A name that differs only in case is a rename, since
that is what the host is called on the keys page.

The host file is renamed, not rewritten, so every key and every field it
carries stays as it is. Its public keys do not change, so files already
encrypted to the host can still be opened. Every group naming the host is
rewritten to the new name; as when a host is deleted, each group is rewritten
from its file, so members that did not resolve are kept. Nothing records the
old name, so anything outside the recipient folder that names the host is
changed by hand.

### The comment

`m` on a host opens one field holding its comment, and `Enter` confirms. An
empty comment removes the field. Only the comment changes: the keys and every
field they carry stay as they are, since the file is rewritten from what was
loaded. A host whose file has an error is not loaded, so it cannot be commented
until the file is fixed.

### Deleting

`d` on an identity deletes both halves when the identity is `dgs`'s own — an age
key in `new_identity_dir`, alone in its file. The file is moved to the trash
(`~/.Trash` on macOS, the freedesktop.org trash elsewhere), not removed, and the
public key is taken out of every host listing it. A trash on another volume is
refused rather than copied to.

Any other identity — an SSH key in `~/.ssh`, an age key elsewhere or sharing its
file with other keys — is only unregistered: its public key is removed and the
private key file is left alone, since it may have uses `dgs` does not know about.
An identity that is neither `dgs`'s own nor registered cannot be deleted here.

`d` on a key in a host's key list unregisters that key only.

`d` on a host in the Hosts list deletes the host: its file, with every public key
in it, goes to the trash, and it is taken out of every group naming it — the group
files are rewritten from disk, so members that did not resolve are kept. It
leaves the groups first, so a failure there leaves the host file in place. Private
keys on this machine for the host's keys are not deleted, and the confirmation
says so when there are any.

A host left with no keys keeps its file, which then warns that it has none.

The confirmation, focused on **Keep**, reads the vault's recipient records and
says how many files are encrypted to the key, and names those encrypted only to
it, which cannot be opened again once the private key is gone. Files without a
record are not known about, and an unreadable record is said to make the answer
incomplete.

## Milestones

1. **A1 — Recipient folder.** Load `credentials.json`, and load hosts and
   groups with the errors and warnings above. `internal/cred/recipients`, with no
   TUI code, tested on constructed folders.
2. **A2 — Identities.** Discover and parse identities in the configured
   directories, derive their public keys, match them against recipients.
   `internal/cred/identities`.
3. **A3 — Keys page.** The read-only page above, inside the `dgs cred` command.
4. **A5 — Creating keys.** Writing identity files and adding keys to hosts,
   in their packages; then the three actions on the page.
5. **A4 — Unlocking protected identities.** Built with the vault's C2: a
   passphrase is asked for in a masked field when a file needs it, unlocks the
   identity for that one file or recipient change, and is not kept. See
   [`vault.md`](vault.md#decrypting).

## Deferred

- **Refusing to encrypt with an incomplete folder.** A left-out file means a
  host may silently be missing from a group. Encryption, when it is built, should
  refuse or ask while the folder has errors.
- **Naming `credentials.json` by a flag or environment variable.**
- **Recipient trust confirmation.** Only the owner writes the recipient folder,
  so for now every recipient in it is trusted. If that changes, a key that is
  new, replaced or renamed should be confirmed on this machine before it can be
  encrypted for, in the manner of SSH's `known_hosts`. Decryption is unaffected
  because it uses only local identities.
- **Editing recipients from the page.** The folder is edited by hand.
- **More key metadata** — kind (SSH user or host key), status (active or
  retired), history. Added when something needs them.
- **Post-quantum age keys** (`age1pq1…`). Not used. age will not mix them with
  other recipients in one file, so supporting them would shape how recipients
  are chosen.
- **Age plugin identities** — YubiKey, Secure Enclave, TPM. Would have to be
  compiled into `dgs` rather than run as plugins.
