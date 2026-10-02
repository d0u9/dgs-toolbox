# Photo configuration

`<config dir>/photo/config.json`:

```json
{
  "import": {
    "state_file": ".dgs-state",
    "source": "",
    "destination": ""
  }
}
```

| Key | Meaning | Default |
| --- | --- | --- |
| `import.state_file` | The name of the file Photo Import keeps its state in, inside the destination. It must be a bare filename, not a path: a configuration giving a path is refused. | `.dgs-state` |
| `import.source` | Where Import reads from. Empty shows the repository's mock path instead. | empty |
| `import.destination` | Where Import publishes to. Empty shows the repository's mock path instead. | empty |

What Import does with these is in [`apps/photo/import.md`](../apps/photo/import.md).

## Encode defaults

The `encode` object in the same `photo/config.json` initializes each fresh
Encode TUI. Omitting a key keeps its built-in default. Explicit invalid numbers
refuse the Photo configuration; session edits never persist automatically.

| Key | Meaning | Default |
| --- | --- | --- |
| `encode.source` | Initial input file or folder. Empty asks the user to choose. Home and environment variables are expanded. | empty |
| `encode.destination` | Initial output directory. Empty asks the user to choose. Home and environment variables are expanded. | empty |
| `encode.quality` | JPEG quality, integer 1–100. | `82` |
| `encode.max_side` | Maximum longest edge in pixels, integer 1–65535; never enlarges. | `800` |
| `encode.ppi` | Output JPEG density in pixels per inch, integer 1–65535. | `240` |
| `encode.background` | Opaque background for transparent input, written as `#RRGGBB`. | `#ffffff` |
| `encode.recursive` | Include non-hidden subdirectories of the input folder. | `false` |
| `encode.preserve_gps` | Retain JPEG/PNG EXIF including GPS, correcting direction/dimensions. When false, retain only capture date; HEIC/TIFF with GPS refuse preservation requests. | `false` |
| `encode.max_pixels` | Maximum decoded input pixel count, integer 1–40000000; bounds allocation before decoding. | `40000000` |

Behaviour and supported formats are in [Photo Encode](../apps/photo/encode.md).
A complete copyable configuration is in
[the Encode example](../../examples/photo-encode/photo/config.json).
