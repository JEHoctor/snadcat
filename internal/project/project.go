// Package project ports cli/lib/path.bash — locating the project root and its
// generated compose file, and deriving the Docker Compose project name.
package project

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Dir is the per-project Sandcat directory (SCT_PROJECT_DIR).
const Dir = ".snadcat"

// ComposeFile is the generated compose file, relative to the project root.
var ComposeFile = filepath.Join(".devcontainer", "compose-all.yml")

// FindRoot walks up from start looking for a .sandcat, .git or .devcontainer
// entry, and returns the first directory that has one.
//
// Any of the three counts: a project that has been init'd has .sandcat and
// .devcontainer, while one that hasn't still resolves to its git root, which
// is where init should write.
func FindRoot(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		for _, marker := range []string{Dir, ".git", ".devcontainer"} {
			if _, err := os.Lstat(filepath.Join(dir, marker)); err == nil {
				return dir, nil
			}
		}
		parent := filepath.Dir(dir)
		// filepath.Dir is a fixed point at the filesystem root, and on
		// Windows at the volume root — so compare rather than testing for "/".
		if parent == dir {
			return "", fmt.Errorf("repository root not found")
		}
		dir = parent
	}
}

// FindComposeFile returns the absolute path of the project's compose file.
func FindComposeFile(start string) (string, error) {
	root, err := FindRoot(start)
	if err != nil {
		return "", err
	}
	path := filepath.Join(root, ComposeFile)
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("no compose-all.yml found at %s — run `snadcat init` first", path)
	}
	return path, nil
}

// DeriveName builds the default Docker Compose project name from a project
// path: the final path element plus a "-sandbox" suffix.
func DeriveName(projectPath string) string {
	abs, err := filepath.Abs(projectPath)
	if err != nil {
		abs = projectPath
	}
	return filepath.Base(abs) + "-sandbox"
}

// VerifyRelativePath checks that path is relative and resolves to an existing
// file under base. Used to validate the settings-file path that gets written
// into the compose file as a bind-mount source.
func VerifyRelativePath(base, path string) error {
	if fi, err := os.Stat(base); err != nil || !fi.IsDir() {
		return fmt.Errorf("base is not a directory: %s", base)
	}
	if filepath.IsAbs(path) || strings.HasPrefix(path, "/") {
		return fmt.Errorf("path must be relative, not absolute: %s", path)
	}
	if _, err := os.Stat(filepath.Join(base, path)); err != nil {
		return fmt.Errorf("file not found: %s", filepath.Join(base, path))
	}
	return nil
}
