# Capture Organizer Model

The data abstraction underneath Capture organizing. It is independent of any
TUI: the sessions in [`scan.md`](scan.md) and [`route.md`](route.md) are one
front end over this model, and nothing here may be shaped by their layout.

Assigning a Capture to a single configured destination is the degenerate case
of this model: a Capture is organized by choosing a *Recipe*, filling in whatever the
Recipe still needs, and executing the *Actions* the Recipe expands into. The
three concepts stay separate and are never collapsed into one another.

- **Recipe** — a way of organizing one class of Capture. It is named for the
  outcome ("Location + Daily Note"), not for the mechanics.
- **Action** — a single concrete execution unit a Recipe expands into
  (`obsidian.daily.append`). A Recipe is not an Action.
- **Missing Fields** — the inputs still lacking for this Capture, right now.
  Not a property of the Recipe, but of Recipe × enabled Actions × Capture ×
  enrichment.
- **Selection** — what the user has decided about one Capture: which Recipe,
  which of its Actions are enabled, and what they filled in.

## Pipeline

```text
Capture
   ↓
FindCandidateRecipes()
   ↓
Select Recipe
   ↓
Toggle Actions          (Recipe supplies the default enabled set)
   ↓
Resolve Fields          (Context)
   ↓
Find Missing Fields
   ↓
Enrich                  (prompt for missing fields)
   ↓
Build Action Plan
   ↓
Preview
   ↓
Execute
```

## Recipe

A Recipe has exactly three parts: `match`, `fields`, `actions`.

```yaml
id: obsidian_location_daily
name: Location + Daily

match:
  workflows:
    - been_here
    - photo_note

fields:
  - id: place.name
    required: true
    input: text
  - id: content
    required: true
    input: multiline
  - id: tags
    required: false
    input: multi_select

actions:
  - id: obsidian.location.upsert
  - id: obsidian.daily.append
  - id: capture.archive
```

`match` does not trigger execution. It decides which Recipes are *offered* for
a Capture, so a caller offers four candidates rather than thirty. A `been_here`
Capture offers `Location`, `Location + Daily`, `Daily`, `Archive`; a
`quick_mark` Capture offers `Daily`, `Reminder`, `Calendar`, `Apple Note`,
`Archive`.

## Actions declare their own requirements

Each Action declares what it needs, and the effective required set is the
**union of the enabled Actions' requirements plus the Recipe's own extra
declarations**. A Recipe never restates its Actions' requirements by hand —
otherwise adding a required field to an Action silently breaks every Recipe
that forgot to sync.

```text
obsidian.location.upsert  requires  place.name, coordinates.latitude,
                                    coordinates.longitude
obsidian.daily.append     requires  createdAt, content
capture.archive           requires  —

Recipe "Location + Daily", all three enabled, therefore requires
  place.name, coordinates.latitude, coordinates.longitude,
  createdAt, content
  + anything the Recipe itself adds (e.g. project)
```

## Actions declare their effects

An Action also declares what it does outside the Capture: what it creates, what
it changes in place, and what running it twice does. Declared on the Action
rather than written into a screen, so the session and the generated reference
say the same thing, and so a new Action arrives with its own explanation instead
of needing one added somewhere else.

```text
obsidian.location.upsert  Creates Locations/<place name>.md, or updates it in
                          place when it exists
obsidian.daily.append     Appends one entry to the note for the day the Capture
                          was taken
                          Creates that note when the day has none
```

This is what a reader needs before running a plan, and the confirmation dialog
shows it beside each Action for exactly that reason.

## Actions declare their parameters

A requirement is what an Action needs; a parameter is how it behaves. The
heading a Capture is written under is a parameter: a default answers for almost
every Capture, and the occasional one wants somewhere else.

Parameters resolve in layers, each more specific than the last: what the Action
ships with, what the configuration says, and what this one Capture was given.
The override is per-Capture and is not remembered across them — one that stayed
on would quietly apply to Captures nobody meant it for.

The values an Action ran with are written into its record. Where a Capture was
written is part of what happened, and a reader coming back months later would
otherwise assume the default.

## Actions are toggleable per Capture

A Recipe supplies the **default** enabled set — every Action it lists. The user
may disable individual Actions for one Capture without leaving the Recipe, so
that Capture can skip `capture.archive` while the rest of the plan stands.

A disabled Action contributes nothing: it is absent from the Action Plan, and
its requirements are absent from the required set, so a field only it needed
never blocks the Capture. Requirements the Recipe declares itself belong to no
Action and therefore **stay required regardless of which Actions are enabled**.

Because the enabled set is per-Capture state rather than a property of the
Recipe, it is passed in rather than read off the Recipe:

```go
type Selection struct {
    Recipe     RecipeID
    Enabled    map[ActionID]bool   // defaults from the Recipe, user may override
    Enrichment map[FieldID]any
}

func MissingFields(ctx Context, recipe Recipe, enabled []ActionID) []FieldRequirement
func Build(ctx Context, recipe Recipe, enabled []ActionID) []ActionPlan
```

Disabling every Action is legal but yields an empty plan; such a Capture counts
as blocked, never as ready. There is no separate single-select mode — enabling
exactly one Action is just one state of the same set, and how many an Action
list starts with is the Recipe's decision.

A standing preference to always skip an Action belongs in a Recipe of its own
(`Location + Daily` beside `Location + Daily (keep)`), not in this per-Capture
override, which is deliberately transient.

## Fields are logical, not JSON paths

A field is a `FieldID`, never a hardcoded path into the Capture JSON, because
the same logical field is sourced differently per workflow:

```text
been_here   content ← payload.note
photo_note  content ← payload.text
quick_mark  content ← payload.mark
```

```text
Capture JSON → Field Resolver → Logical Fields → Recipe
```

Some logical fields have no counterpart in the index at all and are composed
from what it does carry. `place.name` is the case that matters: the index has
no name field and no free-form address to borrow one from, so a name is
composed from the two most specific structured parts present — `Chuo, Osaka`
rather than the whole locality-to-country chain, which reads as an address
instead of a name. A composed value is a starting point, not the Capture's own
data: it satisfies the requirement so nothing is blocked needlessly, and the
user may still replace it.

A field requirement carries enough to drive an input surface on its own — a
bare string list is not sufficient, because the caller must know to ask for a
datetime rather than free text:

```go
type FieldRequirement struct {
    Field    FieldID
    Label    string
    Required bool
    Input    InputType   // text, multiline, multi_select, datetime, …
    When     Predicate   // optional; conditional requirement
    Validate Validator
}
```

`When` covers conditional requirements — Calendar needs `start_at` only when
`all_day` is false, and `end_at` only when `has_end` is true. A requirement
whose `When` evaluates false is skipped entirely when computing missing fields.

## Context

Recipes never read the Capture JSON directly. A `Context` is resolved first and
holds the Capture plus the user's enrichment, read in priority order:

```text
user-supplied enrichment  →  original Capture data  →  derived data
```

Only the middle layer is the Capture's own. A caller distinguishes it to decide
what may be edited: enriched and derived values are the user's to change,
Capture data is not.

The original Capture on disk is never mutated. Given a Capture carrying
`coordinates`, `place.city = Epping`, and `place.region = NSW`, the Context
already composes `place.name = Epping, NSW`; supplying `Epping Station` as
enrichment replaces it, and the Capture file stays untouched either way.

## Missing fields

Requirements are attributed to the Action that declared them, so a caller can
say *which* Action is blocking rather than only that something is missing. A
field required by two enabled Actions is reported once per Action, and filling
it satisfies both.

`Fields` is the view for filling things in rather than for attributing them: it
returns the union across the enabled Actions, deduplicated, split into what the
Context cannot yet answer and what it can. `Needs` is the other direction —
one Action's own requirements — so attribution and completion are separate
questions with separate answers instead of one list trying to serve both.

```go
func MissingFields(ctx Context, recipe Recipe, enabled []ActionID) []FieldRequirement {
    var missing []FieldRequirement
    for _, req := range recipe.RequiredFields(enabled) {
        if req.When != nil && !req.When(ctx) {
            continue
        }
        value, ok := ctx.Get(req.Field)
        if !ok || !req.Validate(value) {
            missing = append(missing, req)
        }
    }
    return missing
}
```

## Action Definition vs Action Plan

An **Action Definition** says what `obsidian.daily.append` *is*. An **Action
Plan** says what it will do to *this* Capture — which file, which content.
Preview renders the plan, and execution consumes it. No Action is implemented
yet: each one declares its requirements and resolves its target, and running a
plan reports what it would do rather than doing it.

```text
Recipe + enabled Actions + Context → Build() → []ActionPlan
```

The plan holds one entry per **enabled** Action, in the order the Recipe lists
them. An Action with unmet requirements still appears in the plan with its
target unresolved, so the blockage is visible rather than silently dropped.

```json
{
  "action": "obsidian.daily.append",
  "target": "Daily/2026-09-09.md",
  "content": "- 这里晚上可以再来看看"
}
```

## Worked example

Capture:

```json
{
  "source": { "workflow": "been_here" },
  "createdAt": "2026-09-09T21:31:22.900+10:00",
  "coordinates": { "latitude": -33.7691, "longitude": 151.082 },
  "place": { "city": "Epping", "region": "NSW" }
}
```

Candidates: `Location`, `Location + Daily`, `Daily`, `Archive`. Selecting
`Location + Daily` resolves:

```text
createdAt   ✓    place.city  ✓    place.name  ~ Epping, NSW (composed)
latitude    ✓    region      ✓    content     ✗
longitude   ✓
```

so only `Note` is strictly missing, attributed to `obsidian.daily.append`. The
composed place name is offered for editing rather than demanded, and renaming
it to `Epping Station` is a choice. With all three Actions enabled, the plan is:

```text
1. UPSERT   Obsidian/Locations/Epping Station.md
2. APPEND   Obsidian/Daily/2026-09-09.md
3. ARCHIVE  Capture 20260909213122900-4620
```

Disabling `capture.archive` leaves the first two entries unchanged; disabling
`obsidian.daily.append` drops entry 2 and, with it, the `content` requirement,
so `Note` is no longer asked for.

## The organize record

A Capture that has been organized carries `organize.json` in its own directory,
beside `index.json`. The two are named for who writes them: `index.json` is the
producer's record of what was captured and stays untouched, `organize.json` is
this tool's record of how it was organized. Keeping the decision next to the
Capture rather than in a session file means a Capture organized in one session
is recognised as handled in the next, and stays recognised if the Capture is
moved.

A Capture may be organized more than once — a new Action appears, or the first
pass turns out to have been wrong — so the record holds a list of runs rather
than one decision. Each run records what was decided at that moment:

```json
{
  "schema": "v1",
  "runs": [
    {
      "organizedAt": "2026-09-09T21:40:12+10:00",
      "recipe": "obsidian_location_daily",
      "recipeName": "Location + Daily",
      "actions": [
        { "action": "obsidian.location.upsert", "target": "Locations/Epping Station.md", "executed": false },
        { "action": "obsidian.daily.append", "target": "Daily/2026-09-09.md", "executed": false }
      ],
      "fields": { "place.name": "Epping Station" }
    }
  ]
}
```

Each recorded Action says whether it `executed`, whether it was `skipped`
because its work was already done, and the `error` that stopped it otherwise, so
a failed pass explains itself later rather than only to whoever was watching.

A Capture counts as **organized** when its most recent run finished — every
Action executed. A run that failed part way through is history, not a state to
move on from, so the Capture stays where it was. `fields` holds the enrichment only — what the user supplied — not
the values that resolve from the Capture, which are not this record's to keep.

Organizing a Capture again **appends** a run: a pass made against an older set
of Actions stays visible beside the one that followed it, which is the whole
point of keeping the record next to the Capture. Nothing is overwritten. What
marks a Capture handled is the presence of a run, not the presence of the file,
and callers with room for one line show the most recent run.

## Recipe files

A Recipe is defined by one YAML file in the Recipe directory —
`<config dir>/capture/recipes`. A folder rather than one list because Recipes are added one at a
time and each should diff on its own; YAML rather than JSON because a Recipe is
written by hand and wants comments explaining why it exists.

```yaml
# Everything noted during work hours goes into the work vault's daily note,
# tagged with the project it belongs to.
name: Work Daily

match:
  workflows: [been_here, quick_mark]

fields:
  - id: project
    label: Project
    required: true

actions:
  - id: obsidian.daily.append
  - id: capture.archive
```

- **The filename is the id.** `work_daily.yaml` defines `work_daily`. A file
  cannot claim an id that disagrees with where it lives, and renaming the file
  is how a Recipe is renamed.
- **A file replaces the built-in with the same id**, in place, rather than
  patching it. A user file is the whole Recipe, so reading it tells the whole
  story without also reading what it inherited.
- **Actions are objects, not bare strings** (`- id: …`). They need nothing else
  today; the shape leaves room for per-Recipe Action parameters without
  rewriting every existing file.
- **An unknown key is an error**, not an ignored line — a misspelled key would
  otherwise change nothing and say nothing. So is naming an Action this binary
  does not have: the Recipe would silently do less than it claims.
- **A bad file fails alone.** The rest of the directory still loads and the
  session still opens, because a Recipe is not a precondition the way the
  configuration file is. `dgs capture --recipes` lists what loaded, where each
  Recipe came from, and every file that was rejected with its reason.

What a file configures is composition — which workflows a Recipe matches, which
Actions it runs, what it asks for beyond them. It cannot define an Action:
Actions are compiled in, and a Recipe references them by id.

## Executing a plan

`Execute` carries a plan out in order and stops at the first failure, because
the Actions of one Recipe are usually related — a location note and the daily
entry pointing at it — and running the rest after one has failed leaves a state
nobody asked for. What did happen is reported rather than rolled back: an entry
added to a note cannot be un-added safely.

Each Action reports one of three outcomes: it ran, it was **skipped** because
its work was already done, or it failed with a reason. An Action that has been
declared but not implemented fails with `ErrNotImplemented` rather than
reporting success — a plan claiming everything is done while nothing was written
is worse than one that refuses. `Unimplemented` names those Actions so a caller
can refuse to offer a plan that cannot work, instead of accepting it and failing
afterwards.

Only `obsidian.daily.append` is implemented. It writes the Capture into the
daily note for the day it was taken:

- **Where** is configured, not discovered. `capture.obsidian.daily_note` is a
  path relative to the vault, written as a template over the date:
  `00 Daily Log/{{.Year}}/{{.Date}}.md`. A vault does keep its own daily note
  settings, and reading them looked at first like sparing the reader a
  duplicate — but a vault says where the plugin in use puts notes, which is not
  the same question as where this tool should write, and a wrong guess files a
  Capture where nobody is looking. What is not configured is an error.
- **What a missing note is created from** is `daily-note.md` in the template
  directory. A note is never created from nothing — an empty note among
  templated ones is one the reader has to repair later — so a directory without
  one is an error naming the file it expected, rather than a bare note.
  Appending to a note that already exists needs no template.
- **What** comes from a template, below.
- **Under which heading**: its own section, `# DGS` by default and configured by
  `capture.obsidian.section`, added at the end of the note when it has none. The
  configured value may carry its own hashes — `## Captured` asks for a
  second-level heading — and a section then ends at the next heading of its own
  level or shallower, so a subheading inside it still belongs to it. Its own section so what this tool writes stays
  distinguishable from what the reader wrote, and so an entry is never appended
  onto the end of somebody else's sentence.
- **Once.** The entry carries the Capture's id as an Obsidian block reference
  (`^dgs-<id>`), and an entry already in the note is skipped rather than added
  again. The note is the record of what it holds: deleting the line by hand is
  enough to have it written again, where bookkeeping kept anywhere else could
  claim a line exists that no longer does. The reference is also a link target,
  so what was written can be pointed at.
- **Nothing is guessed.** With no vault configured the Action refuses.

## Templates

What an Action writes is a Go `text/template`, not a shape invented here: a
standard engine brings conditionals, loops over a Capture's attachments, and
errors reported when the template is read rather than as a blank discovered in a
note later. The default is compiled in, so the tool writes sensibly before
anything is configured, and a directory of templates overrides the defaults **by
filename** — replacing one does not mean supplying them all.

```gotemplate
- {{.When}} {{.ID}}
{{- range .ContentLines}}
  - {{.}}
{{- end}}
  - {{.Coordinates}}
  - address: {{.Address}}
```

A rendered line is dropped when every placeholder on it resolved to nothing —
`- address:` with no address says nothing and still costs a row. That is how
"write only what the Capture knows" is expressed without every template having
to say it, and why the common case needs no conditionals.

The test is what the placeholders produced, not what the line looks like
afterwards, so a line written as a literal heading (`- Content:`) is kept while
one holding only a label and an empty value is not. Fields print themselves
wrapped in markers the renderer strips for exactly this, which is also why a
field is a `Value` rather than a plain string; its underlying kind is still a
string, so `{{if .Place}}` and `{{range}}` behave as expected.

The data a template may name is a declared struct rather than a map, so a
misspelled field fails when the template is parsed: `When`, `Date`, `Time`,
`ID`, `Content`, `ContentLines`, `Coordinates`, `Altitude`, `Address`, `Place`,
`Workflow`, `App`, `Capture`, and `Attachments` (each with `Name` and `Kind`). `indent`,
`join`, `trim`, and `default` are available as functions.

`ContentLines` is the Capture's text one line per item, with blank lines
dropped: a note written across several lines is several things worth reading,
and a blank line between them is spacing rather than content.

### The daily note's own template

`daily-note.md` is rendered the same way, against a different data set: `Date`,
`Year`, `Month`, `Day`, `Weekday`, `DayOfYear`, and `Country`, `Region`, `City`,
`Locality` and `Place` from the Capture that prompted the note.

This is the answer to a vault template written for a plugin that asks the reader
questions as it runs. Half of what it asks need not be asked: the date is the
date, and the Capture already carries where it was taken. The rest — weather,
mood, who was there — genuinely cannot be answered without a person, and is left
empty for them to fill in.

It is also how the same tool serves vaults that are nothing alike. The template
is the mapping: it says which of *this* vault's fields take which value, in this
vault's own names and its own language, and a value it does not want is simply
not referenced. Nothing is pruned from a created note, unlike an entry, because
a note is a document the reader will edit and a line quietly removed is one they
will wonder about.

## Built-in Recipes

The built-ins are always present, so Route has something to offer before any
file exists:

| Recipe | Workflows | Actions |
| --- | --- | --- |
| Location | `been_here` | location.upsert |
| Location + Daily | `been_here` | location.upsert, daily.append |
| Photo + Location | `photo_note` | location.upsert, daily.append |
| Daily | `been_here`, `photo_note`, `quick_mark` | daily.append |
| Apple Note | `photo_note`, `quick_mark` | notes.create |
| Reminder | `quick_mark` | reminders.create |
| Calendar | `quick_mark` | calendar.create |

A file in the Recipe directory is layered over this set. `Archive` matches every
workflow, so no Capture is ever left with an empty candidate list. The two location Recipes additionally require the Capture to
carry a location at all, so a `been_here` with neither coordinates nor place
falls back to `Daily` and `Archive`.

The Apple Actions declare their requirements and name their targets but talk to
no Apple API yet. They exist so the model is exercised against Actions that
need fields the Obsidian ones do not — a `datetime` due date, a conditional
`start_at` that is required only when the event is not all day — rather than
being designed around one workflow.

## Implementation order

First cut is these types and functions, headless and table-tested, with no
Apple integrations at all:

```go
type Recipe
type FieldID
type FieldRequirement
type ActionDefinition
type ActionPlan
type Selection
type Context

func FindRecipes(capture Capture) []Recipe
func MissingFields(ctx Context, recipe Recipe, enabled []ActionID) []FieldRequirement
func Build(ctx Context, recipe Recipe, enabled []ActionID) []ActionPlan
```

The package carries no bubbletea dependency: a Capture goes in, candidate
Recipes come out; two fields are supplied as enrichment, three Action Plans
come out. Every domain judgement lives here, and the TUI only calls these
functions and draws the result.

`been_here → Location / Location + Daily` is driven end to end before
`photo_note` and `quick_mark` are added, and before any of Apple Notes,
Reminders, or Calendar is touched — those bring their own API detail and must
not shape the model.
