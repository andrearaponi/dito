#!/usr/bin/env bash
# Module integrity checks (S-02, R2.AC5):
#   - go.mod and go.sum of the main module and of every plugin module are tidy;
#   - go.mod, every plugin go.mod and the Dockerfile GOTOOLCHAIN declare the
#     same Go version;
#   - modules linked into both the proxy binary and a plugin resolve to the
#     same version: a mismatch is invisible to go build and go mod tidy, but
#     makes plugin.Open fail at runtime (F-52). Only linked
#     modules count: test-only dependencies never reach plugin.Open.
# Every check runs; the script fails at the end if any of them failed.
set -uo pipefail
cd "$(dirname "$0")/../.."

fail=0
report() {
	echo "modules: $*" >&2
	fail=1
}

go_version=$(awk '/^go /{print $2; exit}' go.mod)

if ! out=$(go mod tidy -diff 2>&1); then
	report "go.mod/go.sum are not tidy (run go mod tidy):"
	printf '%s\n' "$out" | sed 's/^/    /' >&2
fi

docker_version=$(sed -n 's/.*GOTOOLCHAIN=go\([0-9][0-9.]*\).*/\1/p' Dockerfile | head -1)
if [ "$docker_version" != "$go_version" ]; then
	report "go version mismatch: go.mod has $go_version, the Dockerfile GOTOOLCHAIN has go${docker_version:-<none>}"
fi

# linked_modules <package>: "path version" of every non-main module linked into
# the package, using the effective version when a module is replaced.
linked_modules() {
	go list -deps -f '{{with .Module}}{{if not .Main}}{{.Path}} {{if .Replace}}{{.Replace.Path}}@{{.Replace.Version}}{{else}}{{.Version}}{{end}}{{end}}{{end}}' "$1" | sort -u
}

if ! host_modules=$(linked_modules ./cmd 2>&1); then
	report "go list -deps failed for the proxy:"
	printf '%s\n' "$host_modules" | sed 's/^/    /' >&2
	host_modules=""
fi

for dir in plugins/*/; do
	dir=${dir%/}
	[ -f "$dir/go.mod" ] || continue

	if ! out=$(cd "$dir" && go mod tidy -diff 2>&1); then
		report "$dir: go.mod/go.sum are not tidy (run go mod tidy in $dir):"
		printf '%s\n' "$out" | sed 's/^/    /' >&2
	fi

	plugin_version=$(awk '/^go /{print $2; exit}' "$dir/go.mod")
	if [ "$plugin_version" != "$go_version" ]; then
		report "go version mismatch: go.mod has $go_version, $dir/go.mod has $plugin_version"
	fi

	if ! plugin_modules=$(cd "$dir" && linked_modules . 2>&1); then
		report "$dir: go list -deps failed:"
		printf '%s\n' "$plugin_modules" | sed 's/^/    /' >&2
		continue
	fi
	mismatches=$(awk '
		NR == FNR { if (NF >= 2) host[$1] = $2; next }
		NF >= 2 && ($1 in host) && host[$1] != $2 { print "    " $1 ": host " host[$1] ", plugin " $2 }
	' <(printf '%s\n' "$host_modules") <(printf '%s\n' "$plugin_modules"))
	if [ -n "$mismatches" ]; then
		report "shared module version mismatch between the host and $dir:"
		printf '%s\n' "$mismatches" >&2
	fi
done

if [ "$fail" -ne 0 ]; then
	echo "modules: FAIL" >&2
	exit 1
fi
echo "modules: ok"
