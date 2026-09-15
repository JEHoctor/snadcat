// Package gitignore ports cli/lib/gitignore.bash.
//
// `sandcat init` appends a managed block to the project's .gitignore so users
// don't accidentally commit .devcontainer/ (regenerated on every init) or
// .sandcat/settings.local.json (per-machine secrets), while re-including
// devbox.tools.json via a negation pattern so the team-shared tool list still
// travels with the repo.
//
// The block is bracketed by marker lines so `--features no-gitignore` can
// remove it again — the toggle converges to the declared state rather than
// only ever adding.
package gitignore

import (
	_ "embed"
	"os"
	"path/filepath"
	"strings"
)

//go:embed block.txt
var block string

// Markers bracketing the managed block. Both are matched as whole lines so a
// user rule that merely contains the text can't be mistaken for a marker.
const (
	StartMarker = "# Sandcat"
	EndMarker   = "# /Sandcat"
)

// gitignorePath returns the .gitignore path, and whether projectPath is a git
// working tree at all. A .git entry may be a directory or, in a worktree or
// submodule, a file — so presence is what matters, not type.
func gitignorePath(projectPath string) (string, bool) {
	if _, err := os.Lstat(filepath.Join(projectPath, ".git")); err != nil {
		return "", false
	}
	return filepath.Join(projectPath, ".gitignore"), true
}

// HasBlock reports whether the managed block is already present.
func HasBlock(projectPath string) bool {
	path, ok := gitignorePath(projectPath)
	if !ok {
		return false
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return containsLine(splitLines(string(b)), StartMarker)
}

// Update appends the managed block, creating .gitignore if needed.
//
// No-op when projectPath is not a git working tree, or when the block is
// already there — a re-run must keep the user's edits rather than stacking
// duplicate blocks.
func Update(projectPath string) error {
	path, ok := gitignorePath(projectPath)
	if !ok {
		return nil
	}

	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	lines := splitLines(string(existing))
	if containsLine(lines, StartMarker) {
		return nil
	}

	var sb strings.Builder
	if len(existing) > 0 {
		sb.Write(existing)
		// Don't glue our block onto an unterminated last line, and leave a
		// blank line between the user's rules and ours.
		if !strings.HasSuffix(string(existing), "\n") {
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}
	sb.WriteString(block)

	return os.WriteFile(path, []byte(sb.String()), 0o644)
}

// RemoveBlock strips the managed block, including both markers and the blank
// separator Update inserted before it.
//
// Anything the user hand-added *between* the markers goes too — editing inside
// the block isn't supported. Rules outside the markers are preserved verbatim.
// When the block was the file's only content, the file is removed, mirroring
// the pre-init state.
func RemoveBlock(projectPath string) error {
	path, ok := gitignorePath(projectPath)
	if !ok {
		return nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	lines := splitLines(string(b))
	if !containsLine(lines, StartMarker) {
		return nil
	}

	var out []string
	for i := 0; i < len(lines); i++ {
		if lines[i] != StartMarker {
			out = append(out, lines[i])
			continue
		}
		// Retroactively drop the separator we added, so repeated
		// enable/disable cycles don't accumulate blank lines.
		if n := len(out); n > 0 && out[n-1] == "" {
			out = out[:n-1]
		}
		for i < len(lines) && lines[i] != EndMarker {
			i++
		}
	}

	if len(out) == 0 {
		return os.Remove(path)
	}
	return os.WriteFile(path, []byte(strings.Join(out, "\n")+"\n"), 0o644)
}

// splitLines splits file content into lines, dropping the artifact empty
// element a trailing newline produces.
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

func containsLine(lines []string, want string) bool {
	for _, l := range lines {
		if l == want {
			return true
		}
	}
	return false
}
