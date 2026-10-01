package photo

const encodeHelp = `Export photos as JPEG in an interactive workspace.

Usage examples:
  dgs photo encode
  dgs --config /path/to/config-directory photo encode

Choose a source file or folder and a destination folder in the workspace.
Tab / Shift+Tab move between controls; Enter selects a path or edits a value.
Press n to review the output plan, then n again to start writing files.
Esc on the plan returns to parameters. During encoding, Esc, q or Ctrl+C
opens a safe-default Stop confirmation. In Results, r returns to parameters.
Arrow keys / h j k l scroll the plan and results, including long sips commands.
No output files are written by planning. Sources and existing outputs are kept.

Settings (initial values can all be configured):
  JPEG quality       1-100; default 82. Like imgconvert -q / --quality.
  Longest edge       Maximum pixels; default 800. Like -s / --size.
                     Keep proportions; never enlarge a smaller image.
  PPI                Output density metadata; default 240. Adds no pixels.
  Background         Opaque #RRGGBB; default #ffffff (white).
  Include children   Scan non-hidden subfolders; default false.
  Preserve GPS       Retain JPEG/PNG EXIF including GPS; default false.
                     When false, only EXIF capture date is retained.
                     XMP and IPTC are omitted in either mode.

Unlike imgconvert -o, Destination selects an output DIRECTORY.
Output names are <stem>-q<quality>-s<longest-edge>.jpg.
Recursive exports preserve relative subfolders. Existing outputs and inputs
sharing an output name are reported as conflicts; nothing is overwritten.
There are no -q, -s or -o CLI flags in this first version: edit these settings
in the workspace, or change their initial values in photo/config.json.

Configuration example, saved as <config-directory>/photo/config.json:
  {
    "encode": {
      "source": "",
      "destination": "",
      "quality": 82,
      "max_side": 800,
      "ppi": 240,
      "background": "#ffffff",
      "recursive": false,
      "preserve_gps": false,
      "max_pixels": 40000000
    }
  }
Empty paths ask you to choose. Omitted keys keep their defaults.
Session edits do not rewrite configuration. max_pixels is the input pixel
limit (1-40000000); input files are also capped at 256 MiB. Jobs run sequentially.

Formats and colour:
  JPEG / PNG         Decoded and encoded by Go; EXIF direction is applied.
  HEIC / HEIF        Requires macOS and a cgo-enabled dgs build.
                     HDR/high-bit-depth and multi-image containers are refused.
                     HEIC carrying GPS is refused when Preserve GPS is enabled.
  TIFF               Not supported. The plan provides a shell-quoted sips
                     command using your size, quality, PPI and output path.
                     Run it manually; dgs never executes external commands.
  Output             JPEG only; an 8-bit export, not an archival copy.

No colour-space conversion is performed. Applicable RGB ICC profiles are
retained; HEIC uses the decoded RGB image's corresponding profile. CMYK,
invalid/non-RGB profiles and unsupported PNG colour signalling are refused.
Untagged images are not assigned an invented sRGB profile.

Each JPEG is written to a same-directory .dgs-part file, synced and closed,
then independently read back for SHA-256 and decode/dimension verification.
Only then is its final name published atomically without replacement.
The destination filesystem must support hard links; otherwise publication
fails explicitly. This is a filesystem API guarantee, not a power-loss
physical-media guarantee.`
