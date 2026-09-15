# Credentials: Vault

`dgs cred vault` works with a folder of age-encrypted files — the vault. It
lists them, says which this machine can open, opens one into memory, and runs
Actions on what is inside.

It builds on [keys](keys.md): the identities it decrypts with and the recipient
folder it names hosts from are the ones that page shows.

## Opening a folder

The vault folder is `vault` in `credentials.json`; see
[`configuration/cred.md`](../../configuration/cred.md). The page can also switch
to another folder with the shared File Explorer. With no `vault` configured the
page opens with no folder and asks for one.

## Browsing (B)

### Which files are listed

- The folder is searched recursively.
- Hidden files and directories (a name starting with `.`, such as `.git`) are
  skipped.
- A symbolic link to a directory is not followed.
- Only names ending in `.age` are considered. Other files — a README, images —
  are not read.
- A `.age` file is an age file when it starts with the binary header
  `age-encryption.org/v1` or the armored `-----BEGIN AGE ENCRYPTED FILE-----`.
  A `.age` file that does not is listed as **damaged**.

### What is known without decrypting

Only the header is read; the payload is not.

- **X25519 recipients (`age1…`) are anonymous.** The header carries a fresh
  ephemeral key per recipient, so the only way to know whether a file is for an
  identity is to try that identity.
- **SSH recipients carry a four-byte tag** of the public key they are for. A tag
  matching one of this machine's SSH identities says the file is for it, even
  while that identity is passphrase-protected. A tag matching an SSH key in the
  recipient folder names that host.

  Such a match is shown as a hint, labelled as SSH keys only, matched by tag and
  incomplete: four bytes can collide, and X25519 recipients never appear.
  A complete record of who a file is for belongs to encrypting (E).
- **A passphrase-encrypted file** (`age -p`) has a single scrypt stanza.

### Status

Each file gets one status, the first that applies:

| Status | Meaning |
| --- | --- |
| **damaged** | The header cannot be read, or an identity unwraps it but the header MAC does not verify. |
| **decryptable** | A usable identity on this machine opens it. The identity is named. |
| **needs passphrase** | An SSH tag matches a passphrase-protected identity on this machine. |
| **passphrase** | Encrypted with a passphrase rather than to recipients. |
| **unsupported** | Every recipient is of a kind `dgs` does not handle: a plugin or post-quantum recipient. |
| **not for this machine** | None of the above. |

The list marks each with a symbol as well as a colour: `✓` decryptable, `⚿`
needs passphrase or passphrase, `✗` not for this machine or unsupported, `!`
damaged.

### Scanning

The file list appears first; statuses fill in as files are checked, four at a
time. Switching folder or leaving cancels a scan in progress. `R` scans again.
Nothing is cached between scans.

### Layout

Two columns, leading narrow. The left is the folder as a tree, directories
expandable and collapsible, each file with its status mark. The right is the
selected file: its path, size and modification time, status and the identity
that opens it, the recipient stanzas in its header by kind and count, and the
SSH hints above.

## Opening a file (C)

Designed and agreed in outline; detailed when it is built.

- `Enter` on a decryptable file decrypts it into memory. The page moves to a
  three-column, trailing-wide layout: the vault dimmed on the left, the
  decrypted **contents** in the centre, and a **preview** with the Actions for
  the selected entry on the right. The breadcrumb names the open file.
- A single file is one entry. A zip or tar archive is expanded into a tree.
- Text is previewed; a binary shows its type, size and hash.
- Only one file is open at a time; opening another closes the first.
- `Esc` closes the file and discards its plaintext. So does a period without
  input, five minutes by default and configurable.
- A passphrase-encrypted file, or one needing a protected identity, asks for the
  passphrase when it is opened. The passphrase is used once and not kept.
- Plaintext is held in process memory. `dgs` cannot guarantee it is never
  swapped to disk or that every copy is overwritten, and does not claim to.

## Actions (D)

Designed and agreed in outline; catalogued in their own document when built.

- Each entry is recognised by content — SSH private key, SSH public key, age
  identity, text, other file, directory — and each Action declares the kinds it
  applies to.
- An Action runs in three steps: a form for its parameters, the shared
  confirmation dialog saying exactly what will be written where, then the
  result in the status bar.
- Actions are the only way plaintext leaves memory — to disk, to ssh-agent, to
  the clipboard — and each says where it puts it.
- Writing a file goes through a `.part` file, read back and checked against the
  hash, then renamed into place.
- An SSH private key is never copied to the clipboard; other text may be.

Initial Actions:

| Kind | Actions |
| --- | --- |
| SSH private key | Install to `~/.ssh`, add to ssh-agent, save to folder |
| SSH public key | Install to `~/.ssh`, copy, save to folder |
| age identity | Save to folder |
| Text | Copy, save to folder |
| Other file | Save to folder |
| Directory | Save to folder |

## Milestones

1. **B1 — Vault scan.** List the age files in a folder by the rules above.
2. **B2 — Header inspection.** Read a header, list its stanzas, try identities,
   match SSH tags, and give the status.
3. **B3 — Vault page.** The browsing page, with scanning in the background.

## Not decided

- Whether the archive format written when encrypting a folder is zip or tar;
  reading expands both.
