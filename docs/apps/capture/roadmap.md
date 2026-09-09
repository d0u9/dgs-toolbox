# Capture Roadmap

Work deliberately deferred, with the reason it was deferred. This is not a wish
list: an item is here because it was cut from a change that shipped without it,
and the note says what shipped instead.

## Deferred

- **`datetime` and `multi_select` input controls.** `FIELDS` renders `text`
  inline and `multiline` in an overlay; the other two input types have no
  control yet, so a requirement carrying one can be seen but not filled. The
  Recipes that need them — Reminder and Calendar — therefore stay blocked and
  cannot be run. They were built anyway, to keep the organizer model honest
  against Actions whose requirements differ from Obsidian's, and are excluded
  from the first pass on purpose.
- **Real Action implementations.** Every Action declares its requirements and
  resolves its target, but none of them writes anything: running a Capture
  shows what it would do and appends to `organize.json`. Implementing them
  brings Obsidian vault paths, idempotent upsert, and failure handling, none of
  which should shape the model. `RecordedAction.executed` already exists for
  that day and stays false until then.
- **User-defined Recipes.** Recipes are compiled in. Configuration comes once
  the built-in set has been used enough to know what is worth configuring, and
  will live with the Actions rather than under `capture.route`.
- **Configuration migration.** Config decoding is strict, so removing a field
  breaks every existing config file with an `unknown field` error that does not
  say what to delete. Removing `capture.route.destinations` broke exactly this
  way. Either downgrade unknown fields to a warning or recognise and ignore
  fields known to be retired.
