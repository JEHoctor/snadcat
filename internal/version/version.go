// Package version ports cli/libexec/version/version.
//
// The bash version derives a string from the checkout it runs out of: a
// .version file if present, else `git log` date + `git describe`. A shipped
// binary has no checkout, so the release path stamps Version at build time:
//
//	go build -ldflags "-X github.com/jehoctor/snadcat/internal/version.Version=v1.2.3"
//
// For unstamped developer builds we read the VCS stamps the Go toolchain
// already embeds (vcs.revision / vcs.time / vcs.modified), which reproduces the
// bash format without shelling out to git and works for `go build` and
// `go run` alike.
package version

import (
	"runtime/debug"
	"strings"
	"time"
)

// Version is injected at build time. Empty means fall back to build info.
var Version string

// Name is the product label printed alongside the version.
const Name = "Sandcat"

// String returns the version string, or "" when it cannot be determined.
func String() string {
	if Version != "" {
		return Version
	}
	return fromBuildInfo()
}

// fromBuildInfo renders "<YYYYMMDD.HHMMSS>-<sha7>[-dirty]", matching the shape
// the bash implementation produces from `git log` + `git describe --dirty`.
func fromBuildInfo() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	var rev, stamp string
	var dirty bool
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.time":
			stamp = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	return render(rev, stamp, dirty)
}

// render builds the version string from raw VCS stamps. Split out from
// fromBuildInfo so it is testable without a particular build environment.
func render(rev, stamp string, dirty bool) string {
	if rev == "" {
		return ""
	}
	if len(rev) > 7 {
		rev = rev[:7]
	}

	out := rev
	if t, err := time.Parse(time.RFC3339, stamp); err == nil {
		out = t.Format("20060102.150405") + "-" + rev
	}
	if dirty {
		out += "-dirty"
	}
	return strings.TrimSpace(out)
}
