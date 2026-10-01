package capture

const scanHelp = `Open the Capture workspace to organize captured data using Recipes.

Examples:
  dgs capture
  dgs capture --recipes
  dgs capture --actions
  dgs --config /path/to/config-directory capture --init

The workspace starts with Scan; choose captured data, then follow the
Recipe-driven stages and contextual key hints. Settings come from
capture/config.json, including scan.root and archive/reject destinations.
--recipes and --actions print supported Recipes and Actions and return.
--init writes shipped Recipes and workflow descriptions into the selected
configuration directory instead of opening the TUI. Read the reported paths
and results; it configures Capture, not a scan-source folder.`
