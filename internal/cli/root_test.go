package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
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
	root := NewRootCmd()
	root.SetArgs([]string{"compose", "up", "--build", "--wait"})
	err := root.Execute()
	// Reaching the stub means the args survived parsing; an "unknown flag"
	// error would mean cobra consumed them.
	if !errors.Is(err, errNotImplemented) {
		t.Fatalf("got %v, want errNotImplemented", err)
	}
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
