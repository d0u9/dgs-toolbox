# Photo Encode

`dgs photo encode` is a real JPEG export workspace. It keeps every source,
never overwrites an existing output, and changes files only after the user
reviews the plan and explicitly starts Processing.

## Workflow

A centered one-column parameter form uses the shared form and File Explorer.
Source accepts one file or a directory; Destination selects a directory.
Quality, longest edge, PPI, background colour, recursive scanning and Preserve GPS start from
[`photo/config.json`](../../configuration/photo.md). Changes in the session
are temporary and do not rewrite configuration.

`n` reviews the read-only plan, then `n` on the plan starts encoding. `Esc`
from the plan returns to parameters. Results scroll vertically and horizontally (`h`/`l`), and `r` starts another
setup. During work, Esc, q and Ctrl+C open the shared safe-default Stop dialog;
stopping waits for the current job to finish cancelling. Published files remain
complete. Leaving or quitting uses the shared shell confirmation and cancels
work and waits for the active engine job to return. Async messages carry a
session identifier so a previous session cannot update a fresh workspace.
There is no nested Bubble Tea program.

## Formats and pixels

JPEG and PNG use the Go standard library. Scaling and orientation reuse
`internal/imaging` with the project's existing official `golang.org/x/image`
Catmull-Rom scaler. No new third-party dependency is introduced.

HEIC/HEIF uses ImageIO/CoreGraphics under `darwin && cgo`. Other builds refuse
only those inputs with an explicit reason. The native path renders in the
**decoded image's own RGB colour space**, and carries its corresponding ICC
profile. It refuses multi-image containers, high-bit-depth/HDR pixels and
unsupported colour representations. It does not promise original HEIC colour
metadata bytes survive native decoding; the profile describes the decoded RGB.
HEIC inputs carrying GPS are refused when Preserve GPS is enabled: native GPS
export has not been implemented, so that request cannot silently lose location.

Output is JPEG. Longest edge is an upper bound, proportions are kept, small
images are not enlarged, EXIF orientations 1–8 are applied, and transparency
is composited onto the configured opaque background (white by default). This is an 8-bit export path, not archival preservation.
PPI is output density metadata and does not add pixels.

TIFF is excluded. Its plan entry supplies a shell-quoted `sips` command using
current quality, size, PPI and output path, for the user to run manually.
**dgs never executes this command.** It does not add an sRGB conversion.

## Colour and metadata

There is no ICC colour conversion. RGB profiles on JPEG/PNG are treated as
opaque container data and copied to output. Invalid/incomplete profiles and
non-RGB profiles are refused. CMYK JPEG is refused. Nonstandard PNG gamma,
chromaticities and cICP colour signalling are refused rather than stripped or
mislabeled. Untagged images are not assigned a made-up ICC profile.

With Preserve GPS disabled (the default), export rebuilds only EXIF capture
date when present; other EXIF, XMP, IPTC and location data are omitted. It does
not leave discarded GPS bytes hidden in a copied EXIF payload. With Preserve
GPS enabled, JPEG/PNG EXIF is retained, orientation and existing pixel dimension
fields are corrected, density is corrected, and its old thumbnail is unlinked. XMP/IPTC are still
omitted. This is not a promise to preserve every proprietary metadata tag.

Container handling is bounded JPEG segment / PNG chunk / EXIF directory framing,
not a custom codec or an ICC interpretation engine.

## Files and publication

The output name is `<stem>-q<quality>-s<longest-edge>.jpg`. Recursive batches
preserve relative subdirectories, skip hidden subdirectories and symlinks, and
exclude a nested Destination from scanning. Two inputs mapping to one output
are both marked as conflicts. Unsupported TIFFs and conflicts are listed in the
plan and reported individually without stopping other jobs.

One job runs at a time. DecodeConfig checks the configured pixel limit before
allocation; input file bytes are capped at 256 MiB. Limits also apply to native
HEIC decoding. Progress is per-file rather than per-byte.

Processing creates a unique same-directory `.dgs-part` file, writes the complete
encoded JPEG, syncs and closes it, then independently reads it back. Its SHA-256
must match the encoded artifact, and independent JPEG decoding must confirm the
expected dimensions. Only then does an atomic hard-link creation publish the
final name without replacement; temporary names are removed. Filesystems that
cannot create hard links receive an explicit publication failure, with no
unsafe rename fallback. Published files retain the temporary file's private
permissions. A killed process may leave an unpublished temporary file.

Source and output hashes are not compared: lossy encoding changes file contents.
The guarantee is at the user-space/filesystem API boundary, not physical-media
power-loss durability. No resume-state file or persistent cache is introduced.

## Deferred

Additional output formats, RAW/TIFF decoding, cropping, filters, target byte
budgets, saved presets and HEIC GPS export are not part of this first version.
