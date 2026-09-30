#!/usr/bin/env bash
# Runs a command with outbound network traffic restricted to loopback
# (S-02, R2.AC4). Usage: scripts/ci/hermetic.sh <command> [args...]
#
# macOS: sandbox-exec denies every outbound connection except loopback and
#        local unix sockets.
# Linux: sudo unshare --net creates a network namespace that only has the
#        loopback interface; privileges are then dropped back to the invoking
#        user with setpriv, so files created by the command keep their owner.
#        Only the command is isolated: the rest of the machine (and a CI
#        runner talking to GitHub) keeps its network.
set -euo pipefail

if [ "$#" -eq 0 ]; then
	echo "usage: $0 <command> [args...]" >&2
	exit 2
fi

case "$(uname -s)" in
Darwin)
	profile='(version 1)(allow default)(deny network-outbound)(allow network-outbound (remote ip "localhost:*"))(allow network-outbound (remote unix-socket))'
	exec sandbox-exec -p "$profile" "$@"
	;;
Linux)
	exec sudo -E env "PATH=$PATH" unshare --net -- sh -c \
		'ip link set lo up && exec setpriv --reuid="$SUDO_UID" --regid="$SUDO_GID" --init-groups -- "$@"' \
		sh "$@"
	;;
*)
	echo "hermetic.sh: unsupported OS $(uname -s)" >&2
	exit 2
	;;
esac
