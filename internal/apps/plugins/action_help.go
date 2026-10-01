package plugins

var actionHelp = map[string]string{
	"status": `Show every plugin dgs carries in every place it goes: installed or not, which version, changed since or not, switched on or not. Changes nothing.

Examples:
  dgs plugins status
  dgs plugins status --vault /path/to/vault

Omitting plugin names selects all bundled plugins. --vault overrides configured
plugins.obsidian.vaults and may be repeated. Prints status without changing
files. Ambiguous names must be qualified as host/plugin.`,
	"install": `Install plugins, every one dgs carries when none is named, in every place they go. Leaves files changed by hand alone unless --force.

Example:
  dgs plugins install --vault /path/to/vault

Omitting names selects all bundled plugins; named plugins can use host/plugin.
--vault may be repeated and overrides configured vaults. Modified installed
files are kept unless --force explicitly permits replacement.`,
	"update": `Replace installed plugins an older dgs wrote with what this one carries. Installs nothing new.

Example:
  dgs plugins update --vault /path/to/vault

Omitting names selects all bundled plugins. Only previously installed plugins
are updated; new ones are not installed. --vault may be repeated.
Modified files are kept unless --force permits replacement.`,
	"uninstall": `Remove the files dgs installed for a plugin. The application's own settings for it stay.

Example:
  dgs plugins uninstall obsidian/dgs-toolbox --vault /path/to/vault

At least one plugin name is required. Use dgs plugins status to discover names.
--vault may be repeated. Removes managed plugin files while keeping the host
application's own settings. --force permits removal of modified managed files.`,
	"bootstrap": `Set up a vault: install everything dgs carries, write the starting files it lacks, and list what is left to do by hand. Overwrites nothing.

Examples:
  dgs plugins bootstrap --vault /path/to/vault
  dgs plugins bootstrap --vault /path/to/vault --from /path/to/starter

Installs bundled plugins and writes missing starting files; existing files
are kept. --from selects a folder previously written by dgs plugins export.
Reports the steps still needed in the host application. --vault may be repeated.`,
	"export": `Copy a vault's starting files, as they are now, into a folder, for someone else's bootstrap --from.

Example:
  dgs plugins export /path/to/starter --vault /path/to/vault

The destination directory is required. Copies a vault's current starting
files for use with bootstrap --from. --vault selects the source vault.
Read the result before using --force to replace changed managed outputs.`,
}
