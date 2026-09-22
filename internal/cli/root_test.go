package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/jehoctor/snadcat/internal/dockercli"
	"github.com/jehoctor/snadcat/internal/log"
)

// find walks the tree to the command reached by the given path.
func find(t *testing.T, path ...string) *cobra.Command {
	t.Helper()
	cmd, _, err := NewRootCmd().Find(path)
	if err != nil {
		t.Fatalf("Find(%v): %v", path, err)
	}
	if cmd.Name() != path[len(path)-1] {
		t.Fatalf("Find(%v) resolved to %q", path, cmd.Name())
	}
	return cmd
}

// The bash dispatcher prints the version banner before every module except
// `version`. rootCommandName is what encodes that, so it has to report the
// top-level module even for nested subcommands.
func TestRootCommandNameReportsModule(t *testing.T) {
	tests := []struct {
		path []string
		want string
	}{
		{[]string{"version"}, "version"},
		{[]string{"init"}, "init"},
		{[]string{"init", "settings"}, "init"},
		{[]string{"cache", "rm"}, "cache"},
		{[]string{"edit", "user-settings"}, "edit"},
	}
	for _, tc := range tests {
		t.Run(strings.Join(tc.path, " "), func(t *testing.T) {
			if got := rootCommandName(find(t, tc.path...)); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// `sandcat compose up --build` must hand --build to docker, not have cobra
// reject it as an unknown sandcat flag. Same for attach and run.
func TestPassthroughCommandsDoNotParseFlags(t *testing.T) {
	for _, name := range []string{"compose", "attach", "run"} {
		t.Run(name, func(t *testing.T) {
			if !find(t, name).DisableFlagParsing {
				t.Errorf("%s must set DisableFlagParsing", name)
			}
		})
	}
}

func TestPassthroughForwardsUnknownFlags(t *testing.T) {
	rec := useFakeDocker(t)
	root := NewRootCmd()
	root.SetArgs([]string{"compose", "up", "--build", "--wait"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	// An "unknown flag" error would mean cobra consumed them; the recorded
	// argv shows they reached docker intact, after the compose file.
	want := "docker compose -f " + rec.composeFile + " up --build --wait"
	if len(rec.calls) != 1 || rec.calls[0] != want {
		t.Errorf("got %v, want [%s]", rec.calls, want)
	}
}

// `sandcat run --build -- cmd args` splits at `--`: flags before go to
// `compose run`, the rest is the command; and `down` always follows.
func TestRunSplitsFlagsFromCommand(t *testing.T) {
	rec := useFakeDocker(t)
	root := NewRootCmd()
	root.SetArgs([]string{"run", "--build", "--", "make", "test"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	prefix := "docker compose -f " + rec.composeFile + " "
	want := []string{
		prefix + "run --rm --build agent make test",
		prefix + "down",
	}
	if strings.Join(rec.calls, "\n") != strings.Join(want, "\n") {
		t.Errorf("got %v, want %v", rec.calls, want)
	}
}

func TestRunDefaultsToBash(t *testing.T) {
	rec := useFakeDocker(t)
	root := NewRootCmd()
	root.SetArgs([]string{"run"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if want := "docker compose -f " + rec.composeFile + " run --rm agent bash"; rec.calls[0] != want {
		t.Errorf("got %q, want %q", rec.calls[0], want)
	}
}

func TestAttachDefaultsToLoginShell(t *testing.T) {
	rec := useFakeDocker(t)
	root := NewRootCmd()
	root.SetArgs([]string{"attach"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if want := "docker compose -f " + rec.composeFile + " exec -u vscode agent bash --login"; rec.calls[0] != want {
		t.Errorf("got %q, want %q", rec.calls[0], want)
	}
}

// recorder captures docker invocations instead of running them.
type recorder struct {
	calls       []string
	composeFile string
}

func (r *recorder) Run(name string, args ...string) error {
	r.calls = append(r.calls, name+" "+strings.Join(args, " "))
	return nil
}

func (r *recorder) OutputQuiet(name string, args ...string) (string, error) {
	return r.Output(name, args...)
}

func (r *recorder) Output(name string, args ...string) (string, error) {
	// Advisory lookups (volume/image inspect) report nothing, so the
	// stale-home warning and cache creation stay quiet.
	return "", errors.New("not found")
}

// useFakeDocker chdirs into a temp project with a compose file, swaps the
// docker runner for a recorder, and puts a fake `docker` on PATH so Require
// passes.
func useFakeDocker(t *testing.T) *recorder {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".devcontainer"), 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, ".devcontainer", "compose-all.yml")
	if err := os.WriteFile(file, []byte("name: demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)

	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)

	rec := &recorder{composeFile: file}
	old := dockercli.Default
	dockercli.Default = rec
	t.Cleanup(func() { dockercli.Default = old })

	oldOut := log.Out
	log.Out = &bytes.Buffer{}
	t.Cleanup(func() { log.Out = oldOut })
	return rec
}

// The deprecated --1password alias and --sp shorthand are part of the
// documented surface and must stay wired.
func TestInitFlagSurface(t *testing.T) {
	f := find(t, "init").Flags()
	for _, name := range []string{
		"name", "path", "agent", "ide", "stacks",
		"proxy", "secret-provider", "sp", "features", "1password",
	} {
		if f.Lookup(name) == nil {
			t.Errorf("init is missing --%s", name)
		}
	}
}

func TestEveryModuleFromBashDispatcherExists(t *testing.T) {
	// One entry per directory under cli/libexec.
	modules := []string{
		"attach", "cache", "compose", "destroy", "edit",
		"init", "proxy", "restart-proxy", "run", "version",
	}
	have := map[string]bool{}
	for _, c := range NewRootCmd().Commands() {
		have[c.Name()] = true
	}
	for _, m := range modules {
		if !have[m] {
			t.Errorf("missing module command %q", m)
		}
	}
}
