package cli

import (
	"github.com/spf13/cobra"

	"github.com/jehoctor/snadcat/internal/dockercli"
	"github.com/jehoctor/snadcat/internal/project"
)

// The commands in this file forward their arguments to `docker compose` more or
// less verbatim. They all set DisableFlagParsing so cobra does not consume
// flags meant for docker — `sandcat compose up --build` must not have --build
// interpreted as a sandcat flag, and `sandcat compose --help` must reach docker
// rather than printing sandcat's usage. See plans/2026-08-07-go-port.md §1.

// composeForCwd locates the project's compose file from the working directory.
func composeForCwd() (dockercli.Compose, error) {
	if err := dockercli.Require(); err != nil {
		return dockercli.Compose{}, err
	}
	file, err := project.FindComposeFile(".")
	if err != nil {
		return dockercli.Compose{}, err
	}
	return dockercli.New(file), nil
}

// childExit turns a docker failure into an ExitError so main propagates the
// status without printing a second message on top of docker's own.
func childExit(err error) error {
	if err == nil {
		return nil
	}
	return dockercli.ExitError{Code: dockercli.ExitCode(err)}
}

func newComposeCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "compose [docker compose args...]",
		Short:              "Run docker compose against the project's compose file",
		DisableFlagParsing: true,
		RunE: func(_ *cobra.Command, args []string) error {
			c, err := composeForCwd()
			if err != nil {
				return err
			}
			return childExit(c.Run(args...))
		},
	}
}

func newAttachCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "attach [command...]",
		Short:              "Exec into the running agent container (default: login shell)",
		DisableFlagParsing: true,
		RunE: func(_ *cobra.Command, args []string) error {
			c, err := composeForCwd()
			if err != nil {
				return err
			}
			if len(args) == 0 {
				args = []string{"bash", "--login"}
			}
			return childExit(c.Run(append([]string{"exec", "-u", "vscode", "agent"}, args...)...))
		},
	}
}

func newRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run [--build] [-- command...]",
		Short: "Run a command in a fresh agent container, then tear it down",
		// Flags before `--` go to `docker compose run`, everything after is the
		// command to execute; parsing that split by hand matches libexec/run/run.
		DisableFlagParsing: true,
		RunE: func(_ *cobra.Command, args []string) error {
			c, err := composeForCwd()
			if err != nil {
				return err
			}

			var runOpts []string
		loop:
			for len(args) > 0 {
				switch args[0] {
				case "--":
					args = args[1:]
					break loop
				case "--build":
					runOpts = append(runOpts, args[0])
					args = args[1:]
				default:
					break loop
				}
			}
			command := args
			if len(command) == 0 {
				command = []string{"bash"}
			}

			d := dockercli.Docker{Runner: c.Runner}
			dockercli.WarnStaleHomeVolume(d, c.File)
			dockercli.EnsureSharedCacheVolumes(d, c.File)

			// Tear down even when the command failed, then report the
			// command's own status — the bash does the same.
			runArgs := append(append([]string{"run", "--rm"}, runOpts...), "agent")
			runErr := c.Run(append(runArgs, command...)...)
			if err := c.Run("down"); err != nil && runErr == nil {
				return childExit(err)
			}
			return childExit(runErr)
		},
	}
}
