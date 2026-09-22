#!/usr/bin/env bash
# Differential harness: runs `sandcat init` through both the bash CLI and the
# Go binary over a matrix of options, each under its own throwaway HOME and
# project directory, and diffs everything either one wrote.
#
# This is the end-to-end parity oracle from plans/2026-08-07-go-port.md §5. The per-package
# parity tests cover each generator in isolation; this covers the whole init
# flow including user settings, host config pre-creation, and .gitignore.
#
# Usage: scripts/difftest.sh [path-to-go-binary]   (default: ./sandcat)
set -euo pipefail

root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
go_bin="${1:-$root/snadcat}"
bash_bin="$root/cli/bin/sandcat"

[[ -x "$go_bin" ]] || { echo "Go binary not found at $go_bin (go build -o snadcat ./cmd/snadcat)" >&2; exit 2; }
[[ -x "$bash_bin" ]] || { echo "bash CLI not found at $bash_bin" >&2; exit 2; }

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

fail=0
pass=0

# normalize_tree <dir>: map snadcat's naming back onto sandcat's so the Go
# tree can be diffed against the bash one. Renames paths, then rewrites file
# contents. Sound rather than heuristic: the shared templates never contain
# the string "snadcat", so every occurrence originates from the Go tool's own
# naming (.snadcat/, ~/.config/snadcat/, SNADCAT_*, snadcat-cache-*, the
# "# Snadcat" markers).
normalize_tree() {
	local root=$1
	# Deepest paths first so a renamed parent doesn't invalidate child paths.
	local p
	while IFS= read -r -d '' p; do
		local base dirn
		base=$(basename "$p"); dirn=$(dirname "$p")
		case "$base" in
		*snadcat*) mv "$p" "$dirn/${base//snadcat/sandcat}" ;;
		esac
	done < <(find "$root" -depth -name '*snadcat*' -print0)
	find "$root" -type f -print0 | xargs -0 -r sed -i \
		-e 's/SNADCAT_/SANDCAT_/g' -e 's/Snadcat/Sandcat/g' -e 's/snadcat/sandcat/g'
}

# run_case <name> <extra env...> -- <init args...>
run_case() {
	local name=$1
	shift
	local -a env=()
	while [[ $# -gt 0 && $1 != "--" ]]; do
		env+=("$1")
		shift
	done
	shift

	local dir="$work/$name"
	mkdir -p "$dir/home-bash" "$dir/home-go" "$dir/proj-bash" "$dir/proj-go"
	# A .git entry makes the gitignore path exercise; the name is derived from
	# the directory, so both sides get --name to keep them equal.
	mkdir "$dir/proj-bash/.git" "$dir/proj-go/.git"
	printf 'node_modules/\n' >"$dir/proj-bash/.gitignore"
	printf 'node_modules/\n' >"$dir/proj-go/.gitignore"

	local -a common=(--name demo-sandbox "$@")

	if ! env -i HOME="$dir/home-bash" PATH="$PATH" GIT_CONFIG_NOSYSTEM=1 "${env[@]}" \
		"$bash_bin" init --path "$dir/proj-bash" "${common[@]}" >"$dir/bash.log" 2>&1; then
		echo "FAIL $name: bash init failed:" >&2
		sed 's/^/    /' "$dir/bash.log" >&2
		fail=$((fail + 1))
		return
	fi
	# The matrix is written in the bash's SANDCAT_* names; the Go tool reads
	# SNADCAT_*. Map them for the Go run only.
	local -a go_env=()
	local e
	for e in "${env[@]+"${env[@]}"}"; do
		go_env+=("${e/#SANDCAT_/SNADCAT_}")
	done
	if ! env -i HOME="$dir/home-go" PATH="$PATH" GIT_CONFIG_NOSYSTEM=1 "${go_env[@]+"${go_env[@]}"}" \
		"$go_bin" init --path "$dir/proj-go" "${common[@]}" >"$dir/go.log" 2>&1; then
		echo "FAIL $name: go init failed:" >&2
		sed 's/^/    /' "$dir/go.log" >&2
		fail=$((fail + 1))
		return
	fi
	normalize_tree "$dir/proj-go"
	normalize_tree "$dir/home-go"

	local ok=true
	if ! diff -ru "$dir/proj-bash" "$dir/proj-go" >"$dir/proj.diff"; then
		ok=false
		echo "FAIL $name: project tree differs:" >&2
		sed 's/^/    /' "$dir/proj.diff" >&2
	fi
	if ! diff -ru "$dir/home-bash" "$dir/home-go" >"$dir/home.diff"; then
		ok=false
		echo "FAIL $name: home tree differs:" >&2
		sed 's/^/    /' "$dir/home.diff" >&2
	fi

	if $ok; then
		pass=$((pass + 1))
		echo "ok   $name"
	else
		fail=$((fail + 1))
	fi
}

for agent in claude cursor codex copilot; do
	for ide in vscode jetbrains none; do
		run_case "$agent-$ide" -- --agent "$agent" --ide "$ide" --stacks "" --features "" --secret-provider none
	done
done

for stacks in node java scala "python,go,rust" "scala,node,dotnet"; do
	run_case "stacks-${stacks//,/+}" -- --agent claude --ide vscode --stacks "$stacks" --features "" --secret-provider none
done

run_case tui           -- --agent claude --ide vscode --stacks "" --features tui --secret-provider none
run_case proxy-flag    -- --agent claude --ide vscode --stacks "" --features "" --proxy tui --secret-provider none
run_case 1password     -- --agent claude --ide vscode --stacks "" --features "" --secret-provider 1password
run_case 1password-old -- --agent claude --ide vscode --stacks "" --features "" --1password
run_case protonpass    -- --agent codex --ide vscode --stacks "" --features "" --sp protonpass
run_case no-rtk        -- --agent claude --ide vscode --stacks "" --features no-rtk --secret-provider none
run_case no-gitignore  -- --agent claude --ide vscode --stacks "" --features no-gitignore --secret-provider none
run_case no-cache      -- --agent claude --ide vscode --stacks java --features no-shared-cache --secret-provider none
run_case strict-net    -- --agent claude --ide vscode --stacks python,java --features strict-network --secret-provider none
run_case strict-empty  -- --agent codex --ide none --stacks "" --features strict-network --secret-provider none
run_case all-features  -- --agent cursor --ide jetbrains --stacks scala --features tui,no-shared-cache,no-gitignore,no-rtk,strict-network --secret-provider protonpass
run_case env-strict    SANDCAT_STRICT_NETWORK=true -- --agent claude --ide jetbrains --stacks go --features "" --secret-provider none

run_case env-git-ro    SANDCAT_MOUNT_GIT_READONLY=true -- --agent claude --ide vscode --stacks "" --features "" --secret-provider none
run_case env-no-agent-cfg SANDCAT_MOUNT_CURSOR_CONFIG=false -- --agent cursor --ide vscode --stacks "" --features "" --secret-provider none
run_case env-rtk-off   SANDCAT_RTK=false -- --agent codex --ide none --stacks "" --features "" --secret-provider none
run_case env-gitignore-off SANDCAT_GITIGNORE=false -- --agent claude --ide vscode --stacks "" --features "" --secret-provider none

echo
echo "passed: $pass  failed: $fail"
[[ $fail -eq 0 ]]
