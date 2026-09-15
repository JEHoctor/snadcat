// Package config ports the settings-file handling from cli/libexec/init/init
// and cli/libexec/init/settings: the per-user ~/.config/sandcat/settings.json
// (git identity, API keys, secret-backend tokens) and the per-project
// .sandcat/settings.json (network rules).
package config

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	sandcat "github.com/jehoctor/snadcat"
	"github.com/jehoctor/snadcat/internal/jsonfile"
	"github.com/jehoctor/snadcat/internal/log"
)

// UserDir returns the user config directory, ~/.config/sandcat.
//
// It is derived from HOME rather than os.UserConfigDir so it matches the bash
// (`$HOME/.config/sandcat`) on every platform, including macOS where
// UserConfigDir would point at ~/Library/Application Support.
func UserDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "sandcat"), nil
}

// UserSettingsPath returns ~/.config/sandcat/settings.json.
func UserSettingsPath() (string, error) {
	dir, err := UserDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "settings.json"), nil
}

// gitConfig is indirected so tests can pin the host identity.
var gitConfig = func(key string) string {
	out, err := exec.Command("git", "config", "--global", key).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// CreateUserSettings writes ~/.config/sandcat/settings.json from the agent's
// template if it does not already exist, seeding the git identity from the
// host's global git config. An existing file is left untouched.
func CreateUserSettings(agent string) error {
	path, err := UserSettingsPath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		return nil
	}

	tmpl, err := fs.ReadFile(sandcat.Templates, "settings-user-"+agent+".json")
	if err != nil {
		return fmt.Errorf("missing user settings template for agent '%s'", agent)
	}
	v, err := jsonfile.Parse(tmpl)
	if err != nil {
		return err
	}
	obj, ok := v.(*jsonfile.Object)
	if !ok {
		return fmt.Errorf("user settings template for %s is not an object", agent)
	}

	name, email := gitConfig("user.name"), gitConfig("user.email")
	if name == "" {
		name = "Your Name"
	}
	if email == "" {
		email = "you@example.com"
	}
	env := jsonfile.EnsureObject(obj, "env")
	env.Set("GIT_USER_NAME", name)
	env.Set("GIT_USER_EMAIL", email)

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return jsonfile.Save(path, obj)
}

// editUserSettings applies fn to the user settings file when it exists.
func editUserSettings(fn func(*jsonfile.Object)) error {
	path, err := UserSettingsPath()
	if err != nil {
		return err
	}
	obj, err := jsonfile.Load(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	fn(obj)
	return jsonfile.Save(path, obj)
}

// ApplyAgentDefaults seeds agent-specific defaults into an existing user
// settings file without overriding anything the user set.
func ApplyAgentDefaults(agent string) error {
	switch agent {
	case "cursor":
		return editUserSettings(cursorDefaults)
	case "codex":
		return editUserSettings(codexDefaults)
	}
	return nil
}

func cursorDefaults(obj *jsonfile.Object) {
	jsonfile.EnsureObject(obj, "env")
	secrets := jsonfile.EnsureObject(obj, "secrets")
	key := jsonfile.EnsureObject(secrets, "CURSOR_API_KEY")
	jsonfile.SetDefault(key, "value", "")
	hosts := jsonfile.EnsureArray(key, "hosts")
	key.Set("hosts", jsonfile.UniqueStrings(hosts,
		"api.cursor.sh", "api2.cursor.sh", "*.cursor.sh", "*.cursor.com"))

	cursor := jsonfile.EnsureObject(obj, "cursor")
	cli := jsonfile.EnsureObject(cursor, "cli")
	jsonfile.SetDefault(cli, "version", json.Number("1"))
	network := jsonfile.EnsureObject(cli, "network")
	// `//` would treat an explicit false as absent and flip it back to true,
	// so this one is keyed on presence alone.
	if _, ok := network.Get("useHttp1ForAgent"); !ok {
		network.Set("useHttp1ForAgent", true)
	}
}

func codexDefaults(obj *jsonfile.Object) {
	jsonfile.EnsureObject(obj, "env")
	secrets := jsonfile.EnsureObject(obj, "secrets")
	key := jsonfile.EnsureObject(secrets, "OPENAI_API_KEY")
	jsonfile.SetDefault(key, "value", "")
	hosts := jsonfile.EnsureArray(key, "hosts")
	key.Set("hosts", jsonfile.UniqueStrings(hosts, "api.openai.com"))

	rule := jsonfile.NewObject()
	rule.Set("action", "allow")
	rule.Set("host", "api.openai.com")
	network := jsonfile.EnsureArray(obj, "network")
	obj.Set("network", jsonfile.UniqueBy(network, networkRuleKey, rule))
}

// networkRuleKey is `.host + (.method // "") + .action`.
func networkRuleKey(v any) string {
	o, ok := v.(*jsonfile.Object)
	if !ok {
		return fmt.Sprint(v)
	}
	return jsonfile.String(o, "host") + jsonfile.String(o, "method") + jsonfile.String(o, "action")
}

// AddSecretProviderToken seeds the provider's token field when missing.
func AddSecretProviderToken(provider string) error {
	var key string
	switch provider {
	case "1password":
		key = "op_service_account_token"
	case "protonpass":
		key = "proton_pass_token"
	default:
		return nil
	}
	return editUserSettings(func(obj *jsonfile.Object) {
		jsonfile.SetDefault(obj, key, "")
	})
}

// ConfiguredSecretProvider reports which provider already has a non-empty
// token in the user settings, so the interactive picker can put it first.
// Returns "" when neither does or the file is missing.
func ConfiguredSecretProvider() string {
	path, err := UserSettingsPath()
	if err != nil {
		return ""
	}
	obj, err := jsonfile.Load(path)
	if err != nil {
		return ""
	}
	if jsonfile.String(obj, "op_service_account_token") != "" {
		return "1password"
	}
	if jsonfile.String(obj, "proton_pass_token") != "" {
		return "protonpass"
	}
	return ""
}

// ProjectSettingsOptions shape the project settings file `init` writes.
type ProjectSettingsOptions struct {
	// StrictNetwork replaces the template's allow-all-GET wildcard with one
	// preset entry per stack. The names are expanded to concrete allow rules
	// at proxy start by the mitmproxy addon, so the domain lists track the
	// sandcat version instead of freezing in the project. With no stacks the
	// list is empty: everything beyond the user-settings layer is denied.
	StrictNetwork bool
	// Stacks are the resolved stack names whose presets seed a strict policy.
	Stacks []string
}

// WriteProjectSettings writes .sandcat/settings.json from the template and,
// when missing, an empty settings.local.json next to it.
//
// The local file is the highest-precedence layer in the addon's settings
// merge and is kept out of git by the Sandcat .gitignore block; unlike
// settings.json it is never overwritten on re-init, since it is where users
// put real credentials.
func WriteProjectSettings(path string, o ProjectSettingsOptions) error {
	b, err := fs.ReadFile(sandcat.Templates, "settings.json")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	if o.StrictNetwork {
		// The bash rewrites through `yq -o=json`, which reformats the whole
		// document; going through jsonfile reproduces that layout.
		v, err := jsonfile.Parse(b)
		if err != nil {
			return err
		}
		obj, ok := v.(*jsonfile.Object)
		if !ok {
			return fmt.Errorf("settings template is not an object")
		}
		presets := []any{}
		for _, s := range o.Stacks {
			p := jsonfile.NewObject()
			p.Set("preset", s)
			presets = append(presets, p)
		}
		obj.Set("network", presets)
		if b, err = jsonfile.Encode(obj); err != nil {
			return err
		}
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return err
	}
	log.Info("Settings file created at %s", path)

	local := strings.TrimSuffix(path, ".json") + ".local.json"
	if _, err := os.Stat(local); err == nil {
		return nil
	}
	if err := os.WriteFile(local, []byte("{}\n"), 0o644); err != nil {
		return err
	}
	log.Info("Local settings scaffold created at %s", local)
	return nil
}
