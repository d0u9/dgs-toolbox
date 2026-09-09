# Capture Roadmap

Work deliberately deferred, with the reason it was deferred. This is not a wish
list: an item is here because it was cut from a change that shipped without it,
and the note says what shipped instead.

## Deferred

- **An Archive session.** Archiving is not a Recipe Action: it does not write a
  Capture somewhere, it takes the Capture out of reach, and a Capture that has
  been archived can no longer be worked on. It belongs to a third Capture tab
  that acts on Captures already organized, not to the session that organizes
  them. `capture.archive` was removed from the Actions rather than left in place
  waiting, so no Recipe can quietly retire a Capture as a side effect of
  organizing it.

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
- **Per-Recipe Action parameters.** A Recipe names its Actions but cannot
  configure them, so every Recipe writing a daily note writes to the same place.
  Recipe files already spell Actions as objects (`- id: …`) so a `with:` can be
  added without rewriting them, and the Action definitions would need to declare
  what they accept.
- **Action documentation as a generated artefact.** `--recipes` covers the
  registry in text. An `--actions` report in text, Markdown, JSON, and HTML —
  plus a generated `docs/apps/capture/actions.md` guarded by a staleness test,
  the way `cmd/mockcapture` guards the mock root — keeps reference material from
  drifting away from the registry.
- **Configuration migration.** Config decoding is strict, so removing a field
  breaks every existing config file with an `unknown field` error that does not
  say what to delete. Removing `capture.route.destinations` broke exactly this
  way. Either downgrade unknown fields to a warning or recognise and ignore
  fields known to be retired.
