package compose

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jehoctor/snadcat/internal/agents"
	"github.com/jehoctor/snadcat/internal/stacks"
	"github.com/jehoctor/snadcat/internal/testutil"
)

// This file is the acceptance test for plans/2026-08-07-go-port.md §3.1: the Go compose
// mutation must produce byte-identical output to the bash + yq pipeline it
// replaces, comments and all.

const repoRel = "../.."

func repoPath(t *testing.T, rel string) string {
	t.Helper()
	p, err := filepath.Abs(filepath.Join(repoRel, rel))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// requireBashTooling skips when the bash originals or yq are unavailable, e.g.
// after cutover deletes cli/ or on a machine without Mike Farah's yq.
func requireBashTooling(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(repoPath(t, "cli/lib/composefile.bash")); err != nil {
		t.Skip("bash originals unavailable")
	}
	for _, bin := range []string{"bash", "yq"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s unavailable", bin)
		}
	}
}

// fixture lays out a project the way `sandcat init` would before
// customize_compose_file runs: a .devcontainer holding the compose template,
// and a .sandcat/settings.json for the settings mount to point at.
func fixture(t *testing.T) (composeDir, composeFile string) {
	t.Helper()
	root := t.TempDir()
	composeDir = filepath.Join(root, ".devcontainer")
	if err := os.MkdirAll(composeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".sandcat"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".sandcat", "settings.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tmpl, err := os.ReadFile(repoPath(t, "cli/templates/devcontainer/compose-all.yml"))
	if err != nil {
		t.Fatal(err)
	}
	composeFile = filepath.Join(composeDir, "compose-all.yml")
	if err := os.WriteFile(composeFile, tmpl, 0o644); err != nil {
		t.Fatal(err)
	}
	return composeDir, composeFile
}

// runBashCustomize invokes the original customize_compose_file + set_project_name
// on a fixture and returns the resulting file.
func runBashCustomize(t *testing.T, env []string, agent, ide, projectName, stacksList string) string {
	t.Helper()
	_, composeFile := fixture(t)

	script := `
set -euo pipefail
source "$SCT_LIBDIR/composefile.bash"
customize_compose_file "$1" "$2" "$3" "$4" "$5" "$6"
set_project_name "$2" "$5"
cat "$2"
`
	cmd := exec.Command("bash", "-c", script, "bash",
		"../.sandcat/settings.json", composeFile, agent, ide, projectName, stacksList)
	cmd.Env = append(append(os.Environ(),
		"SCT_LIBDIR="+repoPath(t, "cli/lib"),
		"SCT_TEMPLATEDIR="+repoPath(t, "cli/templates"),
	), env...)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("bash customize failed: %v\n%s", err, ee.Stderr)
		}
		t.Fatal(err)
	}
	return string(out)
}

// runGoCustomize performs the same work through this package.
func runGoCustomize(t *testing.T, env map[string]string, agent, ide, projectName string, stackList []string) string {
	t.Helper()
	composeDir, composeFile := fixture(t)

	a, ok := agents.Get(agent)
	if !ok {
		t.Fatalf("unknown agent %q", agent)
	}

	opts := DefaultOptions()
	opts.SettingsFile = "../.sandcat/settings.json"
	opts.Agent = a
	opts.IDE = ide
	opts.ProjectName = projectName
	opts.Stacks = stackList
	opts.ApplyEnvOverrides(func(k string) (string, bool) {
		// The matrix is written in the bash's SANDCAT_* names.
		v, ok := env[strings.Replace(k, "SNADCAT_", "SANDCAT_", 1)]
		return v, ok
	})

	f, err := Load(composeFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Customize(composeDir, opts); err != nil {
		t.Fatal(err)
	}
	f.SetProjectName(projectName)

	b, err := f.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	return testutil.Normalize(string(b))
}

// TestCustomizeMatchesBash sweeps the option matrix and requires byte equality
// with the bash pipeline for every combination.
func TestCustomizeMatchesBash(t *testing.T) {
	requireBashTooling(t)

	type tc struct {
		name   string
		agent  string
		ide    string
		stacks []string
		env    map[string]string
	}
	var cases []tc
	for _, agent := range agents.Available() {
		for _, ide := range []string{"vscode", "jetbrains", "none"} {
			cases = append(cases, tc{
				name:  fmt.Sprintf("%s/%s", agent, ide),
				agent: agent, ide: ide,
			})
		}
	}
	// Stack-driven shared caches, including the scala->java inheritance.
	for _, s := range [][]string{{"node"}, {"java"}, {"java", "scala"}, {"python", "go"}} {
		cases = append(cases, tc{
			name:  "stacks/" + fmt.Sprint(s),
			agent: "claude", ide: "vscode", stacks: stacks.Resolve(s),
		})
	}
	// Every mount toggle flipped away from its default — this is where the
	// commented-out-entry rendering gets exercised.
	for _, env := range []map[string]string{
		{"SANDCAT_MOUNT_CLAUDE_CONFIG": "false"},
		{"SANDCAT_MOUNT_GIT_READONLY": "true"},
		{"SANDCAT_MOUNT_IDEA_READONLY": "true"},
		{"SANDCAT_MOUNT_SHARED_CACHE": "false"},
		{"SANDCAT_MOUNT_CLAUDE_CONFIG": "false", "SANDCAT_MOUNT_SHARED_CACHE": "false"},
	} {
		cases = append(cases, tc{
			name:  "toggles/" + fmt.Sprint(env),
			agent: "claude", ide: "vscode", stacks: []string{"java"}, env: env,
		})
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var envList []string
			for k, v := range c.env {
				envList = append(envList, k+"="+v)
			}
			// The bash reads stacks as a single space-separated argument.
			stacksArg := ""
			for i, s := range c.stacks {
				if i > 0 {
					stacksArg += " "
				}
				stacksArg += s
			}

			want := runBashCustomize(t, envList, c.agent, c.ide, "demo-sandbox", stacksArg)
			got := runGoCustomize(t, c.env, c.agent, c.ide, "demo-sandbox", c.stacks)
			if got != want {
				t.Errorf("output differs from bash\n--- got ---\n%s\n--- want ---\n%s", got, want)
			}
		})
	}
}

// The proxy-file mutations have their own bash entry points.
func TestProxyMutationsMatchBash(t *testing.T) {
	requireBashTooling(t)

	proxyTemplate := repoPath(t, "cli/templates/devcontainer/sandcat/compose-proxy.yml")

	run := func(t *testing.T, bashFn string, bashArgs []string, apply func(*File)) {
		t.Helper()
		tmpl, err := os.ReadFile(proxyTemplate)
		if err != nil {
			t.Fatal(err)
		}

		bashFile := filepath.Join(t.TempDir(), "compose-proxy.yml")
		if err := os.WriteFile(bashFile, tmpl, 0o644); err != nil {
			t.Fatal(err)
		}
		script := `set -euo pipefail; source "$SCT_LIBDIR/composefile.bash"; ` + bashFn + ` "$@"; cat "$1"`
		cmd := exec.Command("bash", append([]string{"-c", script, "bash", bashFile}, bashArgs...)...)
		cmd.Env = append(os.Environ(), "SCT_LIBDIR="+repoPath(t, "cli/lib"))
		want, err := cmd.Output()
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				t.Fatalf("%s failed: %v\n%s", bashFn, err, ee.Stderr)
			}
			t.Fatal(err)
		}

		goFile := filepath.Join(t.TempDir(), "compose-proxy.yml")
		if err := os.WriteFile(goFile, tmpl, 0o644); err != nil {
			t.Fatal(err)
		}
		f, err := Load(goFile)
		if err != nil {
			t.Fatal(err)
		}
		apply(f)
		got, err := f.Bytes()
		if err != nil {
			t.Fatal(err)
		}
		if g := testutil.Normalize(string(got)); g != string(want) {
			t.Errorf("output differs from bash\n--- got ---\n%s\n--- want ---\n%s", g, want)
		}
	}

	t.Run("tui mode", func(t *testing.T) {
		run(t, "set_proxy_tui_mode", nil, func(f *File) { f.SetProxyTUIMode() })
	})
	for _, p := range []SecretProvider{SecretProvider1Password, SecretProviderProtonPass} {
		t.Run("secret provider "+string(p), func(t *testing.T) {
			run(t, "apply_secret_provider", []string{string(p)}, func(f *File) {
				if err := f.ApplySecretProvider(p); err != nil {
					t.Fatal(err)
				}
			})
		})
	}
}

// upstream_ca_bundles is off in every harness case (nobody has a bundle
// configured under a throwaway HOME), so it gets its own parity check with a
// fixture bundle on disk.
func TestUpstreamCABundlesMatchBash(t *testing.T) {
	requireBashTooling(t)

	fixtures := t.TempDir()
	pem := "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n"
	bundle1 := filepath.Join(fixtures, "corp-root.pem")
	bundle2 := filepath.Join(fixtures, "proxy ca.crt") // space and .crt: exercises naming
	for _, p := range []string{bundle1, bundle2} {
		if err := os.WriteFile(p, []byte(pem), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// user settings carries one bundle, project settings.local.json the other.
	home := t.TempDir()
	userSettings := filepath.Join(home, ".config", "sandcat", "settings.json")
	if err := os.MkdirAll(filepath.Dir(userSettings), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(userSettings, []byte(`{"upstream_ca_bundles":["`+bundle1+`"]}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Each tool reads its own project dir; give both the same local settings.
	project := t.TempDir()
	for _, dir := range []string{".sandcat", ".snadcat"} {
		if err := os.MkdirAll(filepath.Join(project, dir), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(project, dir, "settings.local.json"),
			[]byte(`{"upstream_ca_bundles":["`+bundle2+`"]}`+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	tmpl, err := os.ReadFile(repoPath(t, "cli/templates/devcontainer/sandcat/compose-proxy.yml"))
	if err != nil {
		t.Fatal(err)
	}

	bashFile := filepath.Join(t.TempDir(), "compose-proxy.yml")
	if err := os.WriteFile(bashFile, tmpl, 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", "-c",
		`set -euo pipefail; source "$SCT_LIBDIR/composefile.bash"; apply_upstream_ca_bundles "$1" "$2"; cat "$1"`,
		"bash", bashFile, project)
	cmd.Env = append(os.Environ(), "HOME="+home, "SCT_LIBDIR="+repoPath(t, "cli/lib"))
	want, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("bash failed: %v\n%s", err, ee.Stderr)
		}
		t.Fatal(err)
	}

	goFile := filepath.Join(t.TempDir(), "compose-proxy.yml")
	if err := os.WriteFile(goFile, tmpl, 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := Load(goFile)
	if err != nil {
		t.Fatal(err)
	}
	bundles := ReadUpstreamCABundles(userSettings, project)
	if len(bundles) != 2 {
		t.Fatalf("expected 2 bundles, got %v", bundles)
	}
	if err := f.ApplyUpstreamCABundles(bundles); err != nil {
		t.Fatal(err)
	}
	got, err := f.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if g := testutil.Normalize(string(got)); g != string(want) {
		t.Errorf("output differs from bash\n--- got ---\n%s\n--- want ---\n%s", g, want)
	}
}

func TestUpstreamCABundlesRejectsBadPaths(t *testing.T) {
	f, err := Load(repoPath(t, "cli/templates/devcontainer/sandcat/compose-proxy.yml"))
	if err != nil {
		t.Fatal(err)
	}
	before, _ := f.Bytes()

	notPEM := filepath.Join(t.TempDir(), "x.pem")
	if err := os.WriteFile(notPEM, []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"relative.pem", filepath.Join(t.TempDir(), "missing.pem"), notPEM} {
		if err := f.ApplyUpstreamCABundles([]string{p}); err == nil {
			t.Errorf("%q should be rejected", p)
		}
	}
	// The document must be untouched after a rejected bundle.
	if after, _ := f.Bytes(); string(after) != string(before) {
		t.Error("compose file changed despite validation failure")
	}
}
