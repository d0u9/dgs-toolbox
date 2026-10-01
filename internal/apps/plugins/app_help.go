package plugins

const appHelp = `Manage plugins bundled with dgs for other applications.

Examples:
  dgs plugins status --vault /path/to/vault
  dgs plugins install --help
  dgs plugins bootstrap --help

These are CLI actions; there is no plugin-management TUI or runtime plugin
loading in dgs. Vault defaults come from plugins/config.json. Use status to
inspect names and installed files before install, update or uninstall.`
