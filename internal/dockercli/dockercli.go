// Package dockercli wraps the docker and docker compose invocations the CLI
// makes. Nothing here talks to the daemon directly — the bash shells out to
// the docker CLI, and so does this, which keeps behavior identical (including
// contexts, credential helpers, and `docker compose` plugin resolution) and
// avoids depending on the Docker SDK.
package dockercli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
)

// Runner executes external commands. The default runs them for real;
// tests substitute a recorder so command construction can be asserted
// without a daemon — which is what the bats-mock tests did.
type Runner interface {
	// Run executes with stdio inherited, for interactive or long-running
	// commands whose output the user should see as it happens.
	Run(name string, args ...string) error
	// Output executes and returns trimmed stdout.
	Output(name string, args ...string) (string, error)
	// OutputQuiet is Output with stderr discarded, for probes whose failure
	// is an expected answer rather than a diagnostic.
	OutputQuiet(name string, args ...string) (string, error)
}

// ExecRunner runs commands with os/exec.
type ExecRunner struct{}

// Run inherits stdio. SIGINT is ignored in this process while the child
// runs: the terminal delivers Ctrl-C to the whole foreground group, and the
// child (docker) should be the one to act on it, after which its exit status
// is reported normally. This is the closest equivalent to the bash `exec`.
func (ExecRunner) Run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	signal.Ignore(os.Interrupt)
	defer signal.Reset(os.Interrupt)
	return cmd.Run()
}

// Output captures stdout; stderr passes through so docker's own diagnostics
// stay visible.
func (ExecRunner) Output(name string, args ...string) (string, error) {
	return capture(name, args, os.Stderr)
}

// OutputQuiet captures stdout and drops stderr.
func (ExecRunner) OutputQuiet(name string, args ...string) (string, error) {
	return capture(name, args, nil)
}

func capture(name string, args []string, stderr *os.File) (string, error) {
	cmd := exec.Command(name, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	if stderr != nil {
		cmd.Stderr = stderr
	}
	err := cmd.Run()
	return strings.TrimSpace(out.String()), err
}

// Default is the runner commands use unless overridden.
var Default Runner = ExecRunner{}

// Require reports an error when docker is not on PATH, mirroring require.bash.
func Require() error {
	if _, err := exec.LookPath("docker"); err != nil {
		return errors.New("docker required")
	}
	return nil
}

// Compose is a docker compose invocation bound to one compose file.
type Compose struct {
	File   string
	Runner Runner
}

// New returns a Compose for the given file using the default runner.
func New(file string) Compose { return Compose{File: file, Runner: Default} }

func (c Compose) runner() Runner {
	if c.Runner != nil {
		return c.Runner
	}
	return Default
}

func (c Compose) args(rest ...string) []string {
	return append([]string{"compose", "-f", c.File}, rest...)
}

// Run executes `docker compose -f <file> args...` with stdio inherited.
func (c Compose) Run(args ...string) error {
	return c.runner().Run("docker", c.args(args...)...)
}

// Output executes and captures stdout.
func (c Compose) Output(args ...string) (string, error) {
	return c.runner().Output("docker", c.args(args...)...)
}

// OutputQuiet executes with stderr discarded.
func (c Compose) OutputQuiet(args ...string) (string, error) {
	return c.runner().OutputQuiet("docker", c.args(args...)...)
}

// RunningServices returns the ids of running containers, optionally for one
// service, via `ps --status running --quiet`. Empty means nothing is up.
func (c Compose) RunningServices(service string) string {
	args := []string{"ps"}
	if service != "" {
		args = append(args, service)
	}
	out, err := c.OutputQuiet(append(args, "--status", "running", "--quiet")...)
	if err != nil {
		return ""
	}
	return out
}

// Docker runs plain `docker` subcommands.
type Docker struct{ Runner Runner }

func (d Docker) runner() Runner {
	if d.Runner != nil {
		return d.Runner
	}
	return Default
}

// Run executes `docker args...` with stdio inherited.
func (d Docker) Run(args ...string) error { return d.runner().Run("docker", args...) }

// Output executes `docker args...` and captures stdout.
func (d Docker) Output(args ...string) (string, error) {
	return d.runner().Output("docker", args...)
}

// OutputQuiet executes `docker args...` with stderr discarded.
func (d Docker) OutputQuiet(args ...string) (string, error) {
	return d.runner().OutputQuiet("docker", args...)
}

// ExitCode extracts a child's exit status from err, or 1 for other failures.
func ExitCode(err error) int {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	if err != nil {
		return 1
	}
	return 0
}

// ExitError carries a child process exit status up to main without a message
// of its own, since the child already printed whatever it had to say.
type ExitError struct{ Code int }

func (e ExitError) Error() string { return fmt.Sprintf("exit status %d", e.Code) }
