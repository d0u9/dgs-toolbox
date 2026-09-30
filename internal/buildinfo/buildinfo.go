// Package buildinfo exposes release metadata injected by the build.
package buildinfo

import (
	"fmt"
	"runtime/debug"
)

var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)

func init() {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	fill(info.Settings)
}

// fill takes the commit and its time from what `go build` stamps into a
// binary built inside a git checkout, unless -ldflags already set them.
// A checkout with uncommitted changes marks the commit "-dirty".
func fill(settings []debug.BuildSetting) {
	var revision, when string
	dirty := false
	for _, s := range settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.time":
			when = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if Commit == "unknown" && revision != "" {
		if len(revision) > 12 {
			revision = revision[:12]
		}
		if dirty {
			revision += "-dirty"
		}
		Commit = revision
	}
	if Date == "unknown" && when != "" {
		Date = when
	}
}

// String returns a human-readable version for the CLI.
func String() string {
	if Commit == "unknown" {
		return Version
	}
	return fmt.Sprintf("%s (commit %s, %s)", Version, Commit, Date)
}
