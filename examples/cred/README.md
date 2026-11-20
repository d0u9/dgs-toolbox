# Credentials CLI examples

This folder contains synthetic recipient definitions for `laptop-mac-01`,
`server-linux-01` and the group `g-servers`. Copy the configuration and recipient
folder to a private configuration location and adjust `recipients` in
`cred/config.json` to that copy. Real keys, sources and outputs belong outside
this public checkout. The examples below assume that configuration is active
and that the working directory is a scratch directory outside the checkout.

List recipients and read command help:

```sh
dgs cred key hosts
dgs cred key groups
dgs cred e --help
dgs cred d --help
```

Encrypt a file for two hosts, with an explicit output name. Repeat `-H` to add
hosts; `-G` can be repeated too, and hosts/groups can be combined. Duplicate
public keys are used once.

```sh
dgs cred e notes.txt -H laptop-mac-01 -H server-linux-01 -o backup.age
dgs cred e notes.txt -H laptop-mac-01 -G g-servers -o shared.age
```

Use default output naming to create `photos.tar.gz.age` from a directory.
The archive keeps the `photos` directory and uses configured `archive_skip`
patterns. Files are streamed with no application size limit.

```sh
dgs cred e photos/ -G g-servers
```

Decrypt under a new name using configured local private identities. Encryption
keeps the source, so the original `notes.txt` may still exist; decryption refuses
to overwrite it. Archives are restored as archives, without automatic extraction.
The sample public recipients need corresponding private identities to decrypt.

```sh
dgs cred d backup.age -o restored-notes.txt
dgs cred d photos.tar.gz.age -o restored-photos.tar.gz
```

Select another configuration directory, or use `--` for dash-prefixed filenames:

```sh
dgs --config /path/to/config-directory cred e notes.txt -G g-servers
dgs cred e -G g-servers -- -notes.txt
```

`encrypt` and `decrypt` remain aliases for `e` and `d`. `--host`, `--group` and
`--output` are the long forms of `-H`, `-G` and `-o`. `-h` displays help.
Existing output files and part files are refused; choose a new name with `-o`.

Enable completion for the current shell:

```bash
source <(dgs completion bash)
```

```zsh
autoload -Uz compinit && compinit
source <(dgs completion zsh)
```

Tab completes commands, options and paths. After `-H` and `-G` it offers the
configured host/group names, including with `--config`. See the
[completion setup notes](../../docs/apps/cred/actions.md#help-examples-and-shell-completion)
for enabling this in future sessions.
