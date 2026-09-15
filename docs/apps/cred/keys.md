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
    au-syd-macmini2018-linux-01.json
    mobile-mbp2018-mac-01.json
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
  "keys": [
    {
      "public_key": "age1ql3z7hjy54pw3hyww5ayyfg7zqgvc7w3j2elw8zmrj2kg5sfn9aqmcac8p",
      "description": "Main age key"
    },
    {
      "public_key": "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI...",
      "description": "SSH host key"
    }
  ]
}
```

- The top level is an object, so a host can gain fields without the file
  changing shape.
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
- An SSH key's SHA256 fingerprint, as `ssh-keygen -l` shows it, is computed by
  `dgs` for display, not stored. An age key is short enough to show whole and has
  no fingerprint.
- A host with no keys is a warning.

### A group

```json
{
  "hosts": ["au-syd-macmini2018-linux-01", "mobile-mbp2018-mac-01"]
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

`dgs cred keys` is read-only. It lists:

- **Identities** — the file, its type, its derived public key, whether it is
  passphrase-protected or unsupported, the host it matches, and any read error
  or warning.
- **Hosts** — each host with its keys: description, key type, shortened public
  key and fingerprint, and whether this machine holds the matching identity.
- **Groups** — each group with its hosts.

A public key can be copied, and shown in full with its fingerprint.

## Milestones

1. **A1 — Recipient folder.** Load `credentials.json`, and load hosts and
   groups with the errors and warnings above. `internal/cred/recipients`, with no
   TUI code, tested on constructed folders.
2. **A2 — Identities.** Discover and parse identities in the configured
   directories, derive their public keys, match them against recipients.
   `internal/cred/identities`.
3. **A3 — Keys page.** The read-only page above, inside the `dgs cred` command.
4. **A4 — Unlocking protected identities.**

## Not decided

- How a passphrase is asked for, and how long an unlocked identity stays
  unlocked.

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
