package doc

var actionHelp = map[string]string{
	"init": `Make a folder a doc tree, by writing its marker and an example Template.

Example:
  dgs doc init /path/to/documents

With no directory argument, use the configured default tree, or the working
directory when none is configured. Writes the tree marker and an example
Template when no Template exists. Other folder contents are kept.`,
	"verify": `Check every PDF against its sidecar and digest. Changes nothing.

Examples:
  dgs doc verify /path/to/documents
  dgs doc verify --tree personal --quiet

An explicit directory wins; otherwise select from doc.trees with --tree.
With no configured tree, use the working directory. --tree names a configured
tree, not a path. --quiet omits per-file progress. Problems return an error
status. PDFs, sidecars and rules are checked but nothing is repaired.`,
	"export": `Export Outlines into their folders: those given, or every one with a folder. Checks everything first; a conflict writes nothing.

Examples:
  dgs doc export --tree personal --dry-run
  dgs doc export bills --tree personal
  dgs doc export bills --tree personal --to bills=/path/to/export

Arguments name Outlines; omitting them selects all Outlines with an output
folder. --to overrides output folders for this run as outline=folder pairs,
comma separated. --dry-run plans and checks without writing.
A detected conflict prevents writes. --tree selects a name in doc.trees.`,
	"link": `Fill each empty link field whose within finds exactly one Item, such as a bill's tenancy by its date. Lists them; writes only with --apply.

Examples:
  dgs doc link /path/to/documents
  dgs doc link --tree personal --apply

Without --apply, list proposed links without writing. With --apply, fill
eligible empty link fields only when their rule finds exactly one Item.
An explicit directory selects the tree; otherwise use doc.trees and --tree,
falling back to the working directory when no tree is configured.`,
	"explain": `Show how each Outline, or those given, places an Item's PDFs, or why it does not: each condition, branch and key. Changes nothing.

Examples:
  dgs doc explain ITEM-ID --tree personal
  dgs doc explain ITEM-ID bills --tree personal

The Item ID is required. Optional remaining arguments name Outlines;
omitting them explains all Outlines. Shows conditions, branches and output
keys, including why a PDF is not placed. --tree names a configured doc.trees
entry. This command writes nothing.`,
}
