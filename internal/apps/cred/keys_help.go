package cred

const keysHelp = `Manage this machine's age identities and the recipient hosts/groups.

Examples:
  dgs cred keys
  dgs --config /path/to/config-directory cred keys

Settings live in <config-directory>/cred/config.json: identities names the
identity files; recipients names the host/group recipient folder;
new_identity_dir is where newly created identities go.
The workspace distinguishes this machine's private identities from public
recipients. Follow its key hints and confirmations for changes.
There are no positional identity paths or key-generation CLI flags.`
