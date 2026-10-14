# Releases

The `Release` GitHub Actions workflow publishes `dgs` when a `v*.*.*` tag
is pushed. It can also be run manually with an existing version tag. Commit
the workflow and source changes before tagging; the workflow builds that tag,
not the current default branch.

```sh
git tag v0.1.0
git push origin v0.1.0
```

Release assets cover Linux amd64/arm64, Windows amd64, and macOS Intel/Apple
Silicon. Each platform has a standalone `dgs_<version>_<os>_<arch>` binary
(`.exe` on Windows), plus a `.tar.gz` or `.zip` containing only `dgs`.
`checksums.txt` contains SHA-256 hashes of both formats.

Download the asset matching your system from the repository's Releases page.
For a standalone Unix binary, rename it to `dgs`, run `chmod +x dgs`, and
run `./dgs --version`. On Windows, rename it to `dgs.exe` and run
`.\dgs.exe --version`. Move it onto your `PATH` if desired. No Go installation,
helper executable, or separate runtime assets are required; bundled plugins
are compiled and embedded before each binary is built.

The workflow tests Go packages, GPX browser behavior, and native/WebAssembly/
bundled timeline parity before publishing. Release builds use the dependency
versions in `go.mod` (`GOWORK=off`); development continues to use the local
`go.work`. Update the pinned dependency before tagging if a feature needs a
newer dependency version.

macOS builds retain native integrations and the embedded Reminders Info.plist.
They are ad-hoc signed, not Developer ID signed or notarized, so macOS may
require explicit approval to run a downloaded binary. Linux and Windows builds
disable cgo; platform-only integrations report that they are unsupported.
