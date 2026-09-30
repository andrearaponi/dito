#!/usr/bin/env bash
# Checks the pinned verification tools (S-02: R6.AC2, C4).
#
# Every tool must report the version pinned in tools.mk, and the analyzers
# must be built with the Go version declared in go.mod. Run through
# `make tools-check`, which exports the variables used below.
set -uo pipefail
cd "$(dirname "$0")/../.."

: "${GO_VERSION:?run through make}" "${TOOLS_GOTOOLCHAIN:?run through make}"

fail=0
report() {
	echo "tools-check: $*" >&2
	fail=1
}

# run_tool <package> <version> <args...>
run_tool() {
	local pkg=$1 version=$2
	shift 2
	GOTOOLCHAIN="$TOOLS_GOTOOLCHAIN" go run "$pkg@$version" "$@" 2>&1 | grep -v '^go: downloading'
}

out=$(run_tool "$GOLANGCI_LINT_PKG" "$GOLANGCI_LINT_VERSION" --version)
case "$out" in
*"has version ${GOLANGCI_LINT_VERSION#v} built with go$GO_VERSION "*) ;;
*) report "golangci-lint: expected ${GOLANGCI_LINT_VERSION#v} built with go$GO_VERSION, got: $out" ;;
esac

out=$(run_tool "$GOVULNCHECK_PKG" "$GOVULNCHECK_VERSION" -version)
case "$out" in
*"govulncheck@$GOVULNCHECK_VERSION"*) ;;
*) report "govulncheck: expected govulncheck@$GOVULNCHECK_VERSION, got: $out" ;;
esac
case "$out" in
*"Go: go$GO_VERSION"*) ;;
*) report "govulncheck: expected to run with go$GO_VERSION, got: $out" ;;
esac

out=$(run_tool "$ACTIONLINT_PKG" "$ACTIONLINT_VERSION" -version)
case "$out" in
"$ACTIONLINT_VERSION"*"built with go$GO_VERSION "*) ;;
*) report "actionlint: expected $ACTIONLINT_VERSION built with go$GO_VERSION, got: $out" ;;
esac

out=$(run_tool "$WALDEN_PKG" "$WALDEN_VERSION" version)
case "$out" in
*"walden $WALDEN_VERSION "*) ;;
*) report "walden: expected $WALDEN_VERSION, got: $out" ;;
esac

if [ "$fail" -ne 0 ]; then
	echo "tools-check: FAIL" >&2
	exit 1
fi
echo "tools-check: ok (go$GO_VERSION)"
