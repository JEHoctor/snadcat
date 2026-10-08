# snadcat

**snadcat** is a fork of [VirtusLab/sandcat](https://github.com/VirtusLab/sandcat) that
rewrites the CLI in Go as a single static binary. It sets up a sandboxed
[dev container](https://containers.dev) for running AI coding agents (Claude Code,
Cursor CLI, Codex CLI, GitHub Copilot CLI) with controlled network access and transparent
secret substitution, while keeping the convenience of working in an IDE like VS Code.

All container traffic is routed through a transparent
[mitmproxy](https://mitmproxy.org/), capturing HTTP/S, DNS, and all other TCP/UDP traffic
without per-tool proxy configuration. An allow/deny list engine controls which requests
go through, and a secret substitution system injects credentials at the proxy level so the
container never sees real values.

> ### Status: early, and changing underneath you
>
> `v0.0.x` is published so the binary can be installed and used, not because the design
> has settled. Specifically:
>
> * **Docker support is going away.** These versions drive `docker compose`; the next
>   significant change replaces that with podman and removes docker support outright.
>   If you need docker, use [upstream sandcat](https://github.com/VirtusLab/sandcat).
> * **No upgrade path yet.** `snadcat init` records none of your choices, so re-running
>   it after an upgrade means re-supplying every flag, and hand-edits to generated files
>   are lost. A manifest is planned
>   ([plan](plans/2026-09-21-file-ownership-and-upgrades.md)).
> * **The `docs/` site documents upstream's bash tool**, not this one — wrong install
>   instructions, wrong command names, wrong config paths. Treat this README as the only
>   current documentation.
>
> The bash CLI is still in the tree under `cli/` as a parity oracle; it keeps the name
> `sandcat`. The two tools are installable side by side: snadcat keeps its own state in
> `.snadcat/`, `~/.config/snadcat/`, `SNADCAT_*` and `snadcat-cache-*` volumes.

## Installation

Requires `docker` (with `docker compose`). Unlike the bash CLI, no `yq` or `jq` on the
host — the binary does that work itself.

**Pre-built binary** (Linux, macOS, Windows; amd64 and arm64). Download from
[Releases](https://github.com/jehoctor/snadcat/releases), verify, and put it on your
`PATH`:

```bash
VERSION=0.0.1   # see Releases for the current tag
OS=linux ARCH=amd64
BASE="https://github.com/jehoctor/snadcat/releases/download/v${VERSION}"
curl -fsSLO "${BASE}/snadcat_${VERSION}_${OS}_${ARCH}.tar.gz"
curl -fsSLO "${BASE}/checksums.txt"
sha256sum --check --ignore-missing checksums.txt
tar xzf "snadcat_${VERSION}_${OS}_${ARCH}.tar.gz" snadcat
install -Dm755 snadcat ~/.local/bin/snadcat
```

**With Go:**

```bash
go install github.com/jehoctor/snadcat/cmd/snadcat@latest
```

Package-manager installs (Homebrew, winget, Scoop, Snap, deb/rpm) are
[planned](plans/2026-08-07-go-port.md), not yet published. Updating means fetching a newer
release; there is no self-updater, and `install.sh` in this repo installs the **bash** CLI,
not snadcat.

## Quick start

```bash
# 1. Initialize the sandbox in your project
snadcat init --agent claude --ide vscode --stacks "python,node" --name myproject

# 2. Add API keys to ~/.config/snadcat/settings.json, then run
snadcat run          # or reopen the project as a dev container
```

`snadcat init` with no flags prompts for every choice. `snadcat --help` lists the
commands; `snadcat <command> --help` documents its flags. Project network rules live in
`.snadcat/settings.json`, per-machine secrets in `.snadcat/settings.local.json`.

## Repository layout

* `cmd/snadcat`, `internal/` — the snadcat CLI (Go)
* `plans/` — design documents, named by the date they were written
* `cli/` — the original bash `sandcat` CLI, retained as the parity oracle until the Go
  port is self-sufficient
* `cli/templates/devcontainer/sandcat/` — reusable proxy definitions:
  `Dockerfile.wg-client`, `compose-proxy.yml`, `compose-agent.yml`, and the
  `scripts/` that perform network filtering & secret substitution
* `cli/templates/devcontainer/` — template application and dev container
  configuration (`Dockerfile.app`, `compose-all.yml`, `devcontainer.json`),
  fine-tuned per project and development stack
* `images/` — sources of the published mitmproxy images (1Password and
  Proton Pass variants)
* `docs/` — upstream's documentation site (Sphinx + MyST); describes the **bash** tool and is currently stale for snadcat

snadcat can be used as a devcontainer setup, or standalone, providing a shell for secure
development.

## Development

```bash
go test ./...                                      # Go tests, including bash-parity tests
go build -o snadcat ./cmd/snadcat && scripts/difftest.sh   # end-to-end parity harness
```

`CLAUDE.md` describes the layout and the parity tooling; `plans/2026-08-07-go-port.md` is
the plan of record for the rewrite. For the bash CLI's own tests (bats), the mitmproxy
addon tests (pytest) and shellcheck, see
[Development](docs/project/development.md).

## Upstream

Upstream sandcat is part of [Visdom](https://virtuslab.com/services/visdom), VirtusLab's
AI-native SDLC platform, and is documented at
[sandcat.virtuslab.com](https://sandcat.virtuslab.com). VirtusLab offers commercial
services around AI-assisted software development — [contact
them](https://virtuslab.com). This fork is unaffiliated and unsupported.

## Copyright

Copyright (C) 2026 VirtusLab [https://virtuslab.com](https://virtuslab.com).

Portions Copyright (C) 2026 James Hoctor. This repository is a fork; see
[NOTICE](NOTICE) for attribution of the Go CLI rewrite.
