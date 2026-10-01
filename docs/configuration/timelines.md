# Timelines

A timeline is a note that grows at the top: days newest first, each under a
marker such as `- *2026-09-24 周四*`, and a day's entries newest first under
it. Years that have rolled over move out into one file per year. The list of
places is one; the family timeline is another.

Every timeline is defined in one JSON file **in the vault**, not in `dgs`'s own
configuration, because two programs write timelines and both read it:

- `dgs capture`, through the Action `obsidian.timeline.<name>`
  ([actions](../apps/capture/actions.md#obsidiantimelinename)). Where the file
  is comes from `capture.obsidian.timelines`
  ([capture](capture.md#timelines)).
- The dgs-toolbox Obsidian plugin, which `dgs plugins install` puts into the
  vault ([plugins](../apps/plugins/index.md)), and which is told where the
  file is in its own settings.

Both edit a note with the same code: `internal/timeline`, which the plugin
runs compiled to WebAssembly from `cmd/timeline-wasm`. The cases under
`internal/timeline/testdata/cases` are the contract they answer to; `make
test-timeline` runs them natively, in the WebAssembly build, and through the
plugin's own `engine.js`.

```json
{
  "version": 1,
  "timelines": {
    "locations": {
      "note": "88 Inbox/06 Locations.md",
      "archive": "88 Inbox/06 Locations",
      "cssclass": "locations",
      "title": "位置速记",
      "template": "location-entry.md"
    },
    "family": {
      "note": "03 Family/00 Timeline/Timeline.md",
      "archive": "03 Family/00 Timeline/Archive",
      "split": "year",
      "cssclass": "timeline",
      "title": "时间线",
      "contentRequired": true
    }
  }
}
```

A name is lowercase letters, digits and hyphens: it becomes part of an Action
id and of the plugin's commands. A key the file does not know, or a version
other than `1`, is refused rather than ignored, by both programs.

| Key | Meaning | Default |
| --- | --- | --- |
| `version` | The version of this file's shape. | none — must be `1` |
| `timelines.<name>.note` | The running note, relative to the vault. | none — required |
| `timelines.<name>.archive` | The folder a year that has rolled over is moved into, as `<archive>/<year>.md`, relative to the vault. | empty — the note's own folder |
| `timelines.<name>.split` | What stays in the running note. `year` keeps the current year and archives the rest; it is the only value so far. | `year` |
| `timelines.<name>.cssclass` | The class written into a year's archive, for the vault's stylesheet. | empty |
| `timelines.<name>.title` | What the list is called in an archive's introduction: `2026 年的<title>`. | empty |
| `timelines.<name>.template` | The entry template `dgs capture` renders a Capture with, by filename in its template directory. The plugin does not use it. | empty — `timeline-entry.md`, else `location-entry.md` |
| `timelines.<name>.contentRequired` | Whether an entry needs text. A place is worth recording unannotated; an event is not. | `false` |
