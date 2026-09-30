#!/usr/bin/env bash
# End-to-end plugin smoke test (S-02, R4).
#
# In a temporary directory: builds the proxy, the plugin-signer and the example
# plugin with the same toolchain, generates a throwaway key pair, signs the
# plugin, writes a config with the public key hash, starts the proxy on
# loopback and calls a location that uses the plugin middleware (its backend
# is the proxy's own /metrics endpoint). Checks that the plugin was loaded and
# that the response carries the middleware header, then removes the directory
# together with the keys.
#
# Environment (set by `make smoke-plugins`):
#   SMOKE_PORT           proxy port (default 18181)
#   SMOKE_EXPECT_HEADER  header set by the middleware (default X-Hello-Plugin)
#   SMOKE_TAMPER         "signature": alter the plugin after signing (selftest)
set -uo pipefail
cd "$(dirname "$0")/../.."

port=${SMOKE_PORT:-18181}
expect_header=${SMOKE_EXPECT_HEADER:-X-Hello-Plugin}
tamper=${SMOKE_TAMPER:-}

work=$(mktemp -d "${TMPDIR:-/tmp}/dito-smoke.XXXXXX") || exit 1
pid=""

cleanup() {
	if [ -n "$pid" ]; then
		kill "$pid" 2>/dev/null
		wait "$pid" 2>/dev/null
		pid=""
	fi
	rm -rf "$work"
}
trap cleanup EXIT

fail() {
	echo "smoke-plugins: FAIL: $*" >&2
	if [ -f "$work/dito.log" ]; then sed 's/^/    dito| /' "$work/dito.log" >&2; fi
	exit 1
}

go build -o "$work/dito" ./cmd || fail "proxy build"
go build -o "$work/plugin-signer" ./cmd/plugin-signer || fail "plugin-signer build"
mkdir -p "$work/plugins/hello-plugin"
(cd plugins/hello-plugin && go build -buildmode=plugin -o "$work/plugins/hello-plugin/hello-plugin.so" .) ||
	fail "plugin build"
cp plugins/hello-plugin/config.yaml "$work/plugins/hello-plugin/"

(cd "$work" && ./plugin-signer generate-keys >/dev/null && ./plugin-signer sign plugins/hello-plugin/hello-plugin.so >/dev/null) ||
	fail "key generation or signing"
if [ "$tamper" = "signature" ]; then
	printf 'tampered' >>"$work/plugins/hello-plugin/hello-plugin.so"
fi

if command -v shasum >/dev/null 2>&1; then
	hash=$(shasum -a 256 "$work/ed25519_public.key" | awk '{print $1}')
else
	hash=$(sha256sum "$work/ed25519_public.key" | awk '{print $1}')
fi

cat >"$work/config.yaml" <<EOF
port: "$port"
hot_reload: false
logging: {enabled: true, verbose: false, level: "info"}
metrics: {enabled: true, path: "/metrics"}
plugins:
  directory: "./plugins"
  public_key_path: "./ed25519_public.key"
  public_key_hash: "$hash"
transport:
  http: {dial_timeout: 2s, response_header_timeout: 5s}
locations:
  - path: "^/smoke$"
    target_url: "http://127.0.0.1:$port/metrics"
    replace_path: true
    middlewares: [hello-plugin]
EOF

(cd "$work" && exec ./dito -f config.yaml >dito.log 2>&1) &
pid=$!

ready=""
for _ in $(seq 1 60); do
	if curl -s -o /dev/null "http://127.0.0.1:$port/metrics"; then
		ready=1
		break
	fi
	kill -0 "$pid" 2>/dev/null || fail "the proxy exited during startup"
	sleep 0.5
done
[ -n "$ready" ] || fail "the proxy is not ready on port $port"

grep -q "Plugin loaded" "$work/dito.log" || fail "plugin not loaded"

headers=$(curl -s -D - -o /dev/null "http://127.0.0.1:$port/smoke") || fail "request to /smoke failed"
printf '%s\n' "$headers" | head -1 | grep -q " 200" ||
	fail "unexpected status from /smoke: $(printf '%s\n' "$headers" | head -1)"
printf '%s\n' "$headers" | grep -qi "^$expect_header:" || fail "the response lacks the middleware header $expect_header"
echo "smoke-plugins: plugin loaded and middleware applied"

cleanup
trap - EXIT
[ ! -e "$work" ] || fail "the temporary directory with the keys was not removed"
echo "smoke-plugins: keys removed"
echo "smoke-plugins: ok"
