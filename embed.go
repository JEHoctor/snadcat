// Package sandcat holds the embedded template tree.
//
// The embed directive lives at the module root because //go:embed paths are
// relative to the embedding file's directory and cannot contain "..", so no
// package under internal/ can reach cli/templates/. Keeping the templates in
// place during the port means the bash CLI and the Go CLI read the same bytes,
// which is what makes the differential harness a real parity oracle. At cutover
// this moves to internal/templates/assets/. See GO-PORT-PLAN.md §2.
package sandcat

import (
	"embed"
	"io/fs"
)

//go:embed all:cli/templates
var templatesFS embed.FS

// Templates is the template tree re-rooted at cli/templates, so callers refer
// to "devcontainer/compose-all.yml" rather than the full embedded path.
var Templates fs.FS

func init() {
	sub, err := fs.Sub(templatesFS, "cli/templates")
	if err != nil {
		// Only reachable if the embed pattern and this path disagree, which is
		// a build-time mistake rather than a runtime condition.
		panic(err)
	}
	Templates = sub
}
