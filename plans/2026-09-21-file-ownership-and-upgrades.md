# File ownership, re-init, and upgrades

Status: **draft for discussion**, formulated 2026-09-21. Not scheduled. Depends
on the Go port (`plans/2026-08-07-go-port.md`) having landed; the templating
change in §6 additionally depends on that plan's steps 6e/6f (bash removed,
byte-parity retired).

## 1. The problem

Sandcat writes files under three different contracts and does not tell the user
which is which:

- some are **regenerated** on every `sandcat init` and are meant to be
  disposable;
- some are **generated once** and then belong to the user;
- some are a **managed region** inside a user-owned file.

A fourth category exists by accident: files that `init` regenerates but that
the tool also invites the user to edit (`sandcat edit compose`, commented-out
mounts "to uncomment later", the project `settings.json` with its network
rules). Edits there survive exactly until the next `init`.

Separately, the only upgrade path is "re-run `init`", and `init` is not
reproducible: it records none of the choices that shaped the tree (agent, IDE,
stacks, proxy mode, secret provider, features, `SANDCAT_MOUNT_*` toggles).
After a sandcat upgrade the user must remember every flag, and loses any
paradigm-4 edits. This is the classic templated-code problem, and the current
design has no answer to it.

## 2. Inventory today

Verified against the tree at `4a451ba` (upstream) + the Go port.

### `sandcat init`, user level

| File | Contract | Observations |
|---|---|---|
| `~/.config/sandcat/settings.json` | create-once, then idempotent merge | Template chosen by the **first** agent ever initialised. Later inits merge cursor/codex defaults and the secret-provider token key additively. **Gap:** no merge for claude/copilot, so a user whose first init was cursor never gets `ANTHROPIC_API_KEY`/`COPILOT_GITHUB_TOKEN` entries added. |
| `~/.claude/…`, `~/.cursor/…`, `~/.codex/…`, `~/.copilot/…` scaffolds | create-once | Pre-created so Docker doesn't materialise bind sources as root-owned. Correct as is. |

### `sandcat init`, project level

| File | Contract | Observations |
|---|---|---|
| `.devcontainer/compose-all.yml` | regenerate | Also the target of `sandcat edit compose` and carries commented-out optional mounts for the user to enable. **Paradigm 4.** |
| `.devcontainer/Dockerfile.app` | regenerate | Also the target of `sandcat edit dockerfile`. **Paradigm 4.** |
| `.devcontainer/devcontainer.json` | regenerate | Users do edit this (extensions, settings) and lose it. |
| `.devcontainer/devbox.stack.json` | regenerate | Sandcat-owned by design; correct. |
| `.devcontainer/devbox.tools.json` | create-once, user-owned, committed | The one fully correct example: sandcat owns `stack`, user owns `tools`, the Dockerfile merges at build time. |
| `.devcontainer/sandcat/**` | regenerate | Proxy compose, scripts, addons. Genuinely sandcat-owned. |
| `.sandcat/settings.json` | **regenerate — but the documented place for user network rules, and committed** | Plain `cp` from the template on every init (rewritten again for strict-network). **Paradigm 4, and the worst instance:** re-init silently discards a project's rules. |
| `.sandcat/settings.local.json` | create-once, user-owned, gitignored | Correct. |
| `.gitignore` | managed block | Marker-delimited; converges to the declared state. Correct. |

### Runtime

| State | Contract |
|---|---|
| `<project>_agent-home` volume | Persistent across runs; written by `app-user-init.sh` and by the agents. The stale-volume warning in `run` is the same problem at this layer: image rebuilt, volume still old. |
| `sandcat-cache-*` volumes | Host-shared, lazily created, removed only by `cache rm`. Correct. |
| `mitmproxy-*` volumes | Rewritten on every proxy start. Correct. |

### `sandcat destroy`

Removes `.devcontainer/` and `.sandcat/` wholesale — including
`settings.local.json`, which is gitignored and therefore unrecoverable. Footgun.

## 3. Target model

Three contracts, stated in the file itself and in the docs, with no fourth:

1. **Generated.** Written by sandcat, never by the user. Carries a header
   comment saying so and naming the source of truth. Regenerated on every
   `init`/`upgrade`. Gitignored.
2. **Owned.** Created once by sandcat as a scaffold; never written again.
   Committed or gitignored according to content (shared config vs secrets).
3. **Managed region.** A marker-delimited block or a JSON key sandcat merges
   into a user-owned file, never touching the rest.

Every place the user is currently invited to edit a generated file gets an
*owned* overlay instead, and the generated file consumes the overlay.

## 4. Changes

### 4.1 A manifest — makes regeneration reproducible

`.sandcat/manifest.json`, generated, committed:

```json
{
  "sandcat": "1.4.0",
  "generated": "2026-09-21T14:03:11Z",
  "agent": "claude",
  "ide": "vscode",
  "stacks": ["java", "scala"],
  "proxy": "web",
  "secret_provider": "1password",
  "features": ["strict-network"],
  "mounts": {"agent_config": true, "git_readonly": false, "idea_readonly": false, "shared_cache": true}
}
```

- `sandcat init` with no flags and a manifest present regenerates from it
  (today it would prompt). Flags override individual fields and the manifest is
  rewritten.
- `sandcat upgrade` (new) = regenerate from the manifest with the current
  binary, then report what changed in the generated files and whether the
  agent-home volume predates the new image (subsuming the stale-volume warning).
- The `SANDCAT_MOUNT_*` and `SANDCAT_RTK`/`SANDCAT_GITIGNORE`/
  `SANDCAT_STRICT_NETWORK` environment variables become *inputs* to the
  manifest on first init rather than ambient state that is forgotten.
- `sandcat` version in the manifest is what lets a future migration step say
  "generated by 1.3, template contract changed in 1.4, here is what to check".

This is a struct plus `jsonfile` on the Go side; the bash would have needed
another `yq` dance. It is the single highest-value change here and can ship
alone.

### 4.2 Overlays — removes paradigm 4

| Today | Becomes |
|---|---|
| Edit `compose-all.yml`; uncomment optional mounts | **`.devcontainer/compose.user.yml`**, owned, created once as `services: {}` with a comment. Both the CLI wrapper (`docker compose -f compose-all.yml -f compose.user.yml`) and `devcontainer.json`'s `dockerComposeFile` list it unconditionally, so it can't be forgotten. Optional mounts become manifest toggles; the commented-out rendering goes away (which is also the single biggest source of yq-shaped complexity in the Go port). `sandcat edit compose` opens the overlay. |
| Edit `Dockerfile.app` | **`.devcontainer/Dockerfile.user`**, owned, created once as a comment-only file; `Dockerfile.app` ends with a stage that `COPY`s and applies it (or `devbox.tools.json` already covers the package case — the overlay is for everything else). `sandcat edit dockerfile` opens the overlay. |
| Edit `devcontainer.json` | `devcontainer.json` gains a documented **managed region** (extensions, settings) with the rest user-owned — or the same overlay idea via `customizations` merge. The JSONC-with-comments format makes a region the more honest choice. |
| Edit `.sandcat/settings.json` | Becomes **owned with a managed key**: created once from the template; on re-init sandcat merges only the entries it owns (the stack presets under strict-network), never the user's rules. Template rules that exist to serve sandcat itself move into the addon's `NETWORK_PRESETS`, which strict-network already started. |

### 4.3 Small fixes that fall out

- `destroy` preserves `settings.local.json` and `manifest.json` unless
  `--purge`; prints what it kept.
- User-settings merge covers every agent (claude and copilot secrets entries),
  keyed off the manifest's agent rather than "first init wins".
- Every generated file starts with a one-line `# Generated by sandcat <ver>
  from .sandcat/manifest.json — do not edit; see <overlay>` header. Cheap, and
  it answers the new-user confusion directly.

## 5. Sequencing

| Step | Depends on | Ships alone? |
|---|---|---|
| 4.1 manifest + `upgrade` | Go port landed | yes — additive, no existing file changes contract |
| 4.3 destroy/merge/headers | 4.1 | yes |
| 4.2 compose overlay + mount toggles | 4.1; **bash removed (6e)** — the commented-out mount rendering is what parity currently pins | no |
| 4.2 Dockerfile overlay, devcontainer.json region, settings.json merge | 4.1 | each independently |
| §6 templating | 4.2 and 6f | no |

The manifest can go in while the bash oracle is still alive: it writes a new
file the bash never produces, so the harness only needs a per-case exclusion.

## 6. Related: template engine for the generated files

Recorded here because it only makes sense once §4.2 has separated generated
from owned, and it changes the template *sources* that the bash tree shares —
so it waits for 6e/6f.

**Today.** Templates are the literal output files with `__PLACEHOLDER__` marker
lines, expanded by whole-line replacement (a multi-line block replaces the
line; an empty block deletes it) or inline substitution, plus marker-comment
scanning for the IDE block in `devcontainer.json`. It is simple and it is
diff-friendly, but conditionals are encoded as "emit empty string", the
JetBrains block is text scanning, and nothing checks that every placeholder
was consumed.

**Proposal.** Move `Dockerfile.app`, `devcontainer.json`, `app-user-init.sh`
and the proxy compose to Go `text/template`, executed against a single typed
render context built from the manifest:

```
{{ if eq .IDE "jetbrains" }}…{{ end }}
{{ range .Stacks }}{{ .Extension }}{{ end }}
{{ .Agent.DockerInstallBlock }}
```

with `Option("missingkey=error")` so a template referencing a field the context
lacks fails at generation, not in the user's container. Whitespace control
(`{{-`/`-}}`) replaces the "drop the line when empty" rule.

**Not the compose files' `services.agent` mounts.** Those are *edited*
comment-preserving YAML today; under §4.2 they become a plain generated file
(toggles from the manifest) and can be templated like the rest. Until then they
stay on `yaml.Node`.

**Why not now.** The templates are shared bytes with the bash CLI and the
oracle diffs whole trees; changing their syntax breaks the oracle. It is a 6f
cleanup, and a modest one — the `devcontainer` package's placeholder functions
are ~150 lines and would be replaced, not extended.

**Alternatives considered.** `pongo2` (Jinja2 port) buys familiarity for people
coming from Python and nothing else here; `text/template` is standard library
and the logic that would need Jinja's richer expressions belongs in Go anyway.

## 7. Open questions

- Should `.sandcat/manifest.json` and `.sandcat/settings.json` be one file? One
  file is simpler for users; two keeps "what sandcat decided" apart from "what
  the user configured", which matters for the merge rules. Leaning two.
- `sandcat upgrade` vs making `init` do it: a separate verb is clearer about
  intent and can refuse when the manifest predates a contract change.
- Whether to keep `sandcat edit *` at all once overlays exist, or replace it
  with printing the path — the editor plumbing is the one piece of the port
  with a Windows-specific fallback.
