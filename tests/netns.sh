#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
test_binary=$(mktemp /tmp/happ-netns.XXXXXX)
trap 'rm -f "$test_binary"' EXIT
CGO_ENABLED=0 go test -c -o "$test_binary" ./internal/cli
# NixOS tools normally live under /run/current-system, which is hidden below.
for tool in ip iptables mount; do
  tool_dir=$(dirname "$(readlink -f "$(command -v "$tool")")")
  PATH="$tool_dir:$PATH"
done
export PATH
# /run and all interfaces exist only inside this unprivileged namespace.
unshare --user --map-root-user --mount --net --fork sh -c '
  mount --make-rprivate /
  mount -t tmpfs tmpfs /run
  export HAPP_NETNS_TEST=1
  exec "$1" -test.run="^TestLinuxTunnelIntegration$" -test.v -test.timeout=45s
' sh "$test_binary"
