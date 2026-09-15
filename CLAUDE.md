# snadcat — agent notes

This is James Hoctor's **fork** of [VirtusLab/sandcat](https://github.com/VirtusLab/sandcat),
a Docker/dev-container sandbox for running AI coding agents. The fork is where the CLI is
being rewritten from bash to Go; that work is not intended for upstream. Machine-level
facts (credentials, tooling, commit convention) live in `~/Projects/CLAUDE.md` and apply
here too.

## Branches and remotes

| Ref | Role |
|---|---|
| `main` | **The fork's default branch.** All work lands here via PR. |
| `master` | Tracks `upstream/master` for reference only. Never commit to it; fast-forward it with `git fetch upstream && git branch -f master upstream/master && git push origin master`. |
| `origin` | `github.com/jehoctor/snadcat` — the fork. Default branch `main`. |
| `upstream` | `github.com/VirtusLab/sandcat` — "old upstream". Default branch `master`. Read-only; we do not open PRs there. |

`main` and `master` diverge on purpose: `main` carries the Go port, `master` mirrors
upstream so `git log main..master` shows exactly what upstream has done since the fork's
base (`c16d8fd`). Upstream changes are brought into `main` deliberately as part of the
port's Milestone 6b (see below), not by merging `master` wholesale.

## The Go port

`GO-PORT-PLAN.md` is the plan of record — read it before touching either CLI. Short form:

- `cmd/sandcat` + `internal/` is the Go CLI. `cli/` is the original bash CLI, kept as the
  **parity oracle** until Milestone 6e. Both read the same templates from `cli/templates/`
  (embedded into the Go binary via `embed.go` at the repo root).
- Text fragments the bash emits from heredocs are checked in under `internal/*/blocks/`.
  Regenerate them after any change to `cli/lib/{agents,rtk,devbox}.bash` with
  `scripts/dump-bash-blocks.sh`; a resulting `git diff` is the signal that the Go side
  needs a matching change.
- Parity tests shell out to the bash originals and compare bytes; they skip themselves
  when `cli/` is gone. Run everything with:
  ```
  go test ./...
  go build -o sandcat ./cmd/sandcat && scripts/difftest.sh   # end-to-end; needs bash, yq, jq (no docker)
  ```
  `difftest.sh` runs `sandcat init` through both CLIs across 27 option combinations under
  pinned `HOME`s and diffs both the project and home trees. It must stay at 27/27 until
  bash is removed.
- The binary on `PATH` (`~/.local/bin/sandcat`) is the *upstream bash* checkout under
  `~/Projects/tools/sandcat`, not this repo. Use `./sandcat` (built above) or `go run
  ./cmd/sandcat` to exercise the port.

## Layout notes

- `.claude/worktrees/` holds git worktrees (currently `go-port`); it is not source.
  Exclude it from searches and linters.
- `UPSTREAM-NOTES.md` is untracked scratch about upstream issues found while using the
  bash CLI; it is not part of the port.
- The bats submodules under `cli/support/` are only needed to run the bash test suite
  (`cli/run-tests.bash`); the Go tests do not depend on them.
