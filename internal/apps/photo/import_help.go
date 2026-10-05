package photo

const importHelp = `Import photos by copying or moving into a chosen destination.

Examples:
  dgs photo import
  dgs --config /path/to/config-directory photo import

Choose Source and Destination, scan and review the parameters, then explicitly
start Processing. Paths initially come from photo/config.json import.source
and import.destination; empty defaults use the repository's mock paths.
Use the workspace controls to choose real locations and Copy or Move.
Existing-file conflicts are handled through the workspace's duplicate policy.
Incremental is enabled initially: current source content is compared with files
anywhere under Destination, including date folders and renamed copies. Matching
size and SHA-256 skips writing a new copy; checking may still read both files.
Disable Incremental in Parameters to use filename-conflict handling alone.

Each file is copied to a same-directory .dgs-part file while computing Source
SHA-256. An independent destination readback must match before the final name
is published. Move removes Source only after a verified new or existing copy;
existing copies are re-read before deletion. Conflict skips never delete Source.
Deletion failures
are reported as source kept. Retries restart the whole file.
Resume bookkeeping uses import.state_file (default .dgs-state) in Destination.
This is a filesystem API guarantee, not physical-media power-loss durability.
Follow on-screen hints; no positional paths or copy/move CLI flags are accepted.`
