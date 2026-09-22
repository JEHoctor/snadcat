// Package rtk ports cli/lib/rtk.bash — install and init fragments for rtk
// (Rust Token Killer), which compresses shell output so agents spend fewer
// tokens per command.
//
// Default-on: the feature is enabled unless explicitly switched off, matching
// SANDCAT_RTK's "anything but the literal string false" test.
package rtk

import (
	"embed"
	"io/fs"
	"strings"
)

//go:embed blocks
var blocksFS embed.FS

// block reads an embedded fragment, returning "" when absent. Trailing
// newlines are stripped because every consumer in the bash original captures
// these through $(...), which does the same — keeping that here means callers
// can concatenate fragments without worrying about blank-line drift.
func block(name string) string {
	b, err := fs.ReadFile(blocksFS, "blocks/"+name)
	if err != nil {
		return ""
	}
	return strings.TrimRight(string(b), "\n")
}

// DockerInstallBlock returns the Dockerfile.app RUN block that installs the
// rtk binary system-wide. Agent-agnostic — the same binary serves every agent,
// and per-agent hook wiring happens at container start.
//
// Returns "" when disabled, so callers can append unconditionally.
func DockerInstallBlock(enabled bool) string {
	if !enabled {
		return ""
	}
	return block("docker-install.txt")
}

// UserInitBlock returns the app-user-init.sh fragment that runs `rtk init` for
// the given agent, guarded to a one-time execution.
//
// Returns "" when disabled, and for agents with no rtk profile. codex is
// deliberately in the latter group: rtk 0.44 has no `--agent codex`, and
// `--codex` cannot be combined with `--hook-only` or `--auto-patch`, so codex
// wires rtk from its own user-init block instead. The binary is still on PATH
// either way.
func UserInitBlock(agent string, enabled bool) string {
	if !enabled {
		return ""
	}
	return block("user-init-" + agent + ".txt")
}
