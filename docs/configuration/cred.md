# Credentials configuration

`dgs cred` does not read `dgs-config.json`. Its settings are in
`credentials.json`, in the same folder: `$XDG_CONFIG_HOME/dgs-toolbox/`, or
`~/.config/dgs-toolbox/` when that variable is unset.

```json
{
  "identities": ["~/.age", "~/.ssh"],
  "recipients": "$DOT_CONF_DIR/recipients",
  "vault": "~/Credentials/vault",
  "new_identity_dir": "~/.config/age",
  "close_after": "5m",
  "archive_skip": [".DS_Store", "._*", "Thumbs.db", ".git"]
}
```

| Key | Meaning | Default |
| --- | --- | --- |
| `identities` | Directories searched recursively for private keys on this machine. Files are recognised by content; files over 128 KiB are skipped. A directory that does not exist is reported and skipped, so one file can be shared by machines that do not all have every directory. | empty — no identities |
| `recipients` | The folder holding `hosts/` and `groups/`. | empty — no recipients |
| `new_identity_dir` | Where generated and imported age identities are written. It should also be listed in `identities`. | `$XDG_CONFIG_HOME/age`, or `~/.config/age` |
| `close_after` | How long an opened vault file stays open with no key pressed or click made before its plaintext is discarded, as a Go duration such as `90s` or `5m`. `0` never closes it. | `5m` |
| `vault` | The folder of age files `dgs cred vault` opens at. It can be switched on the page. | empty — the page asks for a folder |
| `archive_skip` | File name patterns left out when a folder is added to the vault, matched against each entry's own name, case insensitively, with `*` and `?` as in a shell. A matching directory is not walked. Giving the key replaces the built-in list rather than adding to it; `[]` archives every entry. | the built-in list below |

## `archive_skip`

A folder added to the vault is archived as gzip-compressed tar. Its dotfiles
are kept — `.ssh` and `.env` are what a folder is added for — but what an
operating system, a file manager or git writes beside the files is not a
credential, and it changes the archive every time the folder is added again.
The built-in list is:

```
.DS_Store  ._*  .AppleDouble  .LSOverride  .Spotlight-V100  .Trashes
.fseventsd  .TemporaryItems  .DocumentRevisions-V100  .apdisk  Icon\r
Thumbs.db  Thumbs.db:encryptable  ehthumbs.db  ehthumbs_vista.db
desktop.ini  $RECYCLE.BIN  *.stackdump
.directory  .Trash-*
.git
```

`.gitignore` and `.gitattributes` are not in it: they are the folder's own
content, not git's bookkeeping. The confirmation before encrypting names what
will be left out, so nothing goes missing without a word.

## Paths

In every path, a leading `~` is the home directory, and `$NAME` or `${NAME}`
is replaced by that environment variable. A variable that is not set is an
error naming it, rather than an empty string: `$DOT_CONF_DIR/recipients` with
the variable unset would otherwise quietly become `/recipients`. After
expansion a path must be absolute; relative paths are refused.

Unknown keys are an error. A missing `credentials.json` is not: `dgs cred`
opens with no identities and no recipients, and says where the file is looked
for.

What is inside the recipient folder, and how identities are recognised, is in
[`apps/cred/keys.md`](../apps/cred/keys.md). How the vault is read is in
[`apps/cred/vault.md`](../apps/cred/vault.md).
