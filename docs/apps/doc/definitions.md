# Definitions

Besides its Items, a tree keeps five sorts of definition, each one YAML file
per definition in a folder of its own:

| Kind | Folder | File |
|---|---|---|
| Template | `templates/` | `<type>.yaml` |
| Outline | `outlines/` | `<name>.yaml` |
| rule | `rules/` | `<name>.yaml` |
| Snapshot | `snapshots/` | `<name>.yaml` |
| Case | `cases/` | `<name>.yaml` |

All five use the definition store owned by `tree.Store` for listing, reading,
writing, renaming and trashing. Their YAML formats and folders remain the same.

## Today

| Kind | Domain operations | Throw away |
|---|---|---|
| Template | `Store.LoadTemplates`, `Store.SaveTemplate` | `Store.TrashTemplate` |
| Outline | `outline.Load`, `outline.Save` | `outline.Trash` |
| rule | `outline.LoadRules`, `outline.SaveRule` | `outline.TrashRule` |
| Snapshot | `snapshot.Load`, `snapshot.Save` | `snapshot.Trash` |
| Case | `cases.Load`, `cases.Save`, `cases.SaveAs` | `cases.Trash` |

- Throwing away and renaming move the original file to
  `trash/<dir>/<name>-<UTC 20060102T150405Z>.yaml`; an existing trash name is
  refused. A missing Outline remains a no-op and answers an empty trash path.
- Definitions, Item sidecars and the change mark share `tree.WriteFileAtomic`:
  a same-directory `.dgs-part` file, synced and closed before publication by
  rename. This claims file sync and filesystem rename, not directory sync or
  stronger filesystem durability.
- New definitions refuse an existing name. Edits name `previous`; a rename
  requires it to exist, writes the new file, then trashes the original bytes.
- Domain operations remain in their packages: Template inheritance and Item
  checks, shared-rule and Snapshot references, and Case membership and closing.
  Typed Template YAML retains its comments; legacy inline Outline rules still
  read. Cases remain sorted open first, then by name.

## Interface

One generic store keeps every kind. A kind says where it lives, what its
name is and what makes it valid; listing, reading, the safe write, renaming
and the trash are the store's.

```go
// A Kind is one sort of definition a tree keeps, one YAML file each.
type DefinitionKind[T any] struct {
	// Dir is the folder under the root: templates, outlines, rules,
	// snapshots or cases. It is also the folder under trash/.
	Dir string
	// Name is a definition's name, which is its file's name.
	Name func(T) string
	// Validate refuses a definition that may not be saved.
	Validate func(store *tree.Store, v T) error
	// Optional codecs retain legacy forms and typed YAML comments.
	Decode func(store *tree.Store, data []byte) (T, error)
	Encode func(T) ([]byte, error)
}

// ListDefinitions reads every definition of the kind, sorted by name.
func ListDefinitions[T any](store *tree.Store, k DefinitionKind[T]) ([]T, error)

// ReadDefinition reads the one named.
func ReadDefinition[T any](store *tree.Store, k DefinitionKind[T], name string) (T, error)

// SaveDefinition writes v. With previous empty v is new, and its name must not be
// taken; with previous another name, v replaces that one, which goes to the
// trash once v is written.
func SaveDefinition[T any](store *tree.Store, k DefinitionKind[T], previous string, v T, now time.Time) error

// TrashDefinition moves the one named to trash/<dir>/<name>-<UTC time>.yaml and
// answers where it went.
func TrashDefinition[T any](store *tree.Store, k DefinitionKind[T], name string, now time.Time) (string, error)
```

- **One safe write.** A definition is written to a `.dgs-part` file in its
  folder, synced, then renamed over the old; Items use the same function.
- **Nothing is removed.** Throwing away and renaming both move the old file to
  the trash, as Templates, Snapshots and Cases already do.
- **What is particular stays with the kind.** An Outline's save also writes
  its rules; a Template inherits from others and is checked against the
  Items using it; a Case is archived when closed. These stay in their
  packages, built on the store.
- **The store belongs to `tree.Store`.** The same value that places an Item's
  files ([Storage Layouts](layout.md)) reads and writes a tree's definitions,
  so nothing outside `tree` joins a definition's path either. Domain storage
  operations take `*tree.Store`, opened through `tree.Open`, and pass it into
  the generic store and codecs rather than constructing one from a root.

`DefinitionKind` is named this way because `tree.Kind` already names an Item's
kind. Generic functions take the Store because Go methods cannot introduce
additional type parameters.

## Web routes

Every kind uses `GET /api/defs/{kind}`, `POST /api/defs/{kind}` and
`POST /api/defs/{kind}/trash`. Kinds are `templates`, `outlines`, `rules`,
`snapshots` and `cases`. Trash takes `{name}` and answers `{trash}`.

List answers keep the page's supporting information (usage, available fields,
missing Items, raw Template YAML). Saves keep their domain payloads: Template
`{previous, data}`, Outline `{previous, outline, fresh?, renamed?}`, rule
`{previous, rule}`, Snapshot `{previous, snapshot}` and Case commands
`{op, name, ...}`. A Case edit may give `previous` to rename it. Computed
previews, Snapshot Take and Case exports retain their action routes. Pages
use these shared definition routes; the former list/save/delete routes are
removed.

## Plan

1. **Done — throwing away goes to the trash.** `outline.Trash`, `outline.TrashRule`,
   and a rename of either, move the old file to the trash instead of removing
   it. This is a fix on its own and comes first.
2. **Done — one store.** Add `DefinitionKind` and the store with
   [layout step 1](layout.md#plan), and route all five kinds through it. No
   endpoint, page or file format changes.
3. **Done — one set of endpoints.** With [Web API](api.md#plan) steps 2 and 3, the
   five kinds share `GET /api/defs/{kind}`, `POST /api/defs/{kind}` and
   `POST /api/defs/{kind}/trash`, and the pages move to them.
