package conf

var actionHelp = map[string]string{
	"init": `Write the scaffold a generator root starts from, overwriting nothing.

Examples:
  dgs conf init /path/to/generator
  dgs conf init /path/to/generator --secrets /path/to/secrets --gitignore

With no directory argument, use conf.root. --secrets selects a separate secrets
root instead of conf.secrets; --gitignore also writes its ignore file.
Creates starting files without overwriting existing files.`,
	"export": `Render the targets a selector matches and write them to a folder or a zip.

Examples:
  dgs conf export node:host-a --to /path/to/export
  dgs conf export service:caddy --zip /path/to/export.zip
  dgs conf export user:alice export:link --to - --format yaml

At least one selector is required. Terms use field:value; a bare word names
an instance. Values may contain * (quote such selectors in the shell).
Same-field terms are alternatives; different fields narrow each other.
Use dgs conf --targets to discover targets and configure conf.root/secrets.
--to selects a directory (default conf.export.dir) or - for stdout;
--format yaml bundles stdout documents. --zip selects a ZIP destination.
Existing outputs require overwrite confirmation; scripts must use --overwrite.
--yes skips the plaintext-write prompt but does not authorize overwriting.
This renders configuration; it does not run deployment scripts.`,
	"secret": `Generate the credentials the inventory implies and are not on disk yet.

Example:
  dgs conf secret sync

The required argument is literally sync. Reads configured conf.root and
conf.secrets and generates missing credentials implied by the inventory.
Existing credentials are not regenerated or deleted. --yes skips confirmation.`,
	"reservations": `Print the address reservations a network's router should hold, in address order.

Example:
  dgs conf reservations home

The required argument is a network name in the configured inventory.
Prints that network's address reservations in address order and returns.
It does not configure or contact a router.`,
}
