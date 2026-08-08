package sandcat

import (
	"io/fs"
	"testing"
)

// The generated .devcontainer tree is copied wholesale from these files, so a
// missing embed pattern would surface as a silently incomplete sandbox rather
// than a build error. Assert the full expected set.
func TestTemplatesAreEmbedded(t *testing.T) {
	want := []string{
		"settings.json",
		"settings-user-claude.json",
		"settings-user-codex.json",
		"settings-user-cursor.json",
		"devcontainer/compose-all.yml",
		"devcontainer/devcontainer.json",
		"devcontainer/Dockerfile.app",
		"devcontainer/sandcat/compose-proxy.yml",
		"devcontainer/sandcat/Dockerfile.wg-client",
		"devcontainer/sandcat/tmux.conf",
		"devcontainer/sandcat/scripts/app-init.sh",
		"devcontainer/sandcat/scripts/app-post-start.sh",
		"devcontainer/sandcat/scripts/app-user-init.sh",
		"devcontainer/sandcat/scripts/dnsmasq-ready",
		"devcontainer/sandcat/scripts/wg-client-init.sh",
		"devcontainer/sandcat/scripts/mitmproxy_addon_common.py",
		"devcontainer/sandcat/scripts/mitmproxy_addon_claude.py",
		"devcontainer/sandcat/scripts/mitmproxy_addon_codex.py",
		"devcontainer/sandcat/scripts/mitmproxy_addon_cursor.py",
	}
	for _, name := range want {
		b, err := fs.ReadFile(Templates, name)
		if err != nil {
			t.Errorf("ReadFile(%q): %v", name, err)
			continue
		}
		if len(b) == 0 {
			t.Errorf("%q is empty", name)
		}
	}
}

// fs.Sub re-roots the tree; callers must not have to spell cli/templates.
func TestTemplatesAreReRooted(t *testing.T) {
	if _, err := fs.Stat(Templates, "cli/templates"); err == nil {
		t.Error("Templates still exposes the cli/templates prefix")
	}
}
