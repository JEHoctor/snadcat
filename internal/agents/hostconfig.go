package agents

import (
	"os"
	"path/filepath"
	"time"
)

// MountEnabled reports whether the agent's optional host-config mounts are on.
// Missing or unset means enabled — customize_compose_file defaults these to
// "true", and ensure_host_agent_config_paths deliberately matches that default
// so the pre-created paths and the generated mounts never disagree.
func (a Agent) MountEnabled() bool {
	if a.MountEnvVar == "" {
		return false
	}
	v, ok := os.LookupEnv(a.MountEnvVar)
	return !ok || v == "true"
}

// defaultContent returns the seed bytes for a freshly created config file.
//
// Empty files are fine for Markdown, but a zero-length JSON file breaks
// Cursor CLI's parser and sandcat's own jq bootstrap — so those get minimal
// valid documents. The point of writing anything at all is to make Docker bind
// a *file* rather than inventing a directory at the mount target.
func defaultContent(agent, filename string) []byte {
	if agent != "cursor" {
		return nil
	}
	switch filename {
	case "cli-config.json":
		return []byte("{\"version\":1}\n")
	case "hooks.json":
		return []byte("{\"hooks\":{}}\n")
	case "mcp.json":
		return []byte("{\"mcpServers\":{}}\n")
	default:
		return nil
	}
}

// EnsureHostConfigPaths pre-creates the host paths this agent's optional bind
// mounts will target, under home.
//
// No-op when the mounts are disabled. Existing non-empty files are never
// rewritten — they only get an access-time touch, matching the bash original,
// so a user's real config is never clobbered.
func (a Agent) EnsureHostConfigPaths(home, projectName string) error {
	if !a.MountEnabled() {
		return nil
	}
	for _, p := range a.HostConfigPaths(projectName) {
		full := filepath.Join(home, filepath.FromSlash(p.Path))
		if p.IsDir {
			if err := os.MkdirAll(full, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		if err := ensureFile(full, defaultContent(a.Name, filepath.Base(full))); err != nil {
			return err
		}
	}
	return nil
}

// ensureFile creates path with content when it is missing or zero-length,
// and otherwise leaves it alone.
func ensureFile(path string, content []byte) error {
	if fi, err := os.Stat(path); err == nil && fi.Size() > 0 {
		now := time.Now()
		// Best-effort atime bump, as `touch -a` does. A failure here says
		// nothing about whether the file is usable, so it is not fatal.
		_ = os.Chtimes(path, now, fi.ModTime())
		return nil
	}
	return os.WriteFile(path, content, 0o644)
}
