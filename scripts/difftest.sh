#!/usr/bin/env bash
# Differential harness: runs `sandcat init` through both the bash CLI and the
# Go binary over a matrix of options, each under its own throwaway HOME and
# project directory, and diffs everything either one wrote.
#
# This is the end-to-end parity oracle from GO-PORT-PLAN.md §5. The per-package
# parity tests cover each generator in isolation; this covers the whole init
# flow including user settings, host config pre-creation, and .gitignore.
#
# Usage: scripts/difftest.sh [path-to-go-binary]   (default: ./sandcat)
set -euo pipefail

root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
go_bin="${1:-$root/sandcat}"
bash_bin="$root/cli/bin/sandcat"

[[ -x "$go_bin" ]] || { echo "Go binary not found at $go_bin (go build -o sandcat ./cmd/sandcat)" >&2; exit 2; }
[[ -x "$bash_bin" ]] || { echo "bash CLI not found at $bash_bin" >&2; exit 2; }

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

fail=0
pass=0

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
	if ! env -i HOME="$dir/home-go" PATH="$PATH" GIT_CONFIG_NOSYSTEM=1 "${env[@]}" \
		"$go_bin" init --path "$dir/proj-go" "${common[@]}" >"$dir/go.log" 2>&1; then
		echo "FAIL $name: go init failed:" >&2
		sed 's/^/    /' "$dir/go.log" >&2
		fail=$((fail + 1))
		return
	fi

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

for agent in claude cursor codex; do
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
run_case all-features  -- --agent cursor --ide jetbrains --stacks scala --features tui,no-shared-cache,no-gitignore,no-rtk --secret-provider protonpass

run_case env-git-ro    SANDCAT_MOUNT_GIT_READONLY=true -- --agent claude --ide vscode --stacks "" --features "" --secret-provider none
run_case env-no-agent-cfg SANDCAT_MOUNT_CURSOR_CONFIG=false -- --agent cursor --ide vscode --stacks "" --features "" --secret-provider none
run_case env-rtk-off   SANDCAT_RTK=false -- --agent codex --ide none --stacks "" --features "" --secret-provider none
run_case env-gitignore-off SANDCAT_GITIGNORE=false -- --agent claude --ide vscode --stacks "" --features "" --secret-provider none

echo
echo "passed: $pass  failed: $fail"
[[ $fail -eq 0 ]]
