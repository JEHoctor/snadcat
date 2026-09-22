# snadcat — agent notes

**snadcat** is a fork of [VirtusLab/sandcat](https://github.com/VirtusLab/sandcat), a
dev-container sandbox for running AI coding agents. The fork rewrites the CLI in Go and is
diverging on purpose (it is not intended for upstream). Naming: the Go tool and binary are
`snadcat`; the original bash CLI under `cli/` keeps the name `sandcat`. The two are meant
to coexist on one machine, so snadcat's whole host-side surface is its own: `.snadcat/`,
`~/.config/snadcat/`, `SNADCAT_*` variables, `snadcat-cache-*` volumes, and the
`# Snadcat` gitignore block. What stays `sandcat` is internal to the generated tree and
shared with the bash templates (`.devcontainer/sandcat/`, `sandcat.env`, in-container
script names) until the bash tree is removed. The parity tests and harness map
snadcat's names back to sandcat's before diffing (`internal/testutil.Normalize`,
`normalize_tree` in `scripts/difftest.sh`); that is sound because the shared templates
never contain the string `snadcat`.

## Branches

| Branch | Role |
|---|---|
| `main` | **The default branch.** All work lands here via PR. |
| `master` | Mirrors upstream VirtusLab `master` (their default branch) for reference only. Never commit to it; fast-forward it to upstream when syncing. |

`main` and `master` diverge on purpose: `main` carries the Go port, `master` mirrors
upstream so `git log main..master` shows exactly what upstream has done since the fork's
base (`c16d8fd`). Upstream changes are brought into `main` deliberately as a sync step of
the port plan, not by merging `master` wholesale.

## Plans

Design and planning documents live in `plans/`, named `YYYY-MM-DD-<topic>.md` with the date
the plan was formulated, so they sort chronologically. Read the relevant plan before
touching the area it covers. The Go port's plan of record is
`plans/2026-08-07-go-port.md`.

## The Go port, in short

- `cmd/snadcat` + `internal/` is the Go CLI. `cli/` is the original bash CLI, kept as the
  **parity oracle** until the plan's bash-removal step. Both read the same templates from
  `cli/templates/` (embedded into the Go binary via `embed.go` at the repo root).
- Text fragments the bash emits from heredocs are checked in under `internal/*/blocks/`.
  Regenerate them after any change to `cli/lib/{agents,rtk,devbox}.bash` with
  `scripts/dump-bash-blocks.sh`; a resulting `git diff` is the signal that the Go side
  needs a matching change.
- Parity tests shell out to the bash originals and compare bytes; they skip themselves
  when `cli/` is gone. Run everything with:
  ```
  go test ./...
  go build -o snadcat ./cmd/snadcat && scripts/difftest.sh   # end-to-end; needs bash, yq, jq (no docker)
  ```
  `difftest.sh` runs `init` through both CLIs across a matrix of options under
  pinned `HOME`s and diffs both the project and home trees. It must stay fully green
  until bash is removed.
- **A `sandcat` may already be installed on your machine** (e.g. an upstream checkout on
  `PATH`). Be deliberate about which one you are exercising: the system install, the bash
  CLI in this tree (`cli/bin/sandcat`), or the Go CLI (`./sandcat` from the build above, or
  `go run ./cmd/snadcat`). Parity claims are only meaningful between the two in-tree
  versions at the same commit.
- The bats submodules under `cli/support/` are only needed to run the bash test suite
  (`cli/run-tests.bash`); the Go tests do not depend on them.
