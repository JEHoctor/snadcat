package devcontainer

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/jehoctor/snadcat/internal/agents"
	"github.com/jehoctor/snadcat/internal/compose"
	"github.com/jehoctor/snadcat/internal/log"
	"github.com/jehoctor/snadcat/internal/stacks"
)

// Whole-tree parity with cli/libexec/init/devcontainer: every generated file
// must be byte-identical across the option matrix. This is the differential
// harness from plans/2026-08-07-go-port.md §5, scoped to the devcontainer step.

func repoPath(t *testing.T, rel string) string {
	t.Helper()
	p, err := filepath.Abs(filepath.Join("../..", rel))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func requireBashTooling(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(repoPath(t, "cli/libexec/init/devcontainer")); err != nil {
		t.Skip("bash originals unavailable")
	}
	for _, bin := range []string{"bash", "yq", "jq"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s unavailable", bin)
		}
	}
}

// newProject makes a project dir with the settings file init would have
// written before the devcontainer step.
func newProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".sandcat"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".sandcat", "settings.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

type matrixCase struct {
	name     string
	agent    string
	ide      string
	stacks   []string
	proxy    string
	provider string
	rtk      bool
	env      map[string]string
}

func (c matrixCase) envList() []string {
	out := []string{"SANDCAT_RTK=" + fmt.Sprint(c.rtk)}
	for k, v := range c.env {
		out = append(out, k+"="+v)
	}
	return out
}

func runBash(t *testing.T, c matrixCase) string {
	t.Helper()
	root := newProject(t)
	cmd := exec.Command(repoPath(t, "cli/libexec/init/devcontainer"),
		"--settings-file", ".sandcat/settings.json",
		"--project-path", root,
		"--agent", c.agent,
		"--ide", c.ide,
		"--name", "demo-sandbox",
		"--stacks", strings.Join(c.stacks, " "),
		"--proxy", c.proxy,
		"--secret-provider", c.provider,
	)
	cmd.Env = append(append(os.Environ(),
		"SCT_LIBDIR="+repoPath(t, "cli/lib"),
		"SCT_TEMPLATEDIR="+repoPath(t, "cli/templates"),
	), c.envList()...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("bash devcontainer failed: %v\n%s", err, out)
	}
	return root
}

func runGo(t *testing.T, c matrixCase) string {
	t.Helper()
	root := newProject(t)
	a, ok := agents.Get(c.agent)
	if !ok {
		t.Fatalf("unknown agent %q", c.agent)
	}
	mounts := compose.DefaultOptions()
	mounts.Agent = a
	mounts.ApplyEnvOverrides(func(k string) (string, bool) {
		v, ok := c.env[k]
		return v, ok
	})

	old := log.Out
	log.Out = &bytes.Buffer{}
	t.Cleanup(func() { log.Out = old })

	err := Generate(Options{
		ProjectPath:    root,
		ProjectName:    "demo-sandbox",
		SettingsFile:   ".sandcat/settings.json",
		Agent:          a,
		IDE:            c.ide,
		Stacks:         c.stacks,
		ProxyTUI:       c.proxy == "tui",
		SecretProvider: compose.SecretProvider(c.provider),
		RTKEnabled:     c.rtk,
		Mounts:         mounts,
	})
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// snapshot reads every file under dir into a path→content map.
func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		out[filepath.ToSlash(rel)] = string(b)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func diffTrees(t *testing.T, want, got map[string]string) {
	t.Helper()
	var names []string
	seen := map[string]bool{}
	for n := range want {
		names = append(names, n)
		seen[n] = true
	}
	for n := range got {
		if !seen[n] {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	for _, n := range names {
		w, inWant := want[n]
		g, inGot := got[n]
		switch {
		case !inWant:
			t.Errorf("%s: only in Go output", n)
		case !inGot:
			t.Errorf("%s: only in bash output", n)
		case w != g:
			t.Errorf("%s differs\n--- bash ---\n%s\n--- go ---\n%s", n, w, g)
		}
	}
}

func TestGenerateMatchesBash(t *testing.T) {
	requireBashTooling(t)

	base := matrixCase{agent: "claude", ide: "vscode", proxy: "web", provider: "none", rtk: true}
	var cases []matrixCase
	add := func(name string, mutate func(*matrixCase)) {
		c := base
		c.name = name
		mutate(&c)
		cases = append(cases, c)
	}

	for _, agent := range agents.Available() {
		for _, ide := range []string{"vscode", "jetbrains", "none"} {
			add(agent+"/"+ide, func(c *matrixCase) { c.agent, c.ide = agent, ide })
		}
	}
	for _, s := range [][]string{{"node"}, {"java"}, {"scala"}, {"python", "go", "rust"}} {
		add("stacks/"+strings.Join(s, "+"), func(c *matrixCase) { c.stacks = stacks.Resolve(s) })
	}
	add("proxy/tui", func(c *matrixCase) { c.proxy = "tui" })
	add("provider/1password", func(c *matrixCase) { c.provider = "1password" })
	add("provider/protonpass", func(c *matrixCase) { c.provider = "protonpass" })
	add("rtk/off", func(c *matrixCase) { c.rtk = false })
	add("rtk/off/codex", func(c *matrixCase) { c.rtk = false; c.agent = "codex" })
	add("toggles/no-claude-config", func(c *matrixCase) { c.env = map[string]string{"SANDCAT_MOUNT_CLAUDE_CONFIG": "false"} })
	add("toggles/no-copilot-config", func(c *matrixCase) {
		c.agent = "copilot"
		c.env = map[string]string{"SANDCAT_MOUNT_COPILOT_CONFIG": "false"}
	})
	add("toggles/git-ro+no-cache", func(c *matrixCase) {
		c.stacks = []string{"java"}
		c.env = map[string]string{"SANDCAT_MOUNT_GIT_READONLY": "true", "SANDCAT_MOUNT_SHARED_CACHE": "false"}
	})
	add("kitchen-sink", func(c *matrixCase) {
		c.agent, c.ide, c.proxy, c.provider = "cursor", "jetbrains", "tui", "protonpass"
		c.stacks = stacks.Resolve([]string{"scala", "node"})
	})

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			want := snapshot(t, filepath.Join(runBash(t, c), ".devcontainer"))
			got := snapshot(t, filepath.Join(runGo(t, c), ".devcontainer"))
			diffTrees(t, want, got)
		})
	}
}

// devbox.tools.json is user-managed and must survive a re-run untouched,
// while devbox.stack.json is regenerated.
func TestGenerateSecondRunPreservesToolsJSON(t *testing.T) {
	c := matrixCase{agent: "claude", ide: "vscode", proxy: "web", provider: "none", rtk: true}
	root := runGo(t, c)

	tools := filepath.Join(root, ".devcontainer", "devbox.tools.json")
	const custom = `{"packages":["cowsay@latest"]}` + "\n"
	if err := os.WriteFile(tools, []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}

	a, _ := agents.Get("claude")
	mounts := compose.DefaultOptions()
	mounts.Agent = a
	old := log.Out
	log.Out = &bytes.Buffer{}
	t.Cleanup(func() { log.Out = old })
	if err := Generate(Options{
		ProjectPath: root, ProjectName: "demo-sandbox", SettingsFile: ".sandcat/settings.json",
		Agent: a, IDE: "vscode", Stacks: []string{"go"}, RTKEnabled: true, Mounts: mounts,
	}); err != nil {
		t.Fatal(err)
	}

	b, err := os.ReadFile(tools)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != custom {
		t.Errorf("devbox.tools.json was overwritten: %q", b)
	}
	stack, err := os.ReadFile(filepath.Join(root, ".devcontainer", "devbox.stack.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(stack), "go@latest") {
		t.Errorf("devbox.stack.json was not regenerated: %s", stack)
	}
}
