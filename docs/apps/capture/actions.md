# Actions

Every Action this version carries, and what a Recipe may name. Actions are the part
that is compiled in: a Recipe is a file and references them by id, and no file
can define one. The
model behind them is in [`organizer.md`](organizer.md); where a Recipe file
lives and how it is switched on and off is in
[`configuration/capture.md`](../../configuration/capture.md#recipes).

`dgs capture --actions` prints this same catalogue from the binary in front of
you, which is the answer to "does the version I have carry this one?". A test
checks that this document names every Action the binary registers, so a new
Action cannot be added without appearing here — but the binary is still the
authority on what it will actually do.

| Action | Writes | Needs |
| --- | --- | --- |
| [`obsidian.daily.append`](#obsidiandailyappend) | the day's note in the vault | `createdAt`; `content` optional |
| [`obsidian.location.append`](#obsidianlocationappend) | the running list of places | `createdAt`; `content` optional |
| [`gpx.daily.append`](#gpxdailyappend) | a daily GPX in the configured directory | `createdAt`, `coordinates`; `content` optional |
| [`apple.reminders.create`](#applereminderscreate) | a reminder, due at a time | `title`, `due_at` |
| [`apple.reminders.at_place`](#appleremindersat_place) | a reminder, at a place | `title`, `coordinates` |
| [`apple.notes.create`](#apple-actions) | — not implemented | `title`, `content` |
| [`apple.calendar.create`](#apple-actions) | — not implemented | `title`, `start_at` |

## `obsidian.daily.append`

Appends the Capture to the day's note as one nested entry.

- Adds it under the configured section, adding that heading when the note has
  none.
- Creates the note from `daily-note.md` when the day has none.
- Writes nothing the second time: the entry carries the Capture's id, is added
  once, and a second run says why it wrote nothing. Deleting the entry by hand
  is how it is written again.

Needs `createdAt`. `content` is optional: a Capture nobody wrote anything
about is still written, and the template leaves out the lines its text would
have filled. Configured by `capture.obsidian.daily_note`
and `capture.obsidian.section`; shaped by the `daily-entry.md` and
`daily-note.md` templates.

**Parameters** — a per-run override, edited in `FIELDS`:

| Name | Meaning | Default |
| --- | --- | --- |
| `section` | The heading this Capture is written under. | `capture.obsidian.section` |

## `obsidian.location.append`

Puts the Capture at the top of the running list of places, under its day.

- Newest first, under a marker for the day it was captured.
- Moves any year that has rolled over into
  `capture.obsidian.location_archive` as it goes.
- Writes nothing the second time, on the same terms as the daily entry.

Needs `createdAt`; `content` is optional, on the same terms. Configured by
`capture.obsidian.location_note` and `capture.obsidian.location_archive`;
shaped by the `location-entry.md` template, which also decides the map services
the line carries and the vault command a coordinate links to.

## `gpx.daily.append`

Writes one timed GPX waypoint for the Capture's WGS-84 position. The file is
`<capture.gpx.directory>/YYYYMMDD.capture.gpx`, using the date and offset
recorded by the Capture. Captures from the same day share the file, regardless
of workflow. The waypoint name is the resolved `content` note text, when
present, using the same workflow mapping and enrichment as the Location Action.
The Capture id lives in a GPX waypoint extension. A second run adds nothing if
the id is already present, or if a waypoint already has the same coordinates
and time at the precision stored by GPX. Only GPX files created
by `dgs` are extended; an existing GPX from another creator is left untouched.
The directory must be configured before running the Action. A Recipe can name
`gpx.daily.append` and offer it only when `coordinates` resolve.

## Reminders

Both reminder Actions write to Apple Reminders through EventKit, which `dgs`
links on macOS with cgo, so there is nothing to install beside it. The
Reminders scripting dictionary has no location alarm, which is why it is
EventKit rather than AppleScript. The first run asks for access to Reminders;
on another system, or in a build with `CGO_ENABLED=0`, the Action refuses with
the reason.

Shared by both:

- The Capture's `content`, when it has one, becomes the reminder's notes.
- Writes nothing the second time: the notes carry the Capture's id, and a list
  already holding a reminder with it is left alone. Deleting the reminder is
  how it is created again. Nothing is ever edited or deleted.

### `apple.reminders.create`

A reminder due at a time, with an alert at that time.

Needs `title` and `due_at`. `due_at` is RFC 3339, or `2026-09-15 09:00`
read in local time. `FIELDS` has no datetime control yet, so today it comes
from a workflow source.

**Parameters**:

| Name | Meaning | Default |
| --- | --- | --- |
| `list` | The Reminders list it is written into. | `capture.apple.reminders.list`, else Reminders' default list |

### `apple.reminders.at_place`

A reminder that fires on arriving at, or leaving, the Capture's position. The
position is the one the location note reads — a workflow may keep it in the
payload, see
[`configuration/capture.md`](../../configuration/capture.md#a-position-kept-in-the-payload) —
and is named by `place.name` when there is one. It fires on a device with
location access for Reminders, usually the phone, once iCloud has synced it.

Needs `title` and `coordinates`.

**Parameters**:

| Name | Meaning | Default |
| --- | --- | --- |
| `when` | `arrive` or `leave`. | `arrive` |
| `radius` | How close counts as there, in metres. | `capture.apple.reminders.radius`, else 150 |
| `list` | The Reminders list it is written into. | `capture.apple.reminders.list`, else Reminders' default list |

## Apple actions

`apple.notes.create` and `apple.calendar.create`
declare what they need and name what they would write, but talk to no Apple API
yet. A Recipe may name one; running it refuses rather than reporting a success
that did not happen.

They exist so the model is exercised against Actions needing fields the
Obsidian ones do not — a `datetime` due date, a `start_at` required only when
the event is not all day — rather than being designed around one workflow.

## Fields an Action may ask for

| Field | Input | Where it comes from |
| --- | --- | --- |
| `createdAt` | text | the Capture's index |
| `coordinates` | text | the Capture's index — the position, as one field |
| `content` | multiline | the Capture, or typed in `FIELDS` |
| `title` | text | typed in `FIELDS` |
| `due_at` | datetime | typed in `FIELDS` |
| `start_at` | datetime | typed in `FIELDS`, required unless `all_day` |
| `all_day` | text | typed in `FIELDS` |
| `tags` | multi-select | typed in `FIELDS` |

A Recipe's `match.requires_any` may name any of these too, which is how a
Recipe says it is only worth offering for a Capture that carries something —
`coordinates` for one that knows where it was.

`dgs capture --recipes` lists the fields actually in use by the Recipes that
loaded, alongside where each Recipe came from.
