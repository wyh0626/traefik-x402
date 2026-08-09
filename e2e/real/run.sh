#!/usr/bin/env bash

set -Eeuo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
plugin_dir=$(cd -- "$script_dir/../.." && pwd)
runtime_dir=$(mktemp -d "${TMPDIR:-/tmp}/traefik-x402-real.XXXXXX")
module_parent="$runtime_dir/plugins-local/src/github.com/wyh0626"
plugin_link="$module_parent/traefik-x402"

pay_to=${PAY_TO:-}
if [[ ! "$pay_to" =~ ^0x[0-9a-fA-F]{40}$ ]]; then
	echo "PAY_TO must be an EVM address (0x followed by 40 hexadecimal characters)." >&2
	echo "Example: PAY_TO=0xYourReceiverAddress ./run.sh" >&2
	exit 1
fi

traefik_bin=${TRAEFIK_BIN:-}
if [[ -z "$traefik_bin" ]]; then
	traefik_bin=$(command -v traefik || true)
fi
if [[ -z "$traefik_bin" || ! -x "$traefik_bin" ]]; then
	echo "Traefik binary not found. Set TRAEFIK_BIN=/absolute/path/to/traefik." >&2
	exit 1
fi

cleanup() {
	if [[ -n "${upstream_pid:-}" ]]; then
		kill "$upstream_pid" 2>/dev/null || true
		wait "$upstream_pid" 2>/dev/null || true
	fi
	rm -rf -- "$runtime_dir"
}
trap cleanup EXIT INT TERM

mkdir -p "$module_parent"
ln -s "$plugin_dir" "$plugin_link"
sed "s/__PAY_TO__/$pay_to/g" "$script_dir/dynamic.yml.tmpl" >"$runtime_dir/dynamic.yml"

go run "$script_dir/upstream/main.go" &
upstream_pid=$!

for _ in {1..50}; do
	if curl --silent --fail http://127.0.0.1:19091/healthz >/dev/null; then
		break
	fi
	if ! kill -0 "$upstream_pid" 2>/dev/null; then
		echo "Local upstream failed to start." >&2
		exit 1
	fi
	sleep 0.1
done
curl --silent --fail http://127.0.0.1:19091/healthz >/dev/null

echo "Real x402 test is starting:"
echo "  endpoint: http://127.0.0.1:18080/paid"
echo "  network:  Base Sepolia (eip155:84532)"
echo "  price:    0.001 test USDC"
echo "  payTo:    $pay_to"
echo "Press Ctrl-C to stop."

cd "$runtime_dir"
	"$traefik_bin" \
	--entryPoints.web.address=127.0.0.1:18080 \
	--providers.file.filename="$runtime_dir/dynamic.yml" \
	--experimental.localPlugins.x402.moduleName=github.com/wyh0626/traefik-x402 \
	--global.checkNewVersion=false \
	--global.sendAnonymousUsage=false \
	--log.level=INFO
