package devbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Matches `jq .` formatting: 2-space indent, one element per line, trailing
// newline. This file is regenerated on every init and diffed by users.
func TestStackJSONFormat(t *testing.T) {
	got, err := StackJSON([]string{"go"})
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"packages\": [\n" +
		"    \"fd@latest\",\n    \"fzf@latest\",\n    \"gh@latest\",\n    \"jq@latest\",\n" +
		"    \"ripgrep@latest\",\n    \"tmux@latest\",\n    \"vim@latest\",\n    \"go@latest\"\n" +
		"  ]\n}\n"
	if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestStackJSONBaselineOnly(t *testing.T) {
	got, err := StackJSON(nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(got), "@latest") != len(BaselinePackages) {
		t.Errorf("expected only the baseline: %s", got)
	}
}

func TestWriteToolsJSONOnlyWhenMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "devbox.tools.json")
	if err := WriteToolsJSON(path); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"packages": []`) {
		t.Errorf("unexpected seed content: %s", b)
	}

	const custom = "{\"packages\":[\"x\"]}\n"
	if err := os.WriteFile(path, []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteToolsJSON(path); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != custom {
		t.Errorf("existing file was overwritten: %q", b)
	}
}

func TestDockerfileBlock(t *testing.T) {
	got := DockerfileBlock()
	if strings.HasSuffix(got, "\n") {
		t.Error("block should not end with a newline")
	}
	// The jq merge program must be on a single line: Docker parses each
	// backslash-continued line as part of one RUN, and a multi-line jq
	// program would confuse it.
	for _, line := range strings.Split(got, "\n") {
		if strings.Contains(line, "def pkgname") && !strings.Contains(line, "unique)}") {
			t.Errorf("jq merge program is split across lines: %q", line)
		}
	}
}
