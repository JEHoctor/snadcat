// Package devbox ports cli/lib/devbox.bash — Nix packages baked into the
// agent image at build time.
//
// Two-file model:
//
//   - devbox.stack.json is sandcat-managed: regenerated on every `sandcat init`
//     from the --stacks selection plus a baseline of shell tools every sandbox
//     needs.
//   - devbox.tools.json is user-managed: written once with an empty package
//     list and never touched again, so the user's additions survive
//     re-initialization.
//
// The two are merged at Docker build time by the jq program embedded in the
// Dockerfile block; same-name entries in tools override the stack default.
package devbox

import (
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"strings"

	"github.com/jehoctor/snadcat/internal/stacks"
)

//go:embed blocks
var blocksFS embed.FS

// BaselinePackages are installed in every sandbox regardless of --stacks.
var BaselinePackages = []string{
	"fd@latest",      // fast file finder
	"fzf@latest",     // fuzzy finder for files and command history
	"gh@latest",      // GitHub CLI
	"jq@latest",      // JSON processor
	"ripgrep@latest", // fast recursive grep (rg)
	"tmux@latest",    // terminal multiplexer
	"vim@latest",     // text editor
}

// StackJSON renders devbox.stack.json for the given resolved stacks: the
// baseline followed by each stack's packages, in order.
//
// Formatting matches `jq .` (2-space indent, trailing newline) so the file is
// byte-identical to what the bash wrote.
func StackJSON(resolvedStacks []string) ([]byte, error) {
	pkgs := append(append([]string{}, BaselinePackages...), stacks.DevboxPackages(resolvedStacks)...)
	b, err := json.MarshalIndent(struct {
		Packages []string `json:"packages"`
	}{pkgs}, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// WriteStackJSON writes devbox.stack.json unconditionally — it is regenerated
// on every init.
func WriteStackJSON(path string, resolvedStacks []string) error {
	b, err := StackJSON(resolvedStacks)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// WriteToolsJSON writes the empty user-managed devbox.tools.json only when it
// does not already exist.
func WriteToolsJSON(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	b, err := fs.ReadFile(blocksFS, "blocks/tools.json")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// DockerfileBlock is the Dockerfile.app fragment that installs devbox and
// Nix, merges the two configs, and installs the packages in two layers so a
// tools-only edit doesn't invalidate the expensive stack layer.
//
// Trailing newlines are stripped to match the $(...) capture in the bash.
func DockerfileBlock() string {
	b, err := fs.ReadFile(blocksFS, "blocks/dockerfile.txt")
	if err != nil {
		// The file is embedded at build time; a missing block is a packaging
		// bug rather than a runtime condition.
		panic(err)
	}
	return strings.TrimRight(string(b), "\n")
}
