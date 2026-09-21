# Porting the Sandcat CLI to Go

Plan of record for replacing the bash CLI under `cli/` with a single static Go
binary. Branched from `main` at `c16d8fd`.

**Status:** Milestones 1–5 complete. Every command is implemented;
`scripts/difftest.sh` shows 27/27 `init` configurations byte-identical to the
bash CLI across both the project and home trees. Milestone 6 was re-planned on
2026-09-14 as a sequence that keeps the bash tree alive as the oracle through
the upstream sync and the podman rework — see §4.1. Next step: 6a, land this
branch in the fork.

**Drivers** (in priority order, per the decision to do a full rewrite rather
than an incremental command-by-command migration):

1. **Single-binary distribution** — drop the `curl | sh` tree-copy installer and
   the hard `yq` host prerequisite.
2. **Maintainability / type safety** — the current YAML and JSON editing is done
   by shelling out to `yq` with expression strings, and template customization
   is line-oriented `sed`/`while read` filtering. Both are string-typed and only
   observable through end-to-end tests.
3. **Cross-platform, including native Windows** — the CLI currently requires
   Bash >= 3.2 plus `sed`, `awk`, `find`, `stat`, `tail`, `comm`, `jq`, `yq`.

Non-goal: changing what Sandcat *does*. The generated `.devcontainer/` tree, the
proxy templates, and the container-side shell scripts are out of scope. The port
replaces the thing that *writes* those files, not the files themselves.

---

## 1. What is actually being ported

`cli/` is ~3,800 lines of bash: a `bin/sandcat` dispatcher, 15 `lib/*.bash`
helper files, 21 `libexec/<module>/<command>` executables, and 31 `.bats` test
files across 4 vendored bats submodules.

The logic is very unevenly distributed. Four files hold nearly all of it:

| File | Lines | What it does |
|---|---|---|
| `lib/agents.bash` | 515 | Per-agent (claude/cursor/codex) data tables and heredoc text blocks |
| `libexec/init/init` | 503 | Flag parsing, interactive prompts, orchestration, next-steps output |
| `lib/composefile.bash` | 443 | `yq` mutation of `compose-all.yml` / `compose-proxy.yml` |
| `lib/devcontainer.bash` + `lib/devbox.bash` | 258 + 235 | Template placeholder expansion, devbox JSON generation |

The remaining `libexec` entries are 5–100 line wrappers around `docker compose`
(`run`, `compose`, `attach`, `destroy`, `proxy`, `restart-proxy`, `cache`,
`edit`, `version`). Those are quick.

### The command surface is preserved exactly

`bin/sandcat` dispatches `sandcat <module> [command] [args...]`, where module is
a directory under `libexec/` and command is an executable inside it, with `_`
as the catch-all and a bare `<module>` defaulting to the same-named command.
This maps cleanly onto cobra groups and subcommands, so every invocation the
README documents keeps working:

```
sandcat init [settings|devcontainer]   sandcat cache [list|size|rm]
sandcat run [--build] [-- cmd...]      sandcat edit [compose|dockerfile|project-settings|user-settings]
sandcat compose <any docker args>      sandcat attach | destroy | proxy | restart-proxy | version
```

Two dispatcher behaviors are load-bearing and must be reproduced deliberately,
because cobra does not do them by default:

- `sandcat compose <anything>` must pass *all* remaining args through untouched
  to `docker compose` (the `libexec/compose/_` catch-all). Needs
  `DisableFlagParsing` / `TraverseChildren` care, or cobra will eat `--build`,
  `-f`, `--help`, etc.
- Every module except `version` prints the version banner before running.

---

## 2. Package layout

Module at repo root, `module github.com/jehoctor/snadcat`.

```
go.mod
embed.go                     package sandcat — //go:embed all:cli/templates
cmd/sandcat/main.go          thin main; calls internal/cli
internal/
  cli/                       cobra command tree, one file per module
    root.go init.go run.go compose.go cache.go edit.go ...
  config/                    typed settings.json / user-settings models
  compose/                   yaml.Node mutation of compose files
  devcontainer/              devcontainer.json + Dockerfile.app + script placeholders
  devbox/                    devbox.stack.json / devbox.tools.json + Dockerfile block
  stacks/                    stack table: packages, extensions, deps, shared caches
  agents/                    per-agent table: extensions, blocks, mounts, mitm flags
  rtk/                       rtk install + init blocks
  gitignore/                 the marker-bracketed .gitignore block
  project/                   repo-root discovery, compose-file discovery, name derivation
  dockercli/                 os/exec wrappers for docker / docker compose / volumes
  prompt/                    select_option / select_multiple / read_line / editor
  log/                       info / warning / error, all to stderr
```

**Why the embed file sits at the module root.** `//go:embed` paths are relative
to the embedding source file's directory and cannot contain `..`, so no package
under `internal/` can reach `cli/templates/`. Keeping the templates where they
are during the port is worth more than layout purity: it means the bash CLI and
the Go CLI consume the *same bytes*, which is what makes the differential
harness in §5 a real parity oracle. At cutover (Milestone 6), when `cli/`
bash is deleted, the tree moves to `internal/templates/assets/` and `embed.go`
folds into that package.

Use `all:` on the embed pattern — `cli/templates/devcontainer/sandcat/scripts/`
contains files that the default pattern rules would otherwise be fine with, but
`all:` is the safe default and costs nothing.

---

## 3. The three hard problems

Everything else in this port is mechanical. These are not.

### 3.1 YAML comments are load-bearing

This is the single biggest correctness risk, and it is worse than it looks from
a distance. `lib/composefile.bash` does not just set YAML fields — it uses `yq`
to attach `head_comment` and `foot_comment` to specific nodes, and it renders
*disabled* optional mounts as commented-out YAML text inside a foot comment so
the user can uncomment them later:

```bash
add_volume_entry "$compose_file" '${HOME}/.claude/CLAUDE.md:...:ro' "$active" 'Host Claude config (optional)'
# active=true  → real list item + head_comment
# active=false → appended into the previous item's foot_comment as "- ${HOME}/..."
```

There is also a `sed` post-pass in `customize_compose_file` that strips blank
lines yq inserts between a foot comment and the next sibling node.

**Decision: reproduce this exactly, with `gopkg.in/yaml.v3` `yaml.Node`.** Do
not "clean it up" into typed struct marshaling, and do not move the toggles into
`settings.json`. Two reasons:

- `yaml.Node` carries `HeadComment` / `LineComment` / `FootComment` and
  round-trips them, so the capability is there.
- Mike Farah's `yq` — the tool being replaced — is itself Go and is built on
  `yaml.v3`. Matching its output byte-for-byte is therefore plausible rather
  than aspirational, and the blank-line `sed` hack may turn out to be
  unnecessary once we control the encoder directly (set indent to 2 and check).

Byte-for-byte parity here is what lets §5 diff whole trees instead of
hand-auditing semantic equivalence.

**Resolved (Milestone 3).** Byte-for-byte parity is achieved and enforced by
`internal/compose/parity_test.go`, which runs the original bash functions and
the Go ones over the same fixtures and requires identical output across 21
combinations of agent, IDE, stack set and mount toggle. Two findings worth
recording:

- `yaml.Encoder.SetIndent(2)` is the whole of what was needed to match yq's
  layout; comment attachment round-trips unchanged.
- The `sed` post-pass **is** load-bearing, contrary to a first reading. It does
  nothing in the arrangement it was probably written for, but it is required
  when a disabled mount (rendered into a foot comment) is followed by an active
  mount carrying a head comment — the `--ide jetbrains` path hits this. It is
  reproduced in `stripBlankBeforeIndented`, pairwise consumption and all.

### 3.2 Order-preserving JSON with `//`-style defaulting

`libexec/init/init` seeds user settings with `yq -o json` expressions whose
whole semantic is *set only if absent*:

```
.secrets.CURSOR_API_KEY = (.secrets.CURSOR_API_KEY // {"value": "", "hosts": []}) |
.secrets.CURSOR_API_KEY.hosts = ((.secrets.CURSOR_API_KEY.hosts // []) + [...] | unique)
```

`encoding/json` into `map[string]any` loses key order, which would churn the
user's settings file on every `init`.

**Decision (revised in Milestone 4):** `internal/jsonfile`, a small
order-preserving document model with an encoder that reproduces `yq -o json`'s
layout (2-space indent, every element on its own line, no HTML escaping,
numbers verbatim). The original plan named `sjson`/`gjson`; that was wrong for
parity — they preserve the file's *existing* formatting, whereas `yq -o json`
rewrites the whole document into its own style, so a first `init` on the
template would have produced different bytes. `internal/config/parity_test.go`
checks the seeding functions against the bash originals under a pinned `HOME`.

Note the deliberate quirk already documented in the bash: `//` treats `false` as
absent, so `cursor.cli.network.useHttp1ForAgent` gets a separate `has()` check.
Preserve that — a naive `Exists()` port silently changes behavior for a user who
set it to `false`.

`devcontainer.json` is JSONC (comments + `__PLACEHOLDER__` marker lines) and is
edited line-wise on purpose. Keep the line-oriented approach in
`internal/devcontainer`; it is the correct tool for a commented template and a
JSONC parser would be a regression.

### 3.3 `${HOME}` does not exist on Windows

Generated compose files hardcode `${HOME}` in bind-mount sources
(`${HOME}/.claude/agents:...`), which Docker Compose interpolates from the host
environment. On Windows that variable is unset — it is `%USERPROFILE%`. Native
Windows support, one of the three drivers, therefore is not free: it needs a
decision in `internal/compose` between

- (a) emitting an absolute, already-resolved host path on Windows, or
- (b) writing a `.env` next to the compose file that defines `HOME`.

(b) keeps the generated YAML identical across platforms and is the
recommendation, but it is a behavior change and needs its own test. Related
Windows items: `lib/path.bash:get_file_mtime` branches on `$OSTYPE` (use
`os.Stat` — free), `open_editor`'s `open`/`vi` fallback needs a Windows arm, and
`lib/cache.bash` shells out to `alpine du` through Docker (already
platform-independent).

---

## 4. Milestones

Each milestone ends green: `go build ./...`, `go test ./...`, and — from M3
onward — a clean differential run.

| # | Milestone | Contents |
|---|---|---|
| 1 | **Scaffold** | `go.mod`, `embed.go`, `cmd/sandcat`, cobra tree with every command stubbed, `internal/log`, `internal/prompt`, version banner behavior |
| 2 | **Pure logic** | `internal/stacks`, `agents`, `rtk`, `gitignore`, `project`. No I/O beyond files. Table-driven tests ported from the corresponding `.bats` |
| 3 | **Generation** | `internal/devbox`, `internal/devcontainer`, `internal/compose`. This is where §3.1 lives. Golden files for every template output |
| 4 | **`init`** | `internal/config` + the `init` command: flags, prompts, orchestration, next-steps output. Differential harness goes green across the option matrix |
| 5 | **Docker commands** | `internal/dockercli` + `run`, `compose`, `attach`, `destroy`, `proxy`, `restart-proxy`, `cache`, `edit` |
| 6 | **Landing** | See §4.1 — replaces the original single "cutover" step |

Milestones 2 and 3 are independent and could be parallelized; 4 depends on both.

### 4.1 Milestone 6, revised: land first, remove bash last

The original plan had one big cutover that deleted `cli/` immediately. That is
the wrong order, decided 2026-09-14. The bash tree is the *oracle* — every
parity test and `scripts/difftest.sh` run against it — and the two pieces of
work still queued (an upstream sync and the podman engine) are exactly the
kind of change the oracle exists to catch. So bash stays until both are done.
This is not upstreaming work; the fork is the home for it.

| Step | What | Gate |
|---|---|---|
| 6a | **Land `go-port` in the fork** with bash and Go coexisting: bash under `cli/`, Go under `cmd/` + `internal/`, templates still at `cli/templates/`. Add CI running `go test ./...` and `scripts/difftest.sh`. | PR merged; CI green |
| 6b | **Sync upstream into the bash tree.** `tools/sandcat` is at `9f779f6`, 23 commits past our base `c16d8fd`; 22 files / ~900 lines touch `cli/lib`, `cli/libexec`, `cli/templates`. Merge `upstream/master`, re-run `scripts/dump-bash-blocks.sh`, then let the harness list every generated-file drift and port each one to Go. | difftest 27/27 again at the new base |
| 6c | **Redo the podman engine work in Go**, using `claude/docker-podman-migration-pmipfa` as the reference (its `engine.bash` and netns templates), not by merging it. Template-side changes land in `cli/templates/` and flow through the embed; `engine.bash` becomes `internal/dockercli` growing an engine abstraction. | harness green with both engines |
| 6d | **Release tooling and install story** — §4.2. Retire `install.sh`'s clone-the-tree model; README install section rewritten. | first tagged release installs cleanly on Linux/macOS/Windows |
| 6e | **Remove bash.** Delete `cli/lib`, `cli/libexec`, `cli/bin`, the bats submodules and `scripts/difftest.sh`; move `cli/templates/` to `internal/templates/assets/` and fold `embed.go` into it. The bash-backed parity tests are written to skip when `cli/` is absent, so nothing else changes. Replace them with committed golden files owned by the Go tree. | `go test ./...` green with no bash on the machine |
| 6f | **Relax byte parity** (only after 6e). Once no oracle demands it, the yq-shaped output is a liability: `stripBlankBeforeIndented`, the foot-comment rendering of disabled mounts, and `jsonfile`'s yq layout can all be reconsidered on their merits. Any such change regenerates the goldens deliberately. | — |

6e and 6f are not scheduled; they happen when the bash tree stops earning its
keep.

### 4.2 Install and update story for a binary

`install.sh` today clones the git tree and symlinks `cli/bin/sandcat`; updates
are `git pull`. None of that survives a binary, and the decision is to not
carry a `curl | sh` installer forward at all unless demand appears.

Tiers, in the order they should be built. Everything downstream consumes the
first one.

1. **GitHub Releases via goreleaser.** Cross-compiled archives per OS/arch,
   `checksums.txt`, version stamped through `-ldflags -X …/internal/version.Version`.
   This is the only tier that has to exist for the first release.
2. **`go install github.com/jehoctor/snadcat/cmd/sandcat@latest`** — free once
   the module builds, and what Go-literate users reach for first. Note it
   yields an unstamped binary; `version` falls back to the VCS build info, which
   is fine.
3. **Package managers goreleaser publishes natively:** Homebrew tap, winget
   manifest, Scoop, Snap, AUR, `nfpm` deb/rpm, and a Docker image. One config,
   one CI job. This is where "updating" lives: the user's package manager does
   it, and sandcat never needs a self-updater (which would fight the package
   manager anyway).
4. **PyPI and npm** are wrapper packages, not ports: a per-platform wheel that
   contains the release binary (the `ruff`/`uv` pattern), and an npm package
   whose `postinstall` fetches the matching binary from Releases. Both are
   built *from* tier 1 artifacts. Worth doing because they reach users who
   have `pip`/`npm` but no Homebrew.
5. **Flatpak is a poor fit** and should be last, if ever: sandcat's whole job
   is driving the host's docker/podman socket, which is what Flatpak's sandbox
   exists to prevent. It would need `--filesystem=host` and socket holes that
   defeat the format.

Two small things the binary should grow to make this work well:
`sandcat version --check` (compare against the latest release tag; print a
hint, never download), and a documented `SANDCAT_NO_UPDATE_CHECK` opt-out.

---

## 5. Testing strategy

The bats suite does not translate — it asserts on shell function behavior. But
it is still the best available specification of intended behavior, so it gets
used twice: as a checklist when writing Go tests, and as a live oracle.

**Differential harness (the primary parity tool).** Because the bash CLI and the
Go CLI live in the same tree and read the same templates until Milestone 6, we
can run both and diff:

```
scripts/difftest.sh
  for each combination of (agent × ide × stacks × proxy × secret-provider × features):
    HOME=$tmp/home-bash  cli/bin/sandcat init --agent ... --path $tmp/proj-bash
    HOME=$tmp/home-go    ./sandcat        init --agent ... --path $tmp/proj-go
    diff -ru $tmp/proj-bash $tmp/proj-go
    diff -ru $tmp/home-bash $tmp/home-go   # catches user-settings drift
```

3 agents × 3 IDEs × a handful of stack sets × 2 proxy modes × 3 secret providers
is a few hundred runs — cheap, since `init` only writes files. Non-interactive
mode is already reachable: every prompt has a corresponding flag, and
`--features` / `--stacks` suppress the interactive paths.

**Go tests.** Table-driven unit tests for `stacks`/`agents`/`rtk` (pure data),
golden-file tests for `compose`/`devcontainer`/`devbox` output, and
`testing/fstest` or temp dirs for the file-touching paths. `internal/dockercli`
gets an interface so command construction is assertable without a daemon —
that's what the bats-mock tests currently cover.

---

## 6. Risks and open items

- ~~**The docker/podman work on another branch conflicts with this.**~~
  Decided 2026-09-14: the branch is *not* merged. It is redone in Go as step
  6c, with the bash branch as the reference, after the upstream sync (6b). Its
  `engine.bash` maps onto an engine abstraction in `internal/dockercli`; its
  template changes land in `cli/templates/` and flow through the embed.
- ~~**`install.sh` is 10KB of behavior.**~~ Retired rather than replaced —
  §4.2. Its env-var surface (`SANDCAT_REF`, `SANDCAT_HOME`, `SANDCAT_BIN_DIR`)
  describes a clone-the-tree install that a binary makes meaningless. The
  README's install section is rewritten in 6d, not preserved.
- ~~**Byte-for-byte YAML parity may not be fully reachable** in yaml.v3.~~
  Retired in Milestone 3 — see §3.1. Parity holds across the option matrix,
  including quoting of `${HOME}` entries and the em-dashes in template
  comments.
- **`sandcat compose` passthrough** is easy to get subtly wrong under cobra;
  test it explicitly with args that look like sandcat's own flags.
- **Bash 3.2 compatibility comments throughout** (`lib/compat.bash`'s `mapfile`
  shim, `stacks.bash` using case functions instead of associative arrays) become
  dead weight. Do not port them; they are workarounds for the thing being
  removed.
