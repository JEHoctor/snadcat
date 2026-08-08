package cli

import (
	"github.com/spf13/cobra"
)

func newDestroyCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "destroy",
		Short: "Stop containers, remove volumes, and delete .devcontainer and .sandcat",
		Args:  cobra.NoArgs,
		RunE:  func(*cobra.Command, []string) error { return errNotImplemented },
	}
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip the confirmation prompt")
	return cmd
}

func newProxyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "proxy",
		Short: "Open the mitmproxy web UI, or follow proxy logs in console mode",
		Args:  cobra.NoArgs,
		RunE:  func(*cobra.Command, []string) error { return errNotImplemented },
	}
}

func newRestartProxyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restart-proxy",
		Short: "Restart the proxy stack to pick up settings changes",
		Args:  cobra.NoArgs,
		RunE:  func(*cobra.Command, []string) error { return errNotImplemented },
	}
}

func newEditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "edit",
		Short: "Open a Sandcat file in $VISUAL/$EDITOR",
	}
	for _, s := range []struct{ use, short string }{
		{"compose", "Edit .devcontainer/compose-all.yml"},
		{"dockerfile", "Edit .devcontainer/Dockerfile.app"},
		{"project-settings", "Edit .sandcat/settings.json"},
		{"user-settings", "Edit ~/.config/sandcat/settings.json"},
	} {
		cmd.AddCommand(&cobra.Command{
			Use:   s.use,
			Short: s.short,
			Args:  cobra.NoArgs,
			RunE:  func(*cobra.Command, []string) error { return errNotImplemented },
		})
	}
	return cmd
}
