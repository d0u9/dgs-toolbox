package box

var actionHelp = map[string]string{
	"init": `Make a folder a Box, by writing its marker.

Example:
  dgs box init /path/to/box

With no directory argument, use box.root; an empty setting is an error.
Writes the Box marker and its initial log without modifying other contents.
Use dgs box to take scans in afterward.`,
	"index": `Rebuild the discardable local cache from the sidecars.

Example:
  dgs box index /path/to/box

With no directory argument, use box.root. Rebuilds the configured local cache
from original files and sidecars; it does not repair or rewrite Box metadata.
The directory must already be an initialized Box.`,
	"verify": `Read every byte and check it against its recorded digest.

Examples:
  dgs box verify /path/to/box
  dgs box verify --quiet

With no directory argument, use box.root. Reads file contents against recorded
digests; --quiet omits per-file progress. A mismatch returns an error status.
Nothing is repaired and neither the file nor its recorded digest is rewritten.`,
	"dedupe": `Group the scans that are the same piece of paper.

Example:
  dgs box dedupe /path/to/box

With no directory argument, use box.root. Prints duplicate groups and paths,
including whether a copy is in trash. It does not discard or delete a copy;
choosing which one to keep remains a user's decision.`,
}
