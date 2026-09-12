# Capture Roadmap

Work deliberately deferred, with the reason it was deferred. This is not a wish
list: an item is here because it was cut from a change that shipped without it,
and the note says what shipped instead.

## Deferred

- **Archiving a Capture from a Recipe.** The Archive session ships — see
  [`archive.md`](archive.md) — and moves organized Captures out of the root by
  hand. `capture.archive` remains absent from the Actions on purpose, so no
  Recipe can quietly retire a Capture as a side effect of organizing it; an
  Action for it would need to answer what happens to a Capture a later pass
  wants to organize again.

- **`datetime` and `multi_select` input controls.** `FIELDS` renders `text`
  inline and `multiline` in an overlay; the other two input types have no
  control yet, so a requirement carrying one can be seen but not filled. The
  Recipes that need them — Reminder and Calendar — therefore stay blocked and
  cannot be run. They were built anyway, to keep the organizer model honest
  against Actions whose requirements differ from Obsidian's, and are excluded
  from the first pass on purpose.
- **The remaining Action implementations.** `obsidian.daily.append` writes; the
  rest declare and refuse. The Apple Actions need their APIs and the input
  controls their fields ask for.
- **A note per place.** `obsidian.location.append` records places as one running
  timeline, which is what the vault it was written for keeps. A note per place —
  the earlier `obsidian.location.upsert` — is a different thing, deferred for
  want of a need rather than of a design. Writing a note that may already exist and may have been edited by hand
  is a different problem from appending to one: it means deciding what counts as
  the same place when the name is typed by hand, whether a second visit's
  coordinates replace the first's, and how to change one frontmatter key while
  leaving every other key — and the meta-bind controls among them — exactly as
  they were.

  The cheaper design to reach for first, if the need appears: write
  `[[Place Name]]` into the daily entry and create no note at all. Obsidian
  shows it as a link waiting to be filled, its backlinks already list every day
  the place was visited, and a note gets created — with the reader's own
  template — only if they ever want to write something there. That answers every
  question above by not asking it.
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
