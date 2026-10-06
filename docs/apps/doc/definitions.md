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

All five are listed, read, saved, renamed and thrown away in the same way. This
records that today each does so with its own code, and how that becomes one
interface.

## Today

| Kind | List and read | Save | Throw away |
|---|---|---|---|
| Template | `tree.LoadTemplates` | `tree.SaveTemplate` | `tree.TrashTemplate`: to the trash |
| Outline | `outline.Load` | `outline.Save` | `outline.Delete`: **removed** |
| rule | `outline.LoadRules` | `outline.SaveRule` | `outline.DeleteRule`: **removed** |
| Snapshot | `snapshot.Load` | `snapshot.Save` | `snapshot.Trash`: to the trash |
| Case | `cases.Load` | `cases.Save` | `cases.Trash`: to the trash |

- **Throwing away differs.** An Outline or a rule thrown away is removed with
  `os.Remove` and cannot be recovered. Every other definition, and every Item,
  goes to the trash. Nothing in a tree is to be erased.
- **The safe write is copied.** Writing through a temporary file and a rename
  is written out once each for Items, Templates, Outlines, Snapshots, Cases and
  the change mark; some of the copies sync before the rename and some do not,
  and each names its temporary file differently.
- **Renaming is checked separately.** That a new name is not taken, that a
  renamed definition existed, and that the old file goes once the new one is
  written are checked by each kind in its own words.
- **The web layer repeats it.** Each kind has its own list, save and delete
  handlers and endpoints.

## Interface

One generic store keeps every kind. A kind says only where it lives, what its
name is and what makes it valid; listing, reading, the safe write, renaming
and the trash are the store's.

```go
// A Kind is one sort of definition a tree keeps, one YAML file each.
type Kind[T any] struct {
	// Dir is the folder under the root: templates, outlines, rules,
	// snapshots or cases. It is also the folder under trash/.
	Dir string
	// Name is a definition's name, which is its file's name.
	Name func(T) string
	// Validate refuses a definition that may not be saved.
	Validate func(root string, v T) error
}

// List reads every definition of the kind, sorted by name.
func List[T any](root string, k Kind[T]) ([]T, error)

// Read reads the one named.
func Read[T any](root string, k Kind[T], name string) (T, error)

// Save writes v. With previous empty v is new, and its name must not be
// taken; with previous another name, v replaces that one, which goes to the
// trash once v is written.
func Save[T any](root string, k Kind[T], previous string, v T, now time.Time) error

// Trash moves the one named to trash/<dir>/<name>-<UTC time>.yaml and
// answers where it went.
func Trash[T any](root string, k Kind[T], name string, now time.Time) (string, error)
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
  so nothing outside `tree` joins a definition's path either.

## Plan

1. **Throwing away goes to the trash.** `outline.Delete`, `outline.DeleteRule`,
   and a rename of either, move the old file to the trash instead of removing
   it. This is a fix on its own and comes first.
2. **One store.** Add `Kind` and the store with
   [layout step 1](layout.md#plan), and route all five kinds through it. No
   endpoint, page or file format changes.
3. **One set of endpoints.** With [Web API](api.md#plan) steps 2 and 3, the
   five kinds share `GET /api/defs/{kind}`, `POST /api/defs/{kind}` and
   `POST /api/defs/{kind}/trash`, and the pages move to them.
