#!/usr/bin/env bash
# Coverage report and floors (S-02, R5).
#
#   1. Runs the tests with coverage: each package is measured with its own
#      tests only, so its floor depends on nothing else.
#   2. Prints a table with the coverage of every package and the total, and
#      appends it to the job summary when GITHUB_STEP_SUMMARY is set (R5.AC1).
#   3. Fails if a package is below its floor in scripts/ci/coverage-floors.txt
#      (R5.AC2).
#   4. Fails if a floor is lower than on COVERAGE_BASE, or was removed while its
#      package still exists (R5.AC3). Skipped when the base has no floors file.
set -uo pipefail
cd "$(dirname "$0")/../.."

floors=scripts/ci/coverage-floors.txt
base=${COVERAGE_BASE:-main}
tmp=$(mktemp -d "${TMPDIR:-/tmp}/dito-coverage.XXXXXX") || exit 1
trap 'rm -rf "$tmp"' EXIT

fail=0
report() {
	echo "coverage: $*" >&2
	fail=1
}

# floor_of <package> <file>: prints the floor declared for the package, if any.
floor_of() {
	awk -v pkg="$1" '$1 == pkg { print $2; exit }' "$2"
}

# The e2e suite (package dito/e2e, test files only) runs separately: it has
# no statements of its own to measure.
pkgs=$(go list ./... | grep -v '/e2e$')
# shellcheck disable=SC2086 # one argument per package
if ! go test -count=1 -cover -coverprofile="$tmp/cover.out" $pkgs >"$tmp/test.out" 2>&1; then
	cat "$tmp/test.out" >&2
	echo "coverage: FAIL (tests failed)" >&2
	exit 1
fi

# "ok  dito/app  0.5s  coverage: 47.4% of statements" -> "dito/app 47.4"
awk '{
	pkg = ""; cov = ""
	for (i = 1; i <= NF; i++) {
		if (pkg == "" && $i ~ /^dito(\/|$)/) pkg = $i
		if ($i == "coverage:") cov = $(i + 1)
	}
	if (pkg != "" && cov ~ /^[0-9]/) { sub(/%$/, "", cov); print pkg, cov }
}' "$tmp/test.out" | sort >"$tmp/measured"
total=$(go tool cover -func="$tmp/cover.out" | awk '/^total:/ { print $NF }')

grep -v '^#' "$floors" | awk 'NF >= 2' >"$tmp/floors"

{
	echo "| Package | Coverage | Floor |"
	echo "| --- | --- | --- |"
	while read -r pkg cov; do
		floor=$(floor_of "$pkg" "$tmp/floors")
		echo "| $pkg | $cov% | ${floor:+$floor%} |"
	done <"$tmp/measured"
	echo "| **total** | $total | |"
} >"$tmp/table.md"
cat "$tmp/table.md"
if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
	{
		echo "### Coverage"
		cat "$tmp/table.md"
	} >>"$GITHUB_STEP_SUMMARY"
fi

# R5.AC2: every package must be at or above its floor.
while read -r pkg floor; do
	cov=$(floor_of "$pkg" "$tmp/measured")
	if [ -z "$cov" ]; then
		report "$pkg has a floor but no longer exists: remove its line from $floors"
		continue
	fi
	if awk -v c="$cov" -v f="$floor" 'BEGIN { exit !(c + 0 < f + 0) }'; then
		report "$pkg is below floor: $cov% < $floor%"
	fi
done <"$tmp/floors"
while read -r pkg _; do
	[ -n "$(floor_of "$pkg" "$tmp/floors")" ] || echo "coverage: warning: $pkg has no floor (treated as 0%)" >&2
done <"$tmp/measured"

# R5.AC3: floors can only go up.
if git rev-parse --verify --quiet "$base^{commit}" >/dev/null && git cat-file -e "$base:$floors" 2>/dev/null; then
	git show "$base:$floors" | grep -v '^#' | awk 'NF >= 2' >"$tmp/base-floors"
	while read -r pkg base_floor; do
		current=$(floor_of "$pkg" "$tmp/floors")
		if [ -z "$current" ]; then
			if [ -n "$(floor_of "$pkg" "$tmp/measured")" ]; then
				report "floor lowered for $pkg: removed while the package still exists (was $base_floor% on $base)"
			fi
		elif [ "$current" -lt "$base_floor" ]; then
			report "floor lowered for $pkg: $base_floor% on $base, $current% now"
		fi
	done <"$tmp/base-floors"
else
	echo "coverage: $base has no $floors yet, floor ratchet skipped"
fi

if [ "$fail" -ne 0 ]; then
	echo "coverage: FAIL" >&2
	exit 1
fi
echo "coverage: ok (total $total)"
