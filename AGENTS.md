# AGENTS.md

## Public repository — privacy is mandatory

**This repository is public. Anyone can access, copy and retain its contents
and Git history. Never put private, personal, confidential or deployment-specific
data in this repository, even temporarily or in an untracked or ignored file.**

- This applies to code, comments, docs, examples, tests, fixtures, snapshots,
  logs, screenshots, generated files, commit messages and PR descriptions.
- Never include real credentials, tokens, passwords, keys, secrets, personal
  records, account identifiers, local user paths, hostnames, addresses or network
  inventories. Encryption, redaction of only the secret, and `.gitignore` do not
  make real private data suitable for this public repository.
- Use wholly synthetic cases: `alice`, `bob`, `host-a`, `node-1`; domains under
  `example.com`, `example.net`, `example.org` or `.test`; IPs in `192.0.2.0/24`,
  `198.51.100.0/24`, `203.0.113.0/24` or `2001:db8::/32`; invented private
  networks such as `10.0.0.0/24`; invented MACs starting `02:00:00`.
- Keep real configuration, inputs, secrets and outputs outside the repository.
  Reproduce bugs with invented data; implement generic capabilities here and
  keep deployment-specific definitions in private configuration.
- Check changes and generated artifacts for private data before staging or
  sharing. Never bypass privacy hooks (`--no-verify`) or disguise rejected values.
- If real data seems necessary, stop and ask the maintainer for a synthetic
  alternative. If private data is found, stop propagating it and report without
  quoting it. Deleting it in a later commit does not undo disclosure.

## Project and scope

- Go, Cobra, Bubble Tea; Bubbles and Lip Gloss when useful. `dgs` is the only
  installed executable: copying it must suffice. Embed runtime assets and native
  integrations; no required or optional helper executable or script on `PATH`.
  Native APIs use platform/cgo build tags and clear unsupported fallbacks.
- Follow confirmed app designs in [docs/apps/](docs/apps/). Do not expand Photo
  Encode beyond JPEG exports, GPX beyond confirmed milestones, or add runtime
  plugin loading or speculative infrastructure. Bundled plugin management is
  documented in [docs/apps/plugins/](docs/apps/plugins/).
- During development, use `go.work` to import local `../rhumb` rather than
  binding development to a released version: a change may require coordinated
  edits in both repositories. Keep those edits within the requested scope.

## Data integrity

- Photo Import: [design](docs/apps/photo/import.md). File operations begin only
  when the user starts Processing. Use bounded whole-file workers,
  same-directory `.dgs-part` files, source SHA-256 during copy, independent
  destination readback, verified atomic publication, whole-file retry and
  versioned `.dgs-state`. **Never publish the final name before readback matches
  the source digest.** Keep processing outside the TUI and test with filesystems.
  Reuse `internal/verifiedcopy`, also used by Box.
- Box: [designs](docs/apps/box/). Per-scan YAML sidecars are metadata truth;
  publication follows the same readback contract. Paths encode event year/month
  (intake date when absent); date changes move files by rename. Discard moves to
  Box trash; nothing is deleted. Keep the in-memory index until the conditions
  in [index.md](docs/apps/box/index.md) justify revisiting SQLite.
- GPX: [design](docs/apps/geo/gpx.md). Never write to a GPX that `dgs` did not
  create. Edits belong in its sidecar; embedding them requires a new GPX.
  A recording cannot be recorded again; a sidecar can be entered again.
- Credentials: [designs](docs/apps/cred/). Decrypted content stays in memory
  unless a user-confirmed Action writes it. Replace a vault only after the new
  vault decrypts back to the same SHA-256.
- Original files and sidecars are truth. Caches/indexes/databases live locally,
  outside the described tree, at documented configurable paths. Write originals
  first, then cache. Missing, corrupt or unknown-version caches are discarded
  and rebuilt, never migrated or repaired; test that rebuilding yields the same
  result. Claim only the documented filesystem/API durability boundary.

## Interfaces and architecture

- Read [docs/tui.md](docs/tui.md) before changing interactive flows and follow
  its component links. Read [docs/web.md](docs/web.md) before changing served
  pages. App-specific decisions stay in app docs.
- One process, one active leaf, one top-level `tea.Program`. The shell owns
  selection, lifecycle, sizing and bars; leaves expose workspace models, status
  and hints, never nested programs. Switching commands creates a fresh model.
- Leaves open their TUI; incomplete commands open scoped pickers. Registered
  CLI actions take arguments, write stdout and return without a TUI, and are
  excluded from pickers. Esc at a leaf root returns to the picker; there it exits.
- Top and status bars are exactly one row each; shorten or omit on narrow
  terminals, never wrap. Top: path/context left, local time right. Status:
  state left, summary center, hints right. Workspace belongs to the active model.
- Reuse algorithms before adding them. Domain packages take plain values and
  import no TUI, HTTP, configuration or app code. Keep one concern per package,
  parameters as arguments with documented named defaults, and constructed-input
  tests beside the algorithm. Apps/handlers compose results; browser code only
  computes interaction geometry.

## Documentation and validation

- Update design docs only for confirmed decisions; leave undecided parts open.
- Setting changes must update both the relevant [configuration reference](docs/configuration/)
  and its [index](docs/configuration/index.md), including names, meanings and
  defaults. Update affected [examples](examples/); their load test must pass.
- Keep registered Actions and descriptions current in
  [Cred Actions](docs/apps/cred/actions.md) and
  [Capture Actions](docs/apps/capture/actions.md); catalogue tests check coverage.
- Run checks relevant to the change. Use `go test ./...` for repository-wide Go
  validation; timeline changes also use `make test-timeline` for native,
  WebAssembly and bundled plugin parity. Do not install or deploy just to test.
