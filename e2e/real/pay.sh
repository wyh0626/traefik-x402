#!/usr/bin/env bash

set -Eeuo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
client_dir="$script_dir/client"
endpoint=http://127.0.0.1:18080/paid

if ! challenge_headers=$(curl --silent --show-error --dump-header - --output /dev/null "$endpoint"); then
	echo "Cannot reach $endpoint. Start ./run.sh in another terminal first." >&2
	exit 1
fi

if ! grep -q '^HTTP/.* 402' <<<"$challenge_headers"; then
	echo "Expected an HTTP 402 challenge, but the endpoint returned something else." >&2
	exit 1
fi

echo "This command creates one real Base Sepolia testnet payment:"
echo "  endpoint: $endpoint"
(
	cd "$client_dir"
	SERVER_URL="$endpoint" go run . --inspect
)
echo "Testnet USDC has no cash value, but the transfer is an irreversible testnet transaction."
read -r -p "Type PAY to continue: " confirmation
if [[ "$confirmation" != "PAY" ]]; then
	echo "Cancelled."
	exit 0
fi

read -r -s -p "Enter the dedicated x402 Payer test-wallet private key: " evm_private_key
echo
if [[ -z "$evm_private_key" ]]; then
	echo "Private key cannot be empty." >&2
	exit 1
fi
trap 'unset evm_private_key' EXIT

cd "$client_dir"
EVM_PRIVATE_KEY="$evm_private_key" SERVER_URL="$endpoint" go run .
