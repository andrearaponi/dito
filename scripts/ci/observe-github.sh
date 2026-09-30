#!/usr/bin/env bash
# Observes the CI pipeline on GitHub after the branch has been pushed
# (S-02, tasks 7.3-7.5). Requires an authenticated gh CLI.
#
# Usage: scripts/ci/observe-github.sh runs|schedule|renovate
#   runs      latest ci.yml runs for a pull request to main and for a push to
#             main succeeded, took at most OBSERVE_MAX_MINUTES (default 15),
#             and the push run used the Go version of go.mod (R1, NFR1)
#   schedule  a scheduled ci.yml run succeeded in the last 8 days (R3.AC4)
#   renovate  Renovate opened at least one pull request and the ci.yml run on
#             its latest branch succeeded (R7, observed)
set -uo pipefail
cd "$(dirname "$0")/../.."

mode=${1:-}
max_minutes=${OBSERVE_MAX_MINUTES:-15}
go_version=$(awk '/^go /{print $2; exit}' go.mod)

fail() {
	echo "observe $mode: FAIL: $*" >&2
	exit 1
}

command -v gh >/dev/null 2>&1 || fail "the gh CLI is not installed"
gh auth status >/dev/null 2>&1 || fail "the gh CLI is not authenticated"

# latest_run <event> [branch]: "id conclusion duration-seconds age-seconds" of
# the latest completed ci.yml run, or nothing.
latest_run() {
	local event=$1 branch=${2:-}
	set -- --workflow ci.yml --event "$event" --status completed --limit 1 \
		--json databaseId,conclusion,createdAt,updatedAt
	if [ -n "$branch" ]; then set -- "$@" --branch "$branch"; fi
	gh run list "$@" --jq '.[0] // empty |
		"\(.databaseId) \(.conclusion) \((.updatedAt | fromdateiso8601) - (.createdAt | fromdateiso8601)) \(now - (.createdAt | fromdateiso8601) | floor)"'
}

# check_run <label> <event> [branch]: the run exists, succeeded and was fast enough.
check_run() {
	local label=$1 run id conclusion duration
	shift
	run=$(latest_run "$@") || fail "gh run list failed"
	[ -n "$run" ] || fail "no completed ci.yml run for $label"
	read -r id conclusion duration _ <<EOF
$run
EOF
	[ "$conclusion" = "success" ] || fail "ci.yml run $id for $label concluded with $conclusion"
	[ "$duration" -le $((max_minutes * 60)) ] ||
		fail "ci.yml run $id for $label took $((duration / 60)) minutes (limit $max_minutes)"
	echo "observe $mode: $label run $id succeeded in $((duration / 60))m$((duration % 60))s"
	last_run_id=$id
}

case "$mode" in
runs)
	check_run "a pull request" pull_request
	check_run "a push to main" push main
	gh run view "$last_run_id" --log 2>/dev/null | grep -q "tools-check: ok (go$go_version)" ||
		fail "the push run $last_run_id did not report tools-check with go$go_version"
	echo "observe runs: ok"
	;;
schedule)
	run=$(latest_run schedule) || fail "gh run list failed"
	[ -n "$run" ] || fail "no completed scheduled ci.yml run yet"
	read -r id conclusion _ age <<EOF
$run
EOF
	[ "$conclusion" = "success" ] || fail "scheduled run $id concluded with $conclusion"
	[ "$age" -le $((8 * 24 * 3600)) ] || fail "the latest scheduled run $id is older than 8 days"
	echo "observe schedule: ok (run $id)"
	;;
renovate)
	branch=$(gh pr list --author "app/renovate" --state all --limit 1 --json headRefName --jq '.[0].headRefName // empty') ||
		fail "gh pr list failed"
	[ -n "$branch" ] || fail "Renovate has not opened any pull request yet"
	run=$(gh run list --workflow ci.yml --branch "$branch" --status completed --limit 1 \
		--json databaseId,conclusion --jq '.[0] // empty | "\(.databaseId) \(.conclusion)"') || fail "gh run list failed"
	[ -n "$run" ] || fail "no completed ci.yml run on the Renovate branch $branch"
	read -r id conclusion <<EOF
$run
EOF
	[ "$conclusion" = "success" ] || fail "ci.yml run $id on the Renovate branch $branch concluded with $conclusion"
	echo "observe renovate: ok (branch $branch, run $id)"
	;;
*)
	echo "usage: $0 runs|schedule|renovate" >&2
	exit 2
	;;
esac
