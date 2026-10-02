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

HEIC/HEIF and TIFF use ImageIO/CoreGraphics under `darwin && cgo`, macOS
only. On other platforms, and in a macOS build without cgo, the plan lists
those files as not supported yet with the reason; other inputs still run.
The native path decodes in the image's own colour space, RGB or grayscale,
before the sRGB conversion below. It refuses multi-image files, floating
point pixels and unsupported colour representations. HEIC high-bit-depth/HDR
pixels are refused; 16-bit integer TIFF is reduced to 8 bits in its own colour
space. HEIC/TIFF inputs carrying
GPS are refused when Preserve GPS is enabled: native GPS export has not been
implemented, so that request cannot silently lose location.

Output is JPEG. Longest edge is an upper bound, proportions are kept, small
images are not enlarged, EXIF orientations 1–8 are applied, and transparency
is composited onto the configured opaque background (white by default). This is an 8-bit export path, not archival preservation.
PPI is output density metadata and does not add pixels.

## Colour and metadata

Every export is sRGB and carries the sRGB ICC profile, as the earlier `sips -m`
workflow did and as the web and most viewers assume. Pixels with an RGB
profile, or a GRAY profile on grayscale pixels, are converted by macOS
ColorSync (CoreGraphics, default rendering intent) after scaling. Untagged
pixels are taken to be sRGB already and are tagged as such. Grayscale output
is three-channel sRGB.

Conversion is macOS only, under `darwin && cgo`. Elsewhere untagged JPEG/PNG
still export, untagged and unconverted; a tagged image is refused per file as
not supported yet, rather than exported with colours its profile no longer
describes.

Invalid/incomplete profiles and other colour spaces are refused. CMYK JPEG is
refused. Nonstandard PNG gamma or chromaticities are refused when they are the
only colour description; an iCCP or sRGB chunk takes precedence, as the PNG
specification says. cICP is refused rather than stripped or mislabeled.

With Preserve GPS disabled (the default), export rebuilds only EXIF capture
date when present; other EXIF, XMP, IPTC and location data are omitted. It does
not leave discarded GPS bytes hidden in a copied EXIF payload. With Preserve
GPS enabled, JPEG/PNG EXIF is retained, orientation and existing pixel dimension
fields are corrected, density is corrected, EXIF ColorSpace is set to sRGB, and
its old thumbnail is unlinked.
When that EXIF cannot fit one JPEG APP1 segment (large maker notes), it is
rebuilt with only capture date and the GPS directory. XMP/IPTC are still
omitted. This is not a promise to preserve every proprietary metadata tag.

Container handling is bounded JPEG segment / PNG chunk / EXIF directory framing,
not a custom codec or an ICC interpretation engine; colour conversion is
ColorSync's.

## Files and publication

The output name is `<stem>-q<quality>-s<longest-edge>.jpg`. Recursive batches
preserve relative subdirectories, skip hidden files and subdirectories
(including AppleDouble `._` companions) and symlinks, and exclude a nested
Destination from scanning. A folder or file that cannot be read becomes its own
failed plan entry and the scan continues. Two inputs mapping to one output
are both marked as conflicts. Unsupported inputs and conflicts are listed in the
plan and reported individually without stopping other jobs.

One job runs at a time. DecodeConfig checks the configured pixel limit before
allocation; input file bytes are capped at 256 MiB. Limits also apply to native
ImageIO decoding. Progress is per-file rather than per-byte.

Processing creates a unique same-directory `.dgs-part` file, writes the complete
encoded JPEG, sets permission `0644` so the export can be shared, syncs and
closes it, then reads it back through a fresh handle that must be the same file.
Its SHA-256 must match the encoded artifact, and independent JPEG decoding must
confirm the expected dimensions. The temporary name must still be that file;
then it is published under its final name without replacement, using Photo
Import's publication (`verifiedcopy.Publish`): an exclusive rename, else a hard
link, else — only on a filesystem with neither — check-then-rename. A killed
process may leave an unpublished temporary file.

Source and output hashes are not compared: lossy encoding changes file contents.
The guarantee is at the user-space/filesystem API boundary, not physical-media
power-loss durability. No resume-state file or persistent cache is introduced.

## Deferred

Additional output formats, RAW decoding, cropping, filters, target byte
budgets, saved presets and HEIC GPS export are not part of this first version.
