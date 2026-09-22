package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jehoctor/snadcat/internal/config"
	"github.com/jehoctor/snadcat/internal/dockercli"
	"github.com/jehoctor/snadcat/internal/editor"
	"github.com/jehoctor/snadcat/internal/log"
	"github.com/jehoctor/snadcat/internal/project"
	"github.com/jehoctor/snadcat/internal/prompt"
)

func newDestroyCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "destroy",
		Short: "Stop containers, remove volumes, and delete .devcontainer and .sandcat",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			root, err := project.FindRoot(".")
			if err != nil {
				return err
			}
			if !force {
				log.Warn("This will stop containers, remove volumes, and delete .devcontainer and %s directories.", project.Dir)
				yes, err := prompt.YesNo("Continue?")
				if err != nil {
					return err
				}
				if !yes {
					log.Warn("Aborting")
					return nil
				}
			}

			log.Info("Stopping containers")
			c, err := composeForCwd()
			if err != nil {
				return err
			}
			if err := c.Run("down", "--volumes"); err != nil {
				return childExit(err)
			}

			for _, dir := range []string{filepath.Join(root, project.Dir), filepath.Join(root, ".devcontainer")} {
				if _, err := os.Stat(dir); err != nil {
					continue
				}
				log.Info("Removing %s", dir)
				if err := os.RemoveAll(dir); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip the confirmation prompt")
	return cmd
}

func newProxyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "proxy",
		Short: "Open the mitmproxy web UI, or follow proxy logs in console mode",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			c, err := composeForCwd()
			if err != nil {
				return err
			}

			// Web mode publishes the UI port; console mode (mitmdump) does
			// not, so the rendered config tells the two apart.
			rendered, err := c.Output("config", "--format", "json")
			if err != nil {
				return childExit(err)
			}
			var cfg struct {
				Services struct {
					Mitmproxy struct {
						Ports []any `json:"ports"`
					} `json:"mitmproxy"`
				} `json:"services"`
			}
			if err := json.Unmarshal([]byte(rendered), &cfg); err != nil {
				return fmt.Errorf("parsing compose config: %w", err)
			}

			if len(cfg.Services.Mitmproxy.Ports) == 0 {
				log.Info("Following mitmproxy logs (Ctrl+C to stop)...")
				return childExit(c.Run("logs", "-f", "mitmproxy"))
			}
			url, err := c.OutputQuiet("port", "mitmproxy", "8081")
			if err != nil || url == "" {
				return fmt.Errorf("Proxy is not running. Start it first with: snadcat run or snadcat compose up -d")
			}
			log.Info("mitmweb UI: http://%s (password: mitmproxy)", url)
			return nil
		},
	}
}

func newRestartCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "restart",
		Short: "Restart the proxy stack (and re-link the agent) to pick up settings changes",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			c, err := composeForCwd()
			if err != nil {
				return err
			}
			if c.RunningServices("mitmproxy") == "" {
				log.Warn("Proxy service is not running.")
				return nil
			}
			// Captured before anything restarts: the agent is only re-linked
			// if it was up when the user asked.
			agentWasRunning := c.RunningServices("agent") != ""

			log.Info("Restarting proxy service...")
			steps := [][]string{
				{"restart", "mitmproxy"},
				// Wait for health before touching wg-client — a plain restart
				// bypasses the compose depends_on health gate.
				{"up", "-d", "--wait", "--wait-timeout", "60", "mitmproxy"},
				{"restart", "wg-client"},
			}
			if agentWasRunning {
				// The agent runs with network_mode: service:wg-client, which
				// Docker resolves to a netns fd at agent start. Restarting
				// wg-client creates a fresh netns but the agent's fd still
				// points at the old one, so DNS and all outbound traffic
				// break inside it (#69). Waiting for wg-client to be healthy
				// first guarantees the agent re-links to the ready netns.
				steps = append(steps,
					[]string{"up", "-d", "--wait", "--wait-timeout", "60", "wg-client"},
					[]string{"restart", "agent"},
				)
			}
			for _, args := range steps {
				if err := c.Run(args...); err != nil {
					return childExit(err)
				}
			}
			log.Info("Proxy restarted. New settings are now active.")
			return nil
		},
	}
	return cmd
}

// newRestartProxyCmd keeps the pre-#93 name working as a hidden alias so
// existing scripts don't break; the bash dropped it outright.
func newRestartProxyCmd() *cobra.Command {
	cmd := newRestartCmd()
	cmd.Use = "restart-proxy"
	cmd.Hidden = true
	cmd.Deprecated = "use `snadcat restart`"
	return cmd
}

func newEditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "edit",
		Short: "Open a Snadcat file in $VISUAL/$EDITOR",
	}

	// The three plain editors just need a path that must already exist.
	type target struct {
		use, short string
		path       func() (string, error)
		missing    string
	}
	for _, t := range []target{
		{
			use: "dockerfile", short: "Edit .devcontainer/Dockerfile.app",
			path:    func() (string, error) { return rootPath(".devcontainer", "Dockerfile.app") },
			missing: "No Dockerfile found: %s",
		},
		{
			use: "project-settings", short: "Edit .sandcat/settings.json",
			path:    func() (string, error) { return rootPath(project.Dir, "settings.json") },
			missing: "No settings file found: %s",
		},
		{
			use: "user-settings", short: "Edit ~/.config/sandcat/settings.json",
			path:    config.UserSettingsPath,
			missing: "No user settings file found: %s\nRun 'snadcat init' first to create it.",
		},
	} {
		t := t
		cmd.AddCommand(&cobra.Command{
			Use:   t.use,
			Short: t.short,
			Args:  cobra.NoArgs,
			RunE: func(*cobra.Command, []string) error {
				path, err := t.path()
				if err != nil {
					return err
				}
				if _, err := os.Stat(path); err != nil {
					return fmt.Errorf(t.missing, path)
				}
				return editor.Open(path)
			},
		})
	}

	cmd.AddCommand(newEditComposeCmd())
	return cmd
}

func rootPath(parts ...string) (string, error) {
	root, err := project.FindRoot(".")
	if err != nil {
		return "", err
	}
	return filepath.Join(append([]string{root}, parts...)...), nil
}

// `sandcat edit compose` restarts running containers after a change unless
// told not to, since a compose edit that isn't applied is a common trap.
func newEditComposeCmd() *cobra.Command {
	var noRestart bool
	cmd := &cobra.Command{
		Use:   "compose",
		Short: "Edit .devcontainer/compose-all.yml",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			file, err := project.FindComposeFile(".")
			if err != nil {
				return err
			}
			before, err := os.Stat(file)
			if err != nil {
				return err
			}
			if err := editor.Open(file); err != nil {
				return err
			}
			after, err := os.Stat(file)
			if err != nil {
				return err
			}
			if after.ModTime().Equal(before.ModTime()) {
				log.Info("No changes detected.")
				return nil
			}

			c := dockercli.New(file)
			if c.RunningServices("") == "" {
				log.Info("Compose file was modified.")
				return nil
			}
			if noRestart || strings.EqualFold(os.Getenv("SANDCAT_NO_RESTART"), "true") {
				log.Warn("Compose file was modified, and you have containers running.")
				log.Warn("To pick up the changes and restart your containers, run: snadcat compose up -d")
				return nil
			}
			log.Info("Compose file was modified. Restarting containers...")
			return childExit(c.Run("up", "-d"))
		},
	}
	cmd.Flags().BoolVar(&noRestart, "no-restart", false, "Do not restart running containers after editing")
	return cmd
}
