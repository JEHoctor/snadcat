package initialize

import (
	"fmt"
	"strings"

	"github.com/jehoctor/snadcat/internal/agents"
	"github.com/jehoctor/snadcat/internal/log"
	"github.com/jehoctor/snadcat/internal/project"
)

type summary struct {
	agent           agents.Agent
	stacks          []string
	gitignoreStatus string
	rtkEnabled      bool
	provider        string
}

// blank prints an empty line to the log stream, as `echo "" >&2` does between
// sections of the bash summary.
func blank() { fmt.Fprintln(log.Out) }

// printSummary reproduces the "Initialization complete" report and the
// provider-specific next steps.
func printSummary(s summary) {
	blank()
	log.Info("Initialization complete.")
	if len(s.stacks) > 0 {
		log.Info("  Stacks:           %s (devbox packages baked into .devcontainer/devbox.stack.json)", strings.Join(s.stacks, " "))
	}
	log.Info("  Devbox tools:     edit .devcontainer/devbox.tools.json to add packages, then rebuild (sandcat run --build)")
	log.Info("  Devbox stack:     .devcontainer/devbox.stack.json is regenerated on every init from --stacks")
	log.Info("  Project settings: %s/settings.json (network rules)", project.Dir)
	log.Info("  User settings:    ~/.config/sandcat/settings.json (git identity, API keys)")
	log.Info("  Devcontainer:     .devcontainer/")
	log.Info("  Gitignore:        %s", s.gitignoreStatus)
	if s.rtkEnabled {
		log.Info("  RTK:              installed (disable with --features no-rtk)")
	} else {
		log.Info("  RTK:              disabled")
	}
	switch s.provider {
	case "1password":
		log.Info("  Secret provider:  1Password")
	case "protonpass":
		log.Info("  Secret provider:  Proton Pass")
	}

	blank()
	log.Info("Next steps:")
	log.Info("  Edit ~/.config/sandcat/settings.json to add your API keys:")
	switch s.provider {
	case "1password":
		log.Info("    %s", s.agent.OpAPIKeyHelp)
		log.Info(`    GITHUB_TOKEN       "op": "op://vault/GitHub Token/credential"`)
		log.Info("    (adjust vault/item names to match your 1Password setup,")
		log.Info(`     or use plain values with "value": "sk-ant-...")`)
		blank()
		log.Info("  1Password setup:")
		log.Info("    1. Create a service account at https://my.1password.com/developer-tools/infrastructure-secrets/serviceaccount/")
		log.Info("    2. Grant it read access to the vault(s) containing your secrets")
		log.Info("    3. Add the token to ~/.config/sandcat/settings.json:")
		log.Info(`       "op_service_account_token": "ops_..."`)
	case "protonpass":
		log.Info("    %s", s.agent.APIKeyHelp)
		log.Info(`    GITHUB_TOKEN       "pass": "pass://vault/GitHub Token/credential"`)
		blank()
		log.Info("  Proton Pass setup (scoped Personal Access Token):")
		log.Info("    1. Create a PAT with zero access:")
		log.Info(`         pass-cli pat create --name "sandcat" --expiration 3m`)
		log.Info("    2. Grant read-only access to ONLY the vaults/items this project needs:")
		log.Info(`         pass-cli pat access grant --pat-name "sandcat" \`)
		log.Info(`           --vault-name "<YourVault>" --role viewer`)
		log.Info(`       (or restrict to a single item with --item-title "<ItemName>")`)
		log.Info("    3. Paste the printed pst_... token into ~/.config/sandcat/settings.json:")
		log.Info(`         "proton_pass_token": "pst_..."`)
		log.Info(`    4. Reference items with "pass": "pass://<YourVault>/<Item>/<field>"`)
	default:
		log.Info("    %s", s.agent.APIKeyHelp)
		log.Info("    GITHUB_TOKEN       a GitHub personal access token (for git push, gh cli)")
	}
	log.Info("  Then run: sandcat run, or reopen the project using the dev container")
}
