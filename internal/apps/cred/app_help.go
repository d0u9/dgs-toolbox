package cred

const appHelp = `Manage age identities, recipients and encrypted vault files.

Examples:
  dgs cred
  dgs cred keys --help
  dgs cred vault --help
  dgs cred key hosts
  dgs cred key groups
  dgs cred e notes.txt -H host-a -H host-b -G g-team -o backup.age
  dgs cred d notes.txt.age -o restored.txt

Without a subcommand, open the Keys/Vault picker. Settings come from
<config-directory>/cred/config.json. See each command's help for its workflow.

Shell completion (enable in your current shell):
  Bash: source <(dgs completion bash)
  Zsh:  autoload -Uz compinit && compinit; source <(dgs completion zsh)

Tab completes commands/options and paths. Encryption's -H/--host and -G/--group
complete names from the active configuration, including a supplied --config.
Completion reads public recipient definitions only; it opens no private keys
and never starts a workspace or encrypts/decrypts files.`
