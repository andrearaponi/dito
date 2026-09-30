#!/usr/bin/env bash
# Policy checks for the GitHub workflows (S-02). actionlint validates syntax
# and expressions; this script enforces the project rules on top of it:
#   R1.AC1/R1.AC2  ci.yml runs on pull requests to main and on pushes to main
#   R3.AC4         ci.yml has a weekly schedule
#   R1.AC3         every actions/setup-go step uses go-version-file: go.mod
#   R1.AC4         every workflow declares read-only permissions, no write
#   R1.AC5         every action is pinned to a full commit SHA
#   R6.AC1         every step of ci.yml runs `make <target>` with an existing target
#   R6.AC2         no tool version outside tools.mk
#   R7             renovate.json: presets, gomodTidyAll, schedule, testdata
#                  ignored, a custom manager matching every tool in tools.mk;
#                  Dockerfile base images pinned by tag and digest
# Every check runs; the script fails at the end if any of them failed.
set -uo pipefail
cd "$(dirname "$0")/../.."

wf=.github/workflows
ci=$wf/ci.yml
fail=0
report() {
	echo "workflows: $*" >&2
	fail=1
}

[ -f "$ci" ] || {
	echo "workflows: missing $ci" >&2
	exit 1
}

# R1.AC1, R1.AC2, R3.AC4: triggers.
awk '/^on:/ { on = 1; next } /^[^ #]/ { on = 0 } on' "$ci" >"${TMPDIR:-/tmp}/dito-ci-on.$$"
for trigger in pull_request push; do
	awk -v t="  $trigger:" '$0 == t { found = 1; next } found && /^  [a-z_]+:/ { exit } found && /branches: \[main\]/ { ok = 1 } END { exit !ok }' \
		"${TMPDIR:-/tmp}/dito-ci-on.$$" || report "ci.yml must run on $trigger events for branch main"
done
grep -qE '^  schedule:' "${TMPDIR:-/tmp}/dito-ci-on.$$" && grep -qE 'cron:' "${TMPDIR:-/tmp}/dito-ci-on.$$" ||
	report "ci.yml must have a scheduled (cron) trigger"
rm -f "${TMPDIR:-/tmp}/dito-ci-on.$$"

for f in "$wf"/*.yml "$wf"/*.yaml; do
	[ -f "$f" ] || continue

	# R1.AC3: toolchain from go.mod.
	setup=$(grep -cE 'uses:[[:space:]]*actions/setup-go@' "$f")
	fromfile=$(grep -cE 'go-version-file:[[:space:]]*go\.mod' "$f")
	[ "$setup" -eq "$fromfile" ] || report "$f: every actions/setup-go step must use go-version-file: go.mod"

	# R1.AC4: read-only permissions.
	grep -qE '^permissions:' "$f" || report "$f: missing top-level permissions block"
	if grep -nE '(:[[:space:]]*write([[:space:]]|$)|write-all)' "$f" >/dev/null; then
		report "$f: write permission granted: $(grep -nE '(:[[:space:]]*write([[:space:]]|$)|write-all)' "$f" | head -1)"
	fi

	# R1.AC5: actions pinned to a commit SHA.
	grep -hE '^[[:space:]]*(-[[:space:]]+)?uses:' "$f" | sed -E 's/^[^:]*uses:[[:space:]]*"?([^"[:space:]]+).*/\1/' |
		while read -r ref; do
			case "$ref" in ./* | docker://*) continue ;; esac
			printf '%s\n' "$ref" | grep -qE '^[^@]+@[0-9a-f]{40}$' || echo "$ref"
		done >"${TMPDIR:-/tmp}/dito-unpinned.$$"
	while read -r ref; do
		report "$f: action not pinned to a commit SHA: $ref"
	done <"${TMPDIR:-/tmp}/dito-unpinned.$$"
	rm -f "${TMPDIR:-/tmp}/dito-unpinned.$$"
done

# R6.AC1: every step of every ci.yml job runs an existing make target.
awk '
	/^jobs:/ { injobs = 1; next }
	injobs && /^[^ #]/ { injobs = 0 }
	injobs && /^  [A-Za-z0-9_-]+:[[:space:]]*$/ { job = $1; sub(/:$/, "", job); jobs[++n] = job; next }
	injobs && /^[[:space:]]+(-[[:space:]]+)?run:/ { cmd = $0; sub(/^[^:]*run:[[:space:]]*/, "", cmd); print job "\t" cmd; runs[job]++ }
	END { for (i = 1; i <= n; i++) if (!(jobs[i] in runs)) print jobs[i] "\t<no run step>" }
' "$ci" >"${TMPDIR:-/tmp}/dito-runs.$$"
while IFS="$(printf '\t')" read -r job cmd; do
	case "$cmd" in
	"make "*)
		target=$(printf '%s\n' "$cmd" | awk '{ print $2 }')
		make -n "$target" >/dev/null 2>&1 || report "job $job runs make $target, but that target does not exist"
		;;
	*) report "job $job does not run a make target: $cmd" ;;
	esac
done <"${TMPDIR:-/tmp}/dito-runs.$$"
rm -f "${TMPDIR:-/tmp}/dito-runs.$$"

# R6.AC2: tool versions only in tools.mk.
if hits=$(grep -rnE '(golangci-lint|govulncheck|actionlint|walden)(/[A-Za-z0-9._/-]*)?@v[0-9]' .github Makefile scripts 2>/dev/null); then
	report "tool version outside tools.mk: $(printf '%s\n' "$hits" | head -1)"
fi

# R7: Renovate configuration and pinned base images.
renovate_fail=0
rreport() {
	report "$*"
	renovate_fail=1
}
r=renovate.json
if [ ! -f "$r" ]; then
	rreport "missing $r"
elif ! jq empty "$r" 2>/dev/null; then
	rreport "$r is not valid JSON"
else
	for preset in config:recommended group:allNonMajor helpers:pinGitHubActionDigests docker:pinDigests; do
		jq -e --arg p "$preset" '(.extends // []) | index($p) != null' "$r" >/dev/null ||
			rreport "$r: missing preset $preset"
	done
	jq -e '(.postUpdateOptions // []) | index("gomodTidyAll") != null' "$r" >/dev/null ||
		rreport "$r: postUpdateOptions must include gomodTidyAll (plugin modules in step with the host, R7.AC4)"
	jq -e '(.schedule // []) | length > 0' "$r" >/dev/null || rreport "$r: missing schedule"
	jq -e '(.ignorePaths // []) | index("**/testdata/**") != null' "$r" >/dev/null ||
		rreport "$r: ignorePaths must include **/testdata/**"
	# The custom manager must match every tool version in tools.mk. jq uses
	# Oniguruma, which supports the named groups of the Renovate regex.
	versions=$(grep -cE '^[A-Z_]+_VERSION := v' tools.mk)
	matched=$(jq -Rs --slurpfile cfg "$r" \
		'. as $text | [$cfg[0].customManagers[]? | .matchStrings[]? as $re | [$text | match($re; "g")] | length] | add // 0' tools.mk)
	[ "$matched" = "$versions" ] ||
		rreport "$r: the tools.mk custom manager matches $matched of $versions tool versions"
fi
awk '/^FROM / { for (i = 2; i <= NF; i++) if ($i !~ /^--/) { print $i; break } }' Dockerfile |
	while read -r image; do
		printf '%s\n' "$image" | grep -qE ':[^@/]+@sha256:[0-9a-f]{64}$' || echo "$image"
	done >"${TMPDIR:-/tmp}/dito-images.$$"
while read -r image; do
	rreport "Dockerfile base image not pinned by tag and digest: $image"
done <"${TMPDIR:-/tmp}/dito-images.$$"
rm -f "${TMPDIR:-/tmp}/dito-images.$$"
[ "$renovate_fail" -eq 0 ] && echo "renovate: ok"

if [ "$fail" -ne 0 ]; then
	echo "workflows: FAIL" >&2
	exit 1
fi
echo "workflows: policy ok"
