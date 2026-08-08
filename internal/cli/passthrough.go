package cli

import (
	"github.com/spf13/cobra"
)

// The commands in this file forward their arguments to `docker compose` more or
// less verbatim. They all set DisableFlagParsing so cobra does not consume
// flags meant for docker — `sandcat compose up --build` must not have --build
// interpreted as a sandcat flag, and `sandcat compose --help` must reach docker
// rather than printing sandcat's usage. See GO-PORT-PLAN.md §1.

func newComposeCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "compose [docker compose args...]",
		Short:              "Run docker compose against the project's compose file",
		DisableFlagParsing: true,
		RunE:               func(*cobra.Command, []string) error { return errNotImplemented },
	}
}

func newAttachCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "attach [command...]",
		Short:              "Exec into the running agent container (default: login shell)",
		DisableFlagParsing: true,
		RunE:               func(*cobra.Command, []string) error { return errNotImplemented },
	}
}

func newRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run [--build] [-- command...]",
		Short: "Run a command in a fresh agent container, then tear it down",
		// Flags before `--` go to `docker compose run`, everything after is the
		// command to execute; parsing that split by hand matches libexec/run/run.
		DisableFlagParsing: true,
		RunE:               func(*cobra.Command, []string) error { return errNotImplemented },
	}
}
