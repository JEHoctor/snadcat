// Package editor ports open_editor from cli/lib/select.bash.
package editor

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// Command resolves the editor: VISUAL, then EDITOR, then a platform fallback
// (`open` on macOS if present, `notepad` on Windows, `vi` elsewhere). The
// value is split on whitespace so "code --wait" works.
func Command() ([]string, error) {
	spec := os.Getenv("VISUAL")
	if spec == "" {
		spec = os.Getenv("EDITOR")
	}
	if spec == "" {
		spec = fallback()
	}
	argv := strings.Fields(spec)
	if len(argv) == 0 {
		return nil, fmt.Errorf("no editor configured; set EDITOR or VISUAL")
	}
	if _, err := exec.LookPath(argv[0]); err != nil {
		return nil, fmt.Errorf("editor '%s' not found. Set EDITOR or VISUAL environment variable.", spec)
	}
	return argv, nil
}

func fallback() string {
	switch runtime.GOOS {
	case "windows":
		return "notepad"
	default:
		if _, err := exec.LookPath("open"); err == nil {
			return "open"
		}
		return "vi"
	}
}

// Open runs the editor on path and waits for it to exit. macOS `open` returns
// immediately unless asked to wait, so it gets the flags the bash passed.
func Open(path string) error {
	argv, err := Command()
	if err != nil {
		return err
	}
	if strings.HasSuffix(argv[0], "open") && len(argv) == 1 {
		argv = append(argv, "--new", "--wait-apps")
	}
	cmd := exec.Command(argv[0], append(argv[1:], path)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}
