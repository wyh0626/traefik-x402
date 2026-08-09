# Troubleshooting

## Plugin is not loaded

- Public plugins must be declared under `experimental.plugins` with a version.
- Local source must be under
  `/plugins-local/src/github.com/wyh0626/traefik-x402` and declared under
  `experimental.localPlugins` without a version.
- Static plugin changes require a Traefik restart.
- Confirm that `moduleName`, `.traefik.yml` import, and `go.mod` module match.

## Always receives 402

- Confirm the client understands x402 v2 headers.
- Decode `PAYMENT-REQUIRED` and compare scheme, network, asset, amount, and payTo.
- Ensure the payer wallet has the correct token on the exact configured network.
- Check facilitator `/supported` for the configured scheme/network pair.

## Invalid payment signature

- `PAYMENT-SIGNATURE` must be standard Base64-encoded x402 v2 JSON.
- The accepted requirement in the payload must match the server challenge.
- Base Sepolia is `eip155:84532`, not the legacy name `base-sepolia`.

## Facilitator returns 400

Inspect the facilitator response and Traefik logs. Common causes include the
wrong asset contract, EIP-712 token name/version, insufficient balance, expired
authorization, or a requirement mismatch.

## Plugin returns 502

A 502 normally means the facilitator was unavailable or malformed, the upstream
could not be safely released, or the protected response exceeded its configured
buffer limit. Check facilitator/upstream reachability and timeout settings.

## POST returns 405

`GET` and `HEAD` are the safe defaults. Add `POST` (or another required method)
to `allowedMethods` only after the upstream operation is idempotent. The
response `Allow` header lists the currently enabled methods.

## Payment request returns 409

The same EVM authorization is already being processed by this middleware
instance. Do not blindly replay the same payment signature in parallel. Let the
original request finish, then create a fresh payment if another request is
needed.

## Non-EVM network is rejected

v0.1.x supports concrete `eip155:*` networks only. SVM/Solana requires a
canonical signed-transaction replay identity and is intentionally deferred.

## Wallet shows zero test USDC

Confirm the wallet is on Base Sepolia and import Circle test USDC using contract
`0x036CbD53842c5426634e7929541eC2318f3dCF7e`. Testnet USDC and mainnet USDC are
different assets.

## Browser preflight receives 402

Route `OPTIONS` requests through a separate higher-priority router without the
x402 middleware. Do not charge browser CORS preflight requests.
