#!/usr/bin/env bash
# Runs a command once and checks its exit status and output (proofs of S-15).
#
# Usage: expect-output.sh [--contains TEXT]... [--absent TEXT]... -- command [args...]
#
# Prints "expect-output: ok" only if the command exits with 0, its combined
# output contains every --contains text and none of the --absent texts.
# Otherwise it prints what did not match and the output, and exits with 1.
set -uo pipefail

contains=()
absent=()
while [ $# -gt 0 ]; do
	case "$1" in
	--contains)
		contains+=("$2")
		shift 2
		;;
	--absent)
		absent+=("$2")
		shift 2
		;;
	--)
		shift
		break
		;;
	*)
		echo "expect-output: unknown option $1" >&2
		exit 2
		;;
	esac
done
if [ $# -eq 0 ]; then
	echo "usage: expect-output.sh [--contains TEXT]... [--absent TEXT]... -- command [args...]" >&2
	exit 2
fi

out=$(mktemp) || exit 2
trap 'rm -f "$out"' EXIT
"$@" >"$out" 2>&1
rc=$?

fail=0
for text in "${contains[@]+"${contains[@]}"}"; do
	if ! grep -qF -- "$text" "$out"; then
		echo "missing: $text"
		fail=1
	fi
done
for text in "${absent[@]+"${absent[@]}"}"; do
	if grep -qF -- "$text" "$out"; then
		echo "unexpected: $text"
		fail=1
	fi
done
if [ "$rc" -ne 0 ]; then
	echo "exit status: $rc"
	fail=1
fi
if [ "$fail" -ne 0 ]; then
	echo "--- output ---"
	cat "$out"
	exit 1
fi
echo "expect-output: ok"
