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

## Adding a file or folder (E2)

Built before opening files, so the vault has something in it. Changing the
recipients of a file already there is E3 and not part of this.

### The flow

1. `a` on the vault page. The new file goes into the directory under the cursor,
   or the directory of the file under it; the last step can change that.
2. **Source.** The File Explorer in `ALL FILES` mode, so a file or a folder can be
   chosen, opening at the home directory with hidden entries hidden (`H` shows
   them).
3. **Recipients.** A checklist of groups, then hosts with their keys beneath
   them. Checking a group or a host checks every key under it, and a key can then
   be unchecked on its own; a group or host shows `[x]`, `[-]` or `[ ]` for all,
   some or none of its keys. The keys this machine holds an identity for start
   checked.
4. **Name.** The file name, editable, and the directory, changeable with the File
   Explorer.
5. **Confirm.** The shared confirmation dialog names the source, the file to be
   written and every recipient.
6. The result is reported in the status bar and the vault is scanned again.

Refused before confirming:

- no recipient is checked;
- the recipient folder has errors, since a host could be silently missing from a
  group;
- the file to be written, or its record, already exists — replacing a file is E3;
- the plaintext is larger than 64 MiB, the limit a file can later be opened
  with.

When no checked key belongs to this machine, the confirmation says the file
cannot be opened or checked here, and the result is published unverified.

### What is written

- A file is encrypted as it is, to `<name>.age`.
- A folder is archived as gzip-compressed tar, to `<name>.tar.gz.age`. The archive
  holds the folder by its name, every entry within it including hidden ones,
  with their permission bits. A symbolic link or any other non-regular entry
  refuses the folder rather than archiving something unexpected.
- The output is binary age, not armored.
- The source is not touched. The result reminds that its plaintext is still
  where it was.

### Publication

1. The plaintext is read into memory and its SHA-256 taken.
2. It is encrypted to a `<name>.age.dgs-part` file created beside the
   destination, which must not already exist, and synced.
3. The part file is decrypted back with this machine's identities and its
   SHA-256 compared; a mismatch removes it and fails.
4. It is linked to the destination name, which fails rather than replace a file
   that appeared meanwhile, and the part name removed.
5. The recipient record is written.

The plaintext buffer is overwritten when done, as far as the process can.

### The recipient record

Beside each file, `<name>.age.json`:

```json
{
  "version": 1,
  "created": "2026-09-15T14:00:00+10:00",
  "archive": "tar.gz",
  "recipients": [
    {
      "public_key": "age1ql3z7hjy54pw3hyww5ayyfg7zqgvc7w3j2elw8zmrj2kg5sfn9aqmcac8p",
      "host": "au-syd-macmini2018-linux-01",
      "description": "Main age key"
    }
  ]
}
```

- `archive` is `tar.gz` for a folder and absent for a file.
- A recipient is its public key; `host` and `description` are what the recipient
  folder called it when the file was written, kept for reading, not matched on.
- The record is plaintext and names hosts.
- The vault list shows only `.age` files, so records are not listed.

## Opening a file (C)

### Decrypting

`Enter` on a file in the vault list opens it:

- A **decryptable** file is decrypted with the identity that opens it.
- A **passphrase** file asks for its passphrase.
- A file that **needs a passphrase** asks for the passphrase of the protected SSH
  identity its header names, unlocks that identity for this one file, and
  decrypts with it.
- Other statuses do not open, and say why.

The passphrase is typed into a masked field in an overlay. A wrong one says so
and asks again; `Esc` gives up. It is used once and not kept: the next file asks
again, and the unlocked identity is discarded when the file has been decrypted.

Decryption stops at 64 MiB of plaintext, the limit a file can be added with.

### What is inside

The plaintext is recognised by content, not by name:

- **gzip-compressed tar**, **tar** or **zip** is expanded into a tree of entries,
  one level only: an archive inside it is an entry like any other file.
- Anything else is a single entry named after the file without `.age`.

An archive's expanded size is bounded by the same 64 MiB, so a small archive
cannot unpack into more than a file could hold. An archive entry whose name is
absolute or climbs out with `..`, or which is a symbolic link, hard link or
device, is listed and marked `!`; it has no content, and Actions that write
refuse it.

Each file entry is recognised as one of: **SSH private key**, **SSH public
key**, **age identity**, **text** (valid UTF-8 without NUL bytes), or **other**.
Directories are entries too.

### Layout

Opening moves the page to three columns, trailing-wide:

- **VAULT** — the vault list, dimmed, with the open file marked.
- **CONTENTS** — the entries, as a tree for an archive.
- **PREVIEW** — the selected entry: its path, type, size, mode and SHA-256, then
  its content, scrollable. Text is shown with long lines wrapped. An SSH private
  key or age identity is shown masked — its type and, where it has one, its
  public key and fingerprint — until `v` shows its content; `v` again hides it.
  Other content is described, not shown.

Above the entries, when the file has a recipient record, PREVIEW's first view of
the file names the hosts and keys it was encrypted to; without a record only the
header's view from browsing is shown.

`c` copies the selected entry to the clipboard through OSC 52, when it is text
or an SSH public key of at most 64 KiB — terminals cap what OSC 52 carries.
Private keys are never copied, since the clipboard is readable by other
programs and outlives `dgs`; the status bar says that what was copied stays on
the clipboard until replaced.

`Tab`, `Shift+Tab` and `Alt+←/→` move between the three columns. The breadcrumb
names the open file.

### Closing

The open file is closed, and its plaintext discarded, when:

- `Esc` is pressed in any of the three columns;
- another file is opened from VAULT;
- the page is left;
- nothing is pressed or clicked for `close_after`, five minutes by default; see
  [`configuration/cred.md`](../../configuration/cred.md). The status bar says
  that it closed and why.

Only one file is open at a time. Closing overwrites the plaintext buffers `dgs`
holds with zeros before dropping them. That is as far as a process can go:
`dgs` cannot guarantee the plaintext was never swapped to disk, or that no other
copy — made by the Go runtime, or shown on the terminal and kept in its
scrollback — survives, and does not claim to.

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
4. **E1 — Recipient record.** Read and write the record.
5. **E2a — Sealing.** Archive, encrypt, verify and publish, with filesystem
   tests.
6. **E2b — Adding from the page.** The flow above.
7. **C1 — Opening.** Decrypt into memory, expand archives, recognise entries, bound
   sizes, clear on close. A package of its own.
8. **C2 — Passphrases.** Passphrase files and protected SSH identities.
9. **C3 — The opened page.** Three columns, preview and masking, idle close.

