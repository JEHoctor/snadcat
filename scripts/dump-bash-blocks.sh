#!/usr/bin/env bash
# Regenerates the embedded text blocks under internal/agents/blocks/ and
# internal/rtk/blocks/ from the bash originals in cli/lib/.
#
# These blocks are Dockerfile / shell / JSONC fragments that get spliced into
# the generated .devcontainer tree verbatim. They are kept as files rather than
# Go string literals because several contain backticks (so raw string literals
# are out) and all contain significant tabs — transcribing them by hand is how
# byte-level drift gets introduced. The Go packages embed these files; the
# dispatch logic around them is what the port makes typed.
#
# Re-run after any upstream change to cli/lib/agents.bash or cli/lib/rtk.bash;
# a resulting git diff is the signal that upstream moved.
#
# Usage: scripts/dump-bash-blocks.sh   (from anywhere)
set -euo pipefail

root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
export SCT_LIBDIR="$root/cli/lib"

# shellcheck source=../cli/lib/agents.bash
source "$SCT_LIBDIR/agents.bash"
# shellcheck source=../cli/lib/rtk.bash
source "$SCT_LIBDIR/rtk.bash"

agents_dir="$root/internal/agents/blocks"
rtk_dir="$root/internal/rtk/blocks"
rm -rf "$agents_dir" "$rtk_dir"
mkdir -p "$agents_dir" "$rtk_dir"

# Only multi-line blocks are dumped to files. Single-token values (extension
# ids, env var names, one-line help strings) stay as Go constants, where they
# are easier to read and review than a directory of one-line files.
for agent in $(sct_available_agents); do
	# The agent halves are captured with rtk disabled so the Go side can
	# compose agent + rtk itself and be tested on each half independently.
	SANDCAT_RTK=false sct_agent_docker_install_block "$agent" \
		>"$agents_dir/docker-install-$agent.txt"
	SANDCAT_RTK=false sct_agent_user_init_block "$agent" \
		>"$agents_dir/user-init-$agent.txt"

	sct_agent_docker_home_prep_block "$agent" >"$agents_dir/home-prep-$agent.txt"
	sct_agent_devcontainer_settings_block "$agent" \
		>"$agents_dir/devcontainer-settings-$agent.txt"

	SANDCAT_RTK=true sct_rtk_user_init_block "$agent" >"$rtk_dir/user-init-$agent.txt"
done

SANDCAT_RTK=true sct_rtk_docker_install_block >"$rtk_dir/docker-install.txt"

# devbox_dockerfile_block is an *unquoted* heredoc: it interpolates the
# single-line jq merge program and relies on \$ escapes throughout, so the
# rendered text is the only sensible thing to carry over.
# shellcheck source=../cli/lib/devbox.bash
source "$SCT_LIBDIR/devbox.bash"
devbox_dir="$root/internal/devbox/blocks"
rm -rf "$devbox_dir"
mkdir -p "$devbox_dir"
devbox_dockerfile_block >"$devbox_dir/dockerfile.txt"
write_devbox_tools_json "$devbox_dir/tools.json"

# Agents that contribute no block emit either nothing (`return 0`) or a bare
# newline (`echo ""`). Callers wrap every one of these in $(...), which strips
# trailing newlines, so both forms mean "no block". Drop those files rather
# than embedding empty payloads; the Go side returns "" when a block is absent.
for f in "$agents_dir"/*.txt "$rtk_dir"/*.txt "$devbox_dir"/*; do
	[[ -e "$f" ]] || continue
	if [[ -z "$(cat -- "$f")" ]]; then
		rm -- "$f"
	fi
done

echo "regenerated blocks under internal/{agents,rtk,devbox}/blocks/" >&2
