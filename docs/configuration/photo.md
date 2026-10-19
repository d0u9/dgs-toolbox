# Photo configuration

`<config dir>/photo/config.json`:

```json
{
  "import": {
    "state_file": "_dgs-state",
    "source": "",
    "dest": []
  }
}
```

| Key | Meaning | Default |
| --- | --- | --- |
| `import.state_file` | The name of the file Photo Import keeps its state in, inside the destination. It must be a filename starting with `.` or `_`, not a path or a photo name; the default and the earlier `.dgs-state` are always recognized as state. Unrecognized existing contents are preserved and reported as a state error. | `_dgs-state` |
| `import.source` | Where Import reads from. Empty shows the repository's mock path instead. | empty |
| `import.dest` | Initial destination directories, in display order. Each receives a complete copy. An omitted or empty array shows the repository's mock destination instead. | `[]` |

## Import profiles

`import.profiles` names the sources and destinations used often, each with its
own import settings, so an import starts without choosing them again:

```json
{
  "import": {
    "profiles": {
      "card": {
        "source": "/Volumes/SD/DCIM",
        "dest": ["~/Pictures/Inbox", "~/Pictures/Backup"],
        "ext": ["JPG", "CR3"],
        "duplicates": "keep-both",
        "organize": true
      },
      "phone": { "source": "~/Downloads/phone", "dest": ["~/Pictures/Inbox"] }
    }
  }
}
```

| Key | Meaning | Default |
| --- | --- | --- |
| `import.profiles.<name>.source` | Directory to import from. Required. `~` is expanded. | — |
| `import.profiles.<name>.dest` | Destination directories, each receiving a complete copy. At least one is required. | — |
| `import.profiles.<name>.ext` | Extensions to import, as `--ext` takes them; one the card lacks is skipped with a notice. Empty imports every regular file. | `[]` |
| `import.profiles.<name>.duplicates` | `skip` or `keep-both`. | `skip` |
| `import.profiles.<name>.workers` | Whole-file workers per destination, 1–8. | `1` |
| `import.profiles.<name>.incremental` | Find existing copies by size and SHA-256. | `true` |
| `import.profiles.<name>.organize` | Organize imported JPG/RAW into date folders. | `false` |
| `import.profiles.<name>.allow_check_rename` | For destinations that cannot refuse an existing name, such as SMB mounts: publish by checking then renaming, holding `_dgs-lock` in each destination. A file another program writes to the same name at that moment can be replaced. | `false` |

A profile cannot delete sources: there is no `move` key, so moving is always
asked for at the time, with `--move` or in the workspace. A name is letters,
digits, `-` and `_`. `dgs photo import --profile` lists the profiles and
`dgs photo import --profile card` runs one; any flag given
as well replaces the profile's value, and `--dest` replaces its whole list. In
the workspace, Setup lists the profiles first.

The legacy `import.destination` string remains readable as one destination. Do not
combine it with `import.dest`; new configurations use the array.

What Import does with these is in [`apps/photo/import.md`](../apps/photo/import.md).

The Import workspace's **Incremental** checkbox defaults to enabled for each
fresh session. It confirms identical size and SHA-256 anywhere under Destination,
including date subdirectories, before skipping a copy. This is a session control,
not an `import` JSON key; changing it does not persist. State uses version 2 and
can be deleted without losing incremental detection; version 1 is still readable.

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
