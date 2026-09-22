package compose

import (
	"path/filepath"

	"github.com/jehoctor/snadcat/internal/agents"
	"github.com/jehoctor/snadcat/internal/project"
)

// Options are the mount toggles and selections that shape a generated
// compose-all.yml. They correspond to the SANDCAT_MOUNT_* environment
// variables the bash implementation reads, lifted into explicit parameters so
// the behavior is visible at the call site instead of ambient.
type Options struct {
	// SettingsFile is the settings path to mount, relative to the compose file.
	SettingsFile string

	Agent       agents.Agent
	IDE         string
	ProjectName string

	// Stacks are resolved stack names; they select the shared caches.
	Stacks []string

	MountAgentConfig  bool
	MountGitReadonly  bool
	MountIdeaReadonly bool
	MountSharedCache  bool
}

// DefaultOptions returns the toggle defaults the bash uses, before any
// environment overrides: agent config and shared caches on, the read-only
// .git and .idea mounts off.
func DefaultOptions() Options {
	return Options{
		MountAgentConfig: true,
		MountSharedCache: true,
	}
}

// ApplyEnvOverrides folds the SANDCAT_MOUNT_* environment variables into the
// options, matching customize_compose_file's defaulting.
//
// Each variable is "true" to enable; anything else disables. Unset leaves the
// existing value, so DefaultOptions decides the default.
func (o *Options) ApplyEnvOverrides(lookup func(string) (string, bool)) {
	set := func(target *bool, name string) {
		if v, ok := lookup(name); ok {
			*target = v == "true"
		}
	}
	if o.Agent.MountEnvVar != "" {
		set(&o.MountAgentConfig, o.Agent.MountEnvVar)
	}
	set(&o.MountGitReadonly, "SANDCAT_MOUNT_GIT_READONLY")
	set(&o.MountIdeaReadonly, "SANDCAT_MOUNT_IDEA_READONLY")
	set(&o.MountSharedCache, "SANDCAT_MOUNT_SHARED_CACHE")
}

// Customize applies every mutation `sandcat init` makes to compose-all.yml.
//
// The order matters: it decides where entries land in the volumes list and
// which entry ends up carrying the accumulated foot comments, so it is kept
// identical to customize_compose_file.
func (f *File) Customize(composeDir string, o Options) error {
	if err := project.VerifyRelativePath(composeDir, filepath.FromSlash(o.SettingsFile)); err != nil {
		return err
	}

	// The JetBrains backend reads .idea from the mount, so selecting that IDE
	// turns the mount on unless the user explicitly said otherwise.
	if o.IDE == "jetbrains" {
		o.MountIdeaReadonly = true
	}

	if err := f.SetWorkspace(o.ProjectName); err != nil {
		return err
	}
	if err := f.AddSettingsVolume(o.SettingsFile); err != nil {
		return err
	}
	if err := f.AddAgentConfigVolumes(o.Agent, o.MountAgentConfig, o.ProjectName); err != nil {
		return err
	}
	if err := f.AddGitReadonlyVolume(o.MountGitReadonly); err != nil {
		return err
	}
	if err := f.AddIdeaReadonlyVolume(o.MountIdeaReadonly); err != nil {
		return err
	}
	if err := f.AddSharedCacheVolumes(o.MountSharedCache, o.Stacks); err != nil {
		return err
	}
	if o.IDE == "jetbrains" {
		f.AddJetBrainsCapabilities()
	}
	return nil
}
