package conf

var actionHelp = map[string]string{
	"init": `Write the scaffold a generator root starts from, overwriting nothing.

Examples:
  dgs conf init /path/to/generator
  dgs conf init /path/to/generator --secrets /path/to/secrets --gitignore

With no directory argument, use conf.root. --secrets selects a separate secrets
root instead of conf.secrets; --gitignore also writes its ignore file.
Creates starting files without overwriting existing files.`,
	"export": `Render the targets a selector matches and write them to a folder, a zip or deploy bundles.

Examples:
  dgs conf export node:host-a --to /path/to/export
  dgs conf export service:caddy --zip /path/to/export.zip
  dgs conf export user:alice export:link --to - --format yaml
  dgs conf export node:host-a --bundle --to /path/to/bundles

At least one selector is required. Terms use field:value; a bare word names
an instance. Values may contain * (quote such selectors in the shell).
Same-field terms are alternatives; different fields narrow each other.
Use dgs conf --targets to discover targets and configure conf.root/secrets.
--to selects a directory (default conf.export.dir) or - for stdout;
--format yaml bundles stdout documents. --zip selects a ZIP destination.
--bundle builds, under --to, one deploy bundle per matched instance with a
manifest: ctl, compose.yaml and the rendered files. --download build fetches
releases into the bundle now; install leaves it to ctl on the machine.
--services (default conf.services) names deploy definitions to prefer.
--install-root and --label-prefix (default conf.install_root, conf.label_prefix)
set a Linux bundle's install root and the unit/launchd label prefix.
Existing outputs require overwrite confirmation; scripts must use --overwrite.
Confirmed, a bundle built under another label prefix is rebuilt (not macOS).
--yes skips the plaintext-write prompt but does not authorize overwriting.
This renders configuration; it does not copy bundles or run deployment scripts.`,
	"bundle": `Build the deploy bundle for one exported instance, as rhumb deploy build does.

Examples:
  dgs conf bundle /path/to/export/node-1/microbin/node-1-microbin --to /path/to/bundle
  dgs conf bundle /path/to/export/node-1/samba/node-1-samba --to /path/to/bundle --platform linux/arm64

The argument is one instance's export directory, holding its manifest.
Reads only the export, not conf.root or conf.secrets.
--to is required. A bundle already there is rebuilt; another bundle is refused.
--platform defaults to the manifest's, else this machine's os/arch.
--bin bundles a local program instead of downloading a release.
--download build fetches the release now; install leaves it to ctl install.
--services (default conf.services) names deploy definitions consulted before
the built-in ones. --install-root (default conf.install_root, else /srv/rhumb)
is where a Linux bundle with no deploy.dir installs, as <root>/<service>.
--label-prefix (default conf.label_prefix, else rhumb) begins the unit and
launchd label. A bundle of the same instance built under another prefix is
refused unless --overwrite; a macOS one always is: uninstall it first.
Copy the bundle to its machine and run ./ctl install.`,
	"bundle-gc": `List what installed bundles left behind when deleted without ctl uninstall.

Examples:
  dgs conf bundle-gc
  dgs conf bundle-gc --yes

Run it on the machine the bundles were installed on. It recognizes only
bundles built with the same tool name (--tool, else conf.tool, else dgs).
Looks for launchd agents in ~/Library/LaunchAgents and shims in ~/.local/bin
whose bundle directory is gone. A bundle counts as gone only while its parent
directory exists, so a bundle on an unmounted disk is left alone.
Launchd agents are looked for under --label-prefix (default
conf.label_prefix, else rhumb), the prefix the bundles were built with.
Lists only; --yes stops each launchd agent and removes what is listed.`,
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
