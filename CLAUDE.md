# snadcat — agent notes

This is a **fork** of [VirtusLab/sandcat](https://github.com/VirtusLab/sandcat), a
Docker/dev-container sandbox for running AI coding agents. The fork is where the CLI is
being rewritten from bash to Go; that work is not intended for upstream.

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

- `cmd/sandcat` + `internal/` is the Go CLI. `cli/` is the original bash CLI, kept as the
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
  go build -o sandcat ./cmd/sandcat && scripts/difftest.sh   # end-to-end; needs bash, yq, jq (no docker)
  ```
  `difftest.sh` runs `sandcat init` through both CLIs across a matrix of options under
  pinned `HOME`s and diffs both the project and home trees. It must stay fully green
  until bash is removed.
- **A `sandcat` may already be installed on your machine** (e.g. an upstream checkout on
  `PATH`). Be deliberate about which one you are exercising: the system install, the bash
  CLI in this tree (`cli/bin/sandcat`), or the Go CLI (`./sandcat` from the build above, or
  `go run ./cmd/sandcat`). Parity claims are only meaningful between the two in-tree
  versions at the same commit.
- The bats submodules under `cli/support/` are only needed to run the bash test suite
  (`cli/run-tests.bash`); the Go tests do not depend on them.
