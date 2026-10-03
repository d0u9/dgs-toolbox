package conf

const inspectHelp = `Inspect configuration inventory in an interactive workspace.

Examples:
  dgs conf --root /path/to/generator --secrets /path/to/secrets
  dgs conf --targets
  dgs conf --check

--root and --secrets override conf.root and conf.secrets for this process.
Use the workspace to inspect nodes, instances, users and secrets; follow its
contextual key hints; r reloads generator and secret changes from disk.
--targets lists renderable targets and --check reports inventory/manifest/
secrets problems; both print results and return without a TUI.
Use dgs conf export for scripted rendering. Rendered exports are plaintext;
exporting configuration does not deploy it or run its scripts.`
