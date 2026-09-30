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
