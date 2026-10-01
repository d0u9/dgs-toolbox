# Plugins configuration

`<config dir>/plugins/config.json`:

```json
{
  "obsidian": {
    "vaults": [
      "~/Library/Mobile Documents/iCloud~md~obsidian/Documents/Notes"
    ],
    "folders": {
      "public": "99 Toolkit/91 Scripts/01 DGS/00 Public",
      "quickadd": "99 Toolkit/91 Scripts/01 DGS/02 QuickAdd",
      "templates": "99 Toolkit/01 Templates/01 DGS"
    },
    "timelines": "99 Toolkit/timelines.json"
  }
}
```

| Key | Meaning | Default |
| --- | --- | --- |
| `obsidian.vaults` | The Obsidian vaults `dgs plugins` checks and installs into when a run names none with `--vault`. Each is a vault's own folder, the one holding `.obsidian/`. | empty — every run must name a vault with `--vault` |
| `obsidian.folders.public` | The vault folder the shared scripts go into: location, weather, the daily log. Templater's script files folder must be this one, since `tp.user` reads only that folder; the QuickAdd scripts and the dgs-toolbox plugin load them from it too. | `99 Toolkit/91 Scripts/01 DGS/00 Public` |
| `obsidian.folders.quickadd` | The vault folder the QuickAdd scripts go into. | `99 Toolkit/91 Scripts/01 DGS/02 QuickAdd` |
| `obsidian.folders.templates` | The vault folder the Templater templates go into. It must be Templater's template folder or inside it, or the templates are not offered. | `99 Toolkit/01 Templates/01 DGS` |
| `obsidian.timelines` | Where `bootstrap` writes the timelines file when the vault has none, and what the plugin's settings point at. | `99 Toolkit/timelines.json` |

The three folders and the timelines file are paths inside the vault, written
with `/`: relative, not climbing out of the vault and not into a hidden folder.
No folder may hold another. dgs owns the files it writes into them and nothing
else, so a script of your own can sit beside them — but not under a name dgs
also uses. Changing a folder moves nothing: install into the new one, then
remove the old one yourself.

`obsidian.vaults` paths follow the same rules as [`cred`](cred.md#paths): a leading `~` is the
home directory, `$NAME` and `${NAME}` are environment variables, and the
result must be absolute.

What `dgs plugins` does with them is in [`apps/plugins/`](../apps/plugins/index.md).
