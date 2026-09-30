#!/usr/bin/env bash
# Mutation harness for the CI gates (S-02).
#
# Every case runs a make target in a temporary copy of the working tree:
#   1. baseline: the target must pass on the untouched snapshot (cached per
#      target and variables; a case may skip it when it asserts on its output);
#   2. mutation: files are changed and/or make variables are set;
#   3. the target runs again and must fail (or pass, for boundary cases),
#      optionally containing / not containing some output.
# A missing or broken target can therefore never make a case pass.
#
# Usage: scripts/ci/selftest.sh              run SELFTEST_CASES (default: all)
#        scripts/ci/selftest.sh --self-check verify the harness itself
#
# Compatible with the bash 3.2 shipped with macOS.
set -uo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
ROOT=$(cd "$SCRIPT_DIR/../.." && pwd)
TAB=$(printf '\t')
WORK=""
STATE=""
SC_FAKE=""

# Registered cases, in execution order. Each one is a function
# case_<name with underscores> defined below.
ALL_CASES="build-proxy-error build-plugin-error vet-printf modules-untidy modules-go-version-skew modules-shared-version-skew test-failing test-data-race hermetic-external-dial image-broken-dockerfile lint-new-violation lint-legacy-only vuln-reachable smoke-tampered-signature smoke-missing-header coverage-below-floor coverage-floor-lowered coverage-package-removed workflows-tag-ref workflows-write-permission workflows-hardcoded-tool-version workflows-job-without-make renovate-missing-gomodtidyall ci-stops-at-first-failure"

log() { printf '%s\n' "$*"; }

cleanup() {
	[ -n "$WORK" ] && rm -rf "$WORK"
	[ -n "$STATE" ] && rm -rf "$STATE"
	return 0
}

# snapshot <source-dir>: copies tracked and untracked, non-ignored files into
# a new temporary git repository with a single "base" commit.
snapshot() {
	local src=$1
	WORK=$(mktemp -d "${TMPDIR:-/tmp}/dito-selftest.XXXXXX") || return 1
	(
		cd "$src" || exit 1
		git ls-files -z --cached --others --exclude-standard |
			while IFS= read -r -d '' f; do
				if [ -e "$f" ] || [ -L "$f" ]; then printf '%s\0' "$f"; fi
			done |
			tar -cf - --null -T -
	) | tar -xf - -C "$WORK" || return 1
	git -C "$WORK" init -q &&
		git -C "$WORK" add -A &&
		git -C "$WORK" -c user.name=selftest -c user.email=selftest@localhost \
			-c commit.gpgsign=false commit -q -m "selftest base"
}

reset_work() {
	git -C "$WORK" reset -q --hard && git -C "$WORK" clean -qfdx
}

# run_target <log-file> <target> [VAR=value ...]
run_target() {
	local out=$1 target=$2
	shift 2
	make -C "$WORK" --no-print-directory "$target" "$@" >"$out" 2>&1
}

container_engine_available() {
	docker info >/dev/null 2>&1 || podman info >/dev/null 2>&1
}

# baseline_ok <target> [VAR=value ...]: the target passes on the snapshot.
baseline_ok() {
	local key="$*"
	if grep -qxF -- "$key${TAB}0" "$STATE/baselines" 2>/dev/null; then return 0; fi
	if grep -qxF -- "$key${TAB}1" "$STATE/baselines" 2>/dev/null; then return 1; fi
	reset_work || return 1
	if run_target "$STATE/baseline.log" "$@"; then
		printf '%s\t0\n' "$key" >>"$STATE/baselines"
		return 0
	fi
	printf '%s\t1\n' "$key" >>"$STATE/baselines"
	return 1
}

# run_case <name>: returns 0 when the case behaves as declared.
run_case() {
	local name=$1 fn
	fn="case_$(printf '%s' "$1" | tr '-' '_')"
	if ! declare -F "$fn" >/dev/null; then
		log "selftest FAIL: $name (unknown case)"
		return 1
	fi

	C_TARGET="" C_BASE_VARS="" C_VARS="__unset__" C_EXPECT="fail" C_BASELINE="yes"
	C_CONTAINS="" C_NOT_CONTAINS="" C_MUTATE="" C_REQUIRES="" C_EXTRA_CHECK=""
	"$fn"
	[ "$C_VARS" = "__unset__" ] && C_VARS=$C_BASE_VARS

	if [ "$C_REQUIRES" = "container" ] && ! container_engine_available; then
		log "selftest FAIL: $name (not run: no container engine available)"
		return 1
	fi
	if [ "$C_BASELINE" = "yes" ]; then
		# shellcheck disable=SC2086 # variables are space-separated on purpose
		if ! baseline_ok "$C_TARGET" $C_BASE_VARS; then
			log "selftest FAIL: $name (baseline: make $C_TARGET $C_BASE_VARS fails on the untouched snapshot)"
			tail -20 "$STATE/baseline.log" | sed 's/^/    /'
			return 1
		fi
	fi

	reset_work || return 1
	if [ -n "$C_MUTATE" ]; then
		if ! (cd "$WORK" && "$C_MUTATE") >"$STATE/mutate.log" 2>&1; then
			log "selftest FAIL: $name (the mutation could not be applied)"
			sed 's/^/    /' "$STATE/mutate.log"
			return 1
		fi
	fi

	local out="$STATE/$name.log" rc=0 got problem=""
	# shellcheck disable=SC2086
	run_target "$out" "$C_TARGET" $C_VARS || rc=$?
	if [ "$rc" -eq 0 ]; then got="pass"; else got="fail"; fi

	if [ "$got" != "$C_EXPECT" ]; then
		problem="expected make $C_TARGET to $C_EXPECT, but it did $got"
	elif [ -n "$C_CONTAINS" ] && ! grep -qF -- "$C_CONTAINS" "$out"; then
		problem="output lacks: $C_CONTAINS"
	elif [ -n "$C_NOT_CONTAINS" ] && grep -qF -- "$C_NOT_CONTAINS" "$out"; then
		problem="output unexpectedly contains: $C_NOT_CONTAINS"
	elif [ -n "$C_EXTRA_CHECK" ]; then
		problem=$("$C_EXTRA_CHECK")
	fi
	if [ -n "$problem" ]; then
		log "selftest FAIL: $name ($problem)"
		tail -30 "$out" | sed 's/^/    /'
		return 1
	fi
	log "selftest ok: $name"
}

# run_cases <names...>: prints the summary and returns non-zero on failure.
run_cases() {
	local ok=0 failed=0 c
	for c in "$@"; do
		if run_case "$c"; then ok=$((ok + 1)); else failed=$((failed + 1)); fi
	done
	log "selftest summary: $ok ok, $failed failed"
	[ "$failed" -eq 0 ]
}

# ---- cases -----------------------------------------------------------------

# R2.AC1: a compilation error in the proxy or in a plugin fails build-check.
case_build_proxy_error() { C_TARGET=build-check; C_MUTATE=mut_build_proxy_error; C_CONTAINS="app/app.go"; }
mut_build_proxy_error() { printf '\nfunc selftestBroken( {\n' >>app/app.go; }
case_build_plugin_error() { C_TARGET=build-check; C_MUTATE=mut_build_plugin_error; C_CONTAINS="hello_plugin.go"; }
mut_build_plugin_error() { printf '\nvar selftestBroken int = "not an int"\n' >>plugins/hello-plugin/hello_plugin.go; }

# R2.AC2: a go vet finding fails vet, also inside a plugin module.
case_vet_printf() { C_TARGET=vet; C_MUTATE=mut_vet_printf; C_CONTAINS="wrong type"; }
mut_vet_printf() {
	printf '%s\n' 'package main' '' 'import "fmt"' '' 'func selftestVet() { fmt.Printf("%d\n", "not a number") }' \
		>plugins/hello-plugin/zz_selftest_vet.go
}

# R2.AC5 and F-52: untidy modules, a Go version skew and a shared module
# resolved to different versions by host and plugin fail modules.
case_modules_untidy() { C_TARGET=modules; C_MUTATE=mut_modules_untidy; C_CONTAINS="not tidy"; }
mut_modules_untidy() { go mod edit -droprequire=github.com/gorilla/websocket; }
case_modules_go_version_skew() { C_TARGET=modules; C_MUTATE=mut_modules_go_version_skew; C_CONTAINS="go version mismatch"; }
mut_modules_go_version_skew() { (cd plugins/hello-plugin && go mod edit -go=1.24.0); }
case_modules_shared_version_skew() { C_TARGET=modules; C_MUTATE=mut_modules_shared_version_skew; C_CONTAINS="shared module version mismatch"; }
mut_modules_shared_version_skew() { go mod edit -require=gopkg.in/yaml.v3@v3.0.0 && go mod tidy; }

# R2.AC3: a failing test or a data race fails test-race.
case_test_failing() { C_TARGET=test-race; C_MUTATE=mut_test_failing; C_CONTAINS="injected failure"; }
mut_test_failing() {
	printf '%s\n' 'package app' '' 'import "testing"' '' \
		'func TestSelftestInjectedFailure(t *testing.T) { t.Fatal("injected failure") }' \
		>app/zz_selftest_fail_test.go
}
case_test_data_race() { C_TARGET=test-race; C_MUTATE=mut_test_data_race; C_CONTAINS="DATA RACE"; }
mut_test_data_race() {
	printf '%s\n' 'package app' '' 'import (' '	"sync"' '	"testing"' ')' '' \
		'func TestSelftestDataRace(t *testing.T) {' \
		'	counter := 0' \
		'	var wg sync.WaitGroup' \
		'	for range 2 {' \
		'		wg.Go(func() { counter++ })' \
		'	}' \
		'	wg.Wait()' \
		'	t.Log(counter)' \
		'}' >app/zz_selftest_race_test.go
}

# R2.AC4: a test that needs the external network fails test-hermetic.
case_hermetic_external_dial() { C_TARGET=test-hermetic; C_MUTATE=mut_hermetic_external_dial; C_CONTAINS="external dial failed"; }
mut_hermetic_external_dial() {
	printf '%s\n' 'package app' '' 'import (' '	"net"' '	"testing"' '	"time"' ')' '' \
		'func TestSelftestExternalDial(t *testing.T) {' \
		'	conn, err := net.DialTimeout("tcp", "1.1.1.1:443", 3*time.Second)' \
		'	if err != nil {' \
		'		t.Fatalf("external dial failed: %v", err)' \
		'	}' \
		'	_ = conn.Close()' \
		'}' >app/zz_selftest_dial_test.go
}

# R2.AC6: a broken Dockerfile fails image (needs docker or podman).
case_image_broken_dockerfile() { C_TARGET=image; C_REQUIRES=container; C_MUTATE=mut_image_broken_dockerfile; C_CONTAINS="RUN exit 1"; }
mut_image_broken_dockerfile() {
	awk '{ print } /^FROM / && !done { print "RUN exit 1"; done = 1 }' Dockerfile >Dockerfile.selftest &&
		mv Dockerfile.selftest Dockerfile
}

# R3.AC1: a new lint violation fails lint; R3.AC2: the pre-existing debt alone
# does not (lint-all must still find it, or the boundary is not exercised).
case_lint_new_violation() { C_TARGET=lint; C_BASE_VARS="LINT_MODE=rev LINT_BASE=HEAD"; C_MUTATE=mut_lint_new_violation; C_CONTAINS="errcheck"; }
mut_lint_new_violation() {
	printf '%s\n' 'package app' '' 'import "os"' '' 'func selftestLint() { os.Remove("selftest-missing-file") }' \
		>app/zz_selftest_lint.go &&
		git add -N app/zz_selftest_lint.go
}
case_lint_legacy_only() {
	C_TARGET=lint; C_VARS="LINT_MODE=rev LINT_BASE=HEAD"; C_EXPECT=pass; C_BASELINE=no
	C_CONTAINS="lint: ok"; C_EXTRA_CHECK=chk_lint_legacy_debt_exists
}
chk_lint_legacy_debt_exists() {
	if make -C "$WORK" --no-print-directory lint-all >"$STATE/lint-all.log" 2>&1; then
		echo "lint-all found no issues: the pre-existing debt boundary is not exercised"
	fi
}

# R3.AC3: a reachable vulnerability fails vuln. The fixture module is scanned
# with the Go 1.24.0 standard library, which has reachable vulnerabilities.
case_vuln_reachable() {
	C_TARGET=vuln
	C_VARS="VULN_DIRS=scripts/ci/testdata/vulnfixture VULN_RUN_GOTOOLCHAIN=go1.24.0"
	C_CONTAINS="Your code is affected"
}

# R4.AC1 and R4.AC2: a plugin with a tampered signature is not loaded, and a
# missing middleware effect is detected.
case_smoke_tampered_signature() { C_TARGET=smoke-plugins; C_VARS="SMOKE_TAMPER=signature"; C_CONTAINS="plugin not loaded"; }
case_smoke_missing_header() { C_TARGET=smoke-plugins; C_VARS="SMOKE_EXPECT_HEADER=X-Not-Set"; C_CONTAINS="X-Not-Set"; }

# R5.AC2: coverage below a floor fails; R5.AC3: lowering a floor fails, while
# deleting a package together with its floor passes.
case_coverage_below_floor() { C_TARGET=coverage; C_BASE_VARS="COVERAGE_BASE=HEAD"; C_MUTATE=mut_coverage_below_floor; C_CONTAINS="below floor"; }
mut_coverage_below_floor() { rm app/app_test.go; }
case_coverage_floor_lowered() { C_TARGET=coverage; C_BASE_VARS="COVERAGE_BASE=HEAD"; C_MUTATE=mut_coverage_floor_lowered; C_CONTAINS="floor lowered"; }
mut_coverage_floor_lowered() { set_floor dito/app 1; }
case_coverage_package_removed() {
	C_TARGET=coverage; C_BASE_VARS="COVERAGE_BASE=HEAD"; C_EXPECT=pass
	C_MUTATE=mut_coverage_package_removed; C_CONTAINS="coverage: ok"
}
mut_coverage_package_removed() { rm -rf cmd/plugin-signer && set_floor dito/cmd/plugin-signer ""; }
# set_floor <package> <floor>: rewrites (or, with an empty floor, removes) a floor line.
set_floor() {
	awk -v pkg="$1" -v floor="$2" '$1 == pkg { if (floor != "") print pkg, floor; next } { print }' \
		scripts/ci/coverage-floors.txt >coverage-floors.tmp && mv coverage-floors.tmp scripts/ci/coverage-floors.txt
}

# R1.AC5, R1.AC4, R6.AC2 and R6.AC1: policy violations in the workflows fail
# the workflows target.
case_workflows_tag_ref() { C_TARGET=workflows; C_MUTATE=mut_workflows_tag_ref; C_CONTAINS="not pinned to a commit SHA"; }
mut_workflows_tag_ref() { replace_first .github/workflows/ci.yml 'actions/checkout@[0-9a-f]*' 'actions/checkout@v7'; }
case_workflows_write_permission() { C_TARGET=workflows; C_MUTATE=mut_workflows_write_permission; C_CONTAINS="write permission"; }
mut_workflows_write_permission() { replace_first .github/workflows/ci.yml 'contents: read' 'contents: write'; }
case_workflows_hardcoded_tool_version() { C_TARGET=workflows; C_MUTATE=mut_workflows_hardcoded_tool_version; C_CONTAINS="tool version outside tools.mk"; }
mut_workflows_hardcoded_tool_version() {
	# Built at run time: this file must not contain a pinned tool version itself.
	printf '# go run golang.org/x/vuln/cmd/%s@%s ./...\n' govulncheck v1.8.0 >>.github/workflows/ci.yml
}
case_workflows_job_without_make() { C_TARGET=workflows; C_MUTATE=mut_workflows_job_without_make; C_CONTAINS="does not run a make target"; }
mut_workflows_job_without_make() {
	printf '%s\n' '  rogue:' '    runs-on: ubuntu-24.04' '    steps:' '      - run: go test ./...' >>.github/workflows/ci.yml
}
# replace_first <file> <regex> <replacement>: replaces the first match only.
replace_first() {
	awk -v re="$2" -v rep="$3" '!done && match($0, re) { $0 = substr($0, 1, RSTART - 1) rep substr($0, RSTART + RLENGTH); done = 1 } { print }' \
		"$1" >"$1.selftest" && mv "$1.selftest" "$1"
}

# R7.AC4: without gomodTidyAll Renovate would not keep the plugin module in step.
case_renovate_missing_gomodtidyall() { C_TARGET=workflows; C_MUTATE=mut_renovate_missing_gomodtidyall; C_CONTAINS="gomodTidyAll"; }
mut_renovate_missing_gomodtidyall() {
	jq '.postUpdateOptions -= ["gomodTidyAll"]' renovate.json >renovate.selftest && mv renovate.selftest renovate.json
}

# R6.AC3: make ci runs the checks in order and stops at the first failure. No
# baseline (it would be a full ci run): the output must show that ci started
# (build-check ran) and stopped before test-race.
case_ci_stops_at_first_failure() {
	C_TARGET=ci; C_BASELINE=no; C_MUTATE=mut_vet_printf
	C_CONTAINS="build-check: ok"; C_NOT_CONTAINS="test-race: ok"
}

# ---- self-check ------------------------------------------------------------
# A fake repository with known outcomes: the harness must accept an effective
# mutation and a passing boundary case, and reject an ineffective mutation and
# a red baseline.

case_sc_effective() { C_TARGET=good; C_MUTATE=sc_break; }
case_sc_ineffective() { C_TARGET=good; C_MUTATE=sc_noop; }
case_sc_red_baseline() { C_TARGET=bad; C_MUTATE=sc_noop; }
case_sc_boundary_pass() { C_TARGET=good; C_EXPECT=pass; C_MUTATE=sc_noop; C_CONTAINS="good: ok"; }
sc_break() { touch broken; }
sc_noop() { touch unrelated; }

self_check() {
	local got expected="ok:sc-effective FAIL:sc-ineffective FAIL:sc-red-baseline ok:sc-boundary-pass" c
	SC_FAKE=$(mktemp -d "${TMPDIR:-/tmp}/dito-selftest-fake.XXXXXX") || exit 1
	printf '%s\n' '.PHONY: good bad' 'good:' "${TAB}@test ! -f broken" "${TAB}@echo \"good: ok\"" 'bad:' "${TAB}@false" >"$SC_FAKE/Makefile"
	git -C "$SC_FAKE" init -q
	STATE=$(mktemp -d "${TMPDIR:-/tmp}/dito-selftest-state.XXXXXX") || exit 1
	trap 'cleanup; rm -rf "$SC_FAKE"' EXIT
	snapshot "$SC_FAKE" || { log "selftest self-check: could not create the snapshot"; exit 1; }
	got=""
	for c in sc-effective sc-ineffective sc-red-baseline sc-boundary-pass; do
		if run_case "$c" >/dev/null 2>&1; then got="$got ok:$c"; else got="$got FAIL:$c"; fi
	done
	got=${got# }
	if [ "$got" != "$expected" ]; then
		log "selftest self-check: FAIL (expected \"$expected\", got \"$got\")"
		exit 1
	fi
	log "selftest self-check: ok"
}

# ---- main --------------------------------------------------------------------

main() {
	if [ "${1:-}" = "--self-check" ]; then
		self_check
		return
	fi
	local cases=${SELFTEST_CASES:-$ALL_CASES}
	STATE=$(mktemp -d "${TMPDIR:-/tmp}/dito-selftest-state.XXXXXX") || exit 1
	trap cleanup EXIT
	snapshot "$ROOT" || { log "selftest: could not create the snapshot of $ROOT"; exit 1; }
	# shellcheck disable=SC2086 # case names are space-separated
	run_cases $cases
}

main "$@"
