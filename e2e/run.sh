#!/usr/bin/env bash

set -Eeuo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
repo_dir=$(cd -- "$script_dir/.." && pwd)
runtime_dir=$(mktemp -d "${TMPDIR:-/tmp}/traefik-x402-e2e.XXXXXX")
mock_log="$runtime_dir/mock.log"
traefik_log="$runtime_dir/traefik.log"
traefik_port=${TRAEFIK_PORT:-18080}

traefik_bin=${TRAEFIK_BIN:-}
if [[ -z "$traefik_bin" ]]; then
	traefik_bin=$(command -v traefik || true)
fi
if [[ -z "$traefik_bin" || ! -x "$traefik_bin" ]]; then
	echo "Traefik binary not found. Set TRAEFIK_BIN=/absolute/path/to/traefik." >&2
	exit 1
fi

cleanup() {
	status=$?
	if [[ -n "${traefik_pid:-}" ]]; then
		kill "$traefik_pid" 2>/dev/null || true
		wait "$traefik_pid" 2>/dev/null || true
	fi
	if [[ -n "${mock_pid:-}" ]]; then
		kill "$mock_pid" 2>/dev/null || true
		wait "$mock_pid" 2>/dev/null || true
	fi
	if [[ $status -ne 0 ]]; then
		echo "--- mock log ---" >&2
		tail -n 100 "$mock_log" 2>/dev/null || true
		echo "--- Traefik log ---" >&2
		tail -n 100 "$traefik_log" 2>/dev/null || true
	fi
	rm -rf -- "$runtime_dir"
}
trap cleanup EXIT INT TERM

module_parent="$runtime_dir/plugins-local/src/github.com/wyh0626"
mkdir -p "$module_parent"
ln -s "$repo_dir" "$module_parent/traefik-x402"

(
	cd "$repo_dir"
	go run ./e2e/mock
) >"$mock_log" 2>&1 &
mock_pid=$!

mock_ready=false
for _ in {1..120}; do
	if curl --silent --fail http://127.0.0.1:19090/healthz >/dev/null; then
		mock_ready=true
		break
	fi
	sleep 0.5
done
if [[ "$mock_ready" != "true" ]]; then
	echo "Mock facilitator did not become ready within 60 seconds." >&2
	exit 1
fi

(
	cd "$runtime_dir"
	"$traefik_bin" \
		--entryPoints.web.address="127.0.0.1:$traefik_port" \
		--providers.file.filename="$script_dir/dynamic.yml" \
		--experimental.localPlugins.x402.moduleName=github.com/wyh0626/traefik-x402 \
		--global.checkNewVersion=false \
		--global.sendAnonymousUsage=false \
		--log.level=INFO
) >"$traefik_log" 2>&1 &
traefik_pid=$!

status=000
for _ in {1..120}; do
	if ! kill -0 "$traefik_pid" 2>/dev/null; then
		wait "$traefik_pid"
	fi
	status=$(curl --silent --output /dev/null --write-out '%{http_code}' "http://127.0.0.1:$traefik_port/protected" || true)
	if [[ "$status" == "402" ]]; then
		break
	fi
	sleep 0.5
done
if [[ "$status" != "402" ]]; then
	echo "Traefik did not expose the protected 402 route; last status was $status." >&2
	exit 1
fi

(
	cd "$repo_dir"
	SERVER_URL="http://127.0.0.1:$traefik_port/protected" go run ./e2e/client
)

echo "PASS Traefik x402 runtime smoke test"
