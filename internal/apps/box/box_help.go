package box

const boxHelp = `Start the Box terminal workspace and its local web pages.

Examples:
  dgs box --root /path/to/box --inbox /path/to/scans
  dgs box --port 8766

Initialize a new Box with dgs box init first. --root and --inbox override
box.root and box.inbox; --port overrides the browse server's configured port
(default 8766). The workspace shows the local page address.
Intake, browsing and exceptions are handled in those pages. Per-scan YAML
sidecars are metadata truth; the local cache is discardable. Discarding a scan
moves it into the Box trash. Maintenance actions below run without a TUI.`
