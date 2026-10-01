package cred

const vaultHelp = `Browse and open age-encrypted vault files in an interactive workspace.

Examples:
  dgs cred vault
  dgs --config /path/to/config-directory cred vault

cred/config.json supplies vault, identities, recipients and close_after.
With no vault configured, select its folder in the workspace.
Decrypted content stays in memory unless you confirm an Action that writes or
exports it. Enter on an opened content entry lists applicable Actions.
A rewritten vault is published only after decrypting it back and comparing
SHA-256. Follow the workspace's contextual hints and write confirmations.`
