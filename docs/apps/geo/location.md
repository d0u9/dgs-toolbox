# Location

Status: parked implementation checkpoint. The location Action is not registered,
so `dgs geo location` is not available in the CLI, help or completion. Single-file
macOS authorization remains unresolved. The examples below describe the tested
prototype, not an enabled command. See
[the investigation handoff](location-status.md).

The prototype `dgs geo location` prints the current device location once and exits. It is a
CLI Action, outside the TUI picker. macOS builds with cgo call Core Location
and Contacts through `internal/desktop/location`; other builds refuse clearly.
No helper executable is installed or invoked.

```sh
dgs geo location
dgs geo location --json
dgs geo location --format '%latitude %longitude %address'
```

`-j/--json` prints all 22 fields as one JSON object. Values are strings, absent
values are null. `-f/--format` replaces the following placeholders, with absent
values replaced by an empty string; unknown placeholders remain unchanged:

| Fields | Meaning |
| --- | --- |
| `%latitude`, `%longitude` | Degrees, six decimal places; north/east positive. |
| `%altitude` | Metres, two decimal places. |
| `%direction` | Course in degrees from true north; unavailable may be negative. |
| `%speed` | Metres per second, truncated to integer; unavailable may be negative. |
| `%h_accuracy`, `%v_accuracy` | Metres, truncated to integer; unavailable may be negative. |
| `%time` | Location timestamp in UTC, `yyyy-MM-dd HH:mm:ss Z`. |
| `%address`, `%name` | System-formatted postal address and place name. |
| `%isoCountryCode`, `%country`, `%postalCode` | Country code, country and postal code. |
| `%administrativeArea`, `%subAdministrativeArea` | Administrative subdivisions. |
| `%locality`, `%subLocality` | Locality and subdivision. |
| `%thoroughfare`, `%subThoroughfare` | Street and street number. |
| `%region` | Placemark region identifier. |
| `%timeZone`, `%time_local` | Place time zone identifier and timestamp in that zone. |

Default format is `%latitude %longitude`. Output ends with a newline.
JSON and address-related placeholders trigger system reverse geocoding; plain
coordinates do not. Reverse geocoding may need a network connection.
`-v/--verbose` adds capability and authorization diagnostics before output,
including before a native error; combined with JSON it is no longer pure JSON.
`--version` reports the location command interface version without locating.
`-h/--help` prints usage. `--watch` is not implemented.

Core Location requests best accuracy with a two-metre distance filter. The
native call is bounded by ten seconds, including any reverse geocoding; on
completion or timeout it stops updates and cancels geocoding. This intentionally
also bounds geocoding, whereas CoreLocationCLI cancels its timeout after the
first location. Format substitution is deterministic and does not expand tokens
inside returned values. JSON takes precedence over format when both are given.

## macOS authorization validation

The existing macOS install embeds the dgs Info.plist and signs the single
binary with identifier `dev.dgs-toolbox.dgs`. The plist now contains location
usage explanations alongside Reminders explanations. This is not sufficient
to obtain location authorization in the observed macOS 26.6.2 environment.
The location command is implemented but its single-file deployment remains
unresolved; no production app-bundle installation has been introduced.

The 2026-10-01 local experiments found:

- A standalone signed binary with embedded usage explanations did not obtain
  authorization. CoreLocationAgent logged `client bundle is NULL. Skip showing
  AuthPrompt`.
- A complete embedded application plist with a separate test identifier also
  timed out. Launch Services refused to register the standalone executable.
- Removing the embedded bundle identifier also timed out with the same
  CoreLocationAgent message.
- A minimal Objective-C standalone executable calling the same bridge timed
  out too, so the failure was reproduced outside Go/cgo.
- A temporary `dgs.app` containing the executable and an external Info.plist,
  registered with Launch Services, obtained authorization and returned both
  coordinates and the complete JSON including a reverse-geocoded address.
- The original standalone executable still failed after the application was
  authorized; application authorization alone did not make it work.

The bridge now waits for an authorized status before starting location updates,
so it does not exit on an early location failure while authorization is pending.
These experiments do not establish that all possible single-file approaches
fail, nor that an app bundle should become the production design. The user's
single-file requirement remains in force.
