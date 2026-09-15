# Credentials Roadmap

The parts of `dgs cred`, in the order they build on each other, and where each
stands. The designs are in [`keys.md`](keys.md), [`vault.md`](vault.md) and
[`actions.md`](actions.md).

| Part | What | State |
| --- | --- | --- |
| A | Identities, recipients, groups; generating, importing, registering, adding and deleting keys | built |
| A4 | Unlocking passphrase-protected SSH identities | built, for one file at a time, with C2 |
| B | Browsing the vault and checking headers | built |
| C | Opening a file into memory, with passphrases | built |
| D | Actions for key files and text | built |
| E2 | Adding a file or folder, to recipients or with a passphrase | built |
| E3 | Changing a file's recipients, or its passphrase | built |
| E4 | Removing or replacing a host across every file | not started |

## Deferred

- **E4 — Recipients across files.** Remove a host from, or replace it in, every
  file whose record lists it, from the keys page: list the files first, then
  re-encrypt each with `seal.Reseal`, reporting those that fail.
- **Actions for binary files and directories**, such as saving a folder out of
  an archive.
- **Publishing without hard links**, for a vault on exFAT or a network share: see
  [`vault.md`](vault.md#limitations).
- **Recipient trust confirmation**, **editing groups from the page**, and the
  other items deferred in [`keys.md`](keys.md#deferred).
