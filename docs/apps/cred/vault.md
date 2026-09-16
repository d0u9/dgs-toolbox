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
   - `p` opens **Passphrase** instead: the passphrase and a repeat of it, in
     masked fields; an empty one or a mismatch is refused. age does not combine
     a passphrase with recipients, so the checked keys are not used. `Esc`
     returns to the checklist and forgets what was typed.
4. **Name.** The file name, editable, and the directory, changeable with the File
   Explorer.
5. **Confirm.** The shared confirmation dialog names the source, the file to be
   written and every recipient.
6. The result is reported in the status bar and the vault is scanned again.

Refused before confirming:

- no recipient is checked and no passphrase was given;
- the recipient folder has errors, when encrypting to recipients, since a host could be silently missing from a
  group;
- the file to be written, or its record, already exists — replacing a file is E3;
- the plaintext is larger than 64 MiB, the limit a file can later be opened
  with.

When no checked key belongs to this machine, the confirmation says the file
cannot be opened or checked here, and the result is published unverified.

With a passphrase the confirmation says the file cannot be opened without it and
that a forgotten one cannot be recovered. The result is always checked, with the
passphrase.

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
3. The part file is decrypted back with this machine's identities, or the
   passphrase, and its
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
  "updated": "2026-10-02T09:30:00+10:00",
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
- `encryption` is `passphrase` for a file encrypted with a passphrase, whose
  `recipients` is then empty, and absent otherwise.
- `updated` is when the recipients were last changed, absent until they are.
- A recipient is its public key; `host` and `description` are what the recipient
  folder called it when the file was written, kept for reading, not matched on.
- The record is plaintext and names hosts.
- The vault list shows only `.age` files, so records are not listed.

## Changing recipients (E3)

`e` on a file in the vault list changes the keys it is encrypted to.

- A **decryptable** file starts straight away; one that **needs a passphrase**
  asks for the passphrase of its protected SSH identity first, unlocked for this
  change only. A **passphrase** file has no recipients: `e` changes its
  passphrase instead, below. Other statuses cannot be decrypted and are
  refused.
- The checklist is the one adding uses. It starts with the keys the file's
  record lists checked, and lists recorded keys the recipient folder no longer
  has under **not in the recipient folder**, checked, so that they can be
  removed. A file without a record starts with this machine's keys checked, and
  says that what it was encrypted to before is not known.
- Continuing is refused while the recipient folder has errors, with nothing
  checked, or when the checked keys are the ones recorded.
- The confirmation names the keys added and removed. Whenever a key is removed,
  or the file had no record, it says that a removed key can still open any copy
  of the old version — its own, git history, a backup — and that a secret it
  holds should be changed too. With no key of this machine checked it says the
  file cannot be opened or checked here afterwards.

Re-encrypting:

1. The file is decrypted into memory, up to 64 MiB.
2. It is encrypted again, with a new file key, to a `.dgs-part` beside it, in the
   same format — armored stays armored — and synced.
3. The part is decrypted back with this machine's identities and compared by
   SHA-256.
4. It is renamed over the file, keeping the file's mode. This is the one place
   the vault replaces a file, and only once the check has passed. No copy of the
   old version is kept: it is exactly what a removed key can still open.
5. The record is rewritten through a temporary file: `created` and `archive`
   kept, `updated` set, `recipients` replaced. A file without a record gains one.

## Changing a passphrase

`e` on a **passphrase** file changes its passphrase.

- Three masked fields: the current passphrase, the new one and a repeat. An
  empty field, a repeat that does not match, or a new passphrase equal to the
  current one is refused.
- The current passphrase is checked by decrypting, after confirming; a wrong
  one returns to the fields and says so.
- The confirmation says that the old passphrase can still open any copy of the
  old version, and that a secret it holds should be changed too.
- The file is re-encrypted as above, with the new passphrase instead of
  recipients, and always checked with it. The record keeps `encryption:
  passphrase` and sets `updated`.

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
- **CONTENTS** — the entries, as a tree for an archive. Its top row stands for
  the whole file, so an archive can be saved in one Action rather than a
  top-level folder at a time. Folding uses the File Explorer's keys: `l` and `→`
  open a folder, `h` and `←` close it or go to the folder holding it, `o`
  toggles it, and `O` closes the folder holding it and goes there.
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

Actions are catalogued, field by field, in [`actions.md`](actions.md). For now
they cover key files and text: SSH private keys, SSH public keys, age identities
and text. Binary files and directories have none yet. Adding a public key to
`authorized_keys` is deliberately not offered: it grants login to whoever holds
the private key.

- `Enter` on an entry in CONTENTS opens a menu of the Actions for its kind, a
  folder included: `dir.save` writes it and everything under it to a chosen
  folder.
- An Action runs in three steps: a form for its parameters, the shared
  confirmation dialog naming every path it writes, then the result in the
  status bar.
- Actions, with `c`, are the only way plaintext leaves memory, and each says
  where it puts it.

Several SSH login keys are kept one folder per server — the key, its `.pub` and
optionally a `Host` block — added with `a`. `ssh.install` puts each key in
`~/.ssh/keys/` with its own `Host` entry in `~/.ssh/config.d/`, using
`IdentitiesOnly` so that SSH offers only the right key.

## Limitations

These are known and not handled.

- **Filesystems without hard links.** A new file — a vault file, its record, an
  identity, an Action's output — is published by linking a checked temporary
  file to its name, because a link fails where the name is already taken and a
  rename would silently replace it. exFAT and FAT32 (most USB sticks and SD
  cards), and many SMB or FUSE mounts, have no hard links, so there those writes
  fail with "operation not supported". Nothing is damaged: the temporary file is
  removed and existing files are untouched. Reading, opening, and changing a
  file's recipients, which renames over the file on purpose, are not affected.
  Keep the vault, and the folders Actions save to, on a filesystem with hard
  links, such as APFS, ext4, btrfs or xfs.
- **Copying needs OSC 52.** `c` asks the terminal to set the clipboard with the
  OSC 52 sequence. iTerm2, kitty, WezTerm and Ghostty support it; macOS's
  Terminal does not, and copies nothing. Inside tmux it needs
  `set -g set-clipboard on`. A terminal may also drop a sequence longer than it
  accepts, which is why copying is limited to 64 KiB.
- **Leftover part files.** A `.dgs-part` file left by a crash mid-write is not
  cleaned up. The vault list ignores it, and it can be deleted by hand.

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
10. **D1 — Actions.** Writing files safely, SSH configuration, the agent, in
    their packages.
11. **D2 — Actions on the page.** The menu, forms and confirmation.
12. **E3a — Re-encrypting.** Decrypt, encrypt again, verify, replace, record.
13. **E3b — Changing recipients from the page.** The flow above.

