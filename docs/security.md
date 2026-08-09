# Security considerations

## Key boundary

The plugin never requires a payer or settlement private key. Clients sign their
own payment authorization and the facilitator performs verification and
settlement. Facilitator API credentials, if any, belong in protected Traefik
configuration rather than public labels or source control.

## Request and response headers

- `PAYMENT-SIGNATURE` is removed before proxying unless explicitly enabled.
- A client-supplied payer header is always removed.
- The verified payer returned by the facilitator is injected into the configured
  payer header.
- Upstream-provided `PAYMENT-REQUIRED` and `PAYMENT-RESPONSE` headers are removed;
  only the middleware owns protocol response headers.
- Settled responses force `Cache-Control: private, no-store` and remove common
  CDN cache-control headers. A CDN must not reuse paid content across clients.

Only trust the payer header when the upstream is reachable exclusively through
the trusted Traefik path. A client that can bypass Traefik can forge it directly.

## Facilitator

- HTTPS is required by default.
- Use explicit timeouts and bounded response bodies.
- Select a production facilitator deliberately; the public x402 test facilitator
  is intended for development and supported testnet paths.
- Treat facilitator failure as fail-closed. Do not bypass payment automatically.

## Upstream side effects

This implementation uses `verify -> upstream -> settle`. It prevents charging
for an upstream HTTP 4xx/5xx response, but cannot roll back an upstream side
effect if settlement later fails. The default `allowedMethods` is therefore
`GET, HEAD`. Mutating APIs must be explicitly enabled and should use idempotency
keys or a prepare/commit design.

Before verification, the middleware derives a stable identity from the EVM data
that is actually signed: authorization owner plus nonce. Changing unsigned outer
fields does not bypass the concurrent-use guard. The guard is acquired before
the facilitator call and retained through settlement, so a stale verification
result cannot enter the upstream later. This remains a local in-memory guard,
not a distributed replay database. With multiple Traefik replicas, or after the
first request completes, replay protection still depends on facilitator
settlement guarantees and upstream idempotency. v0.1.0 rejects non-EVM networks;
SVM support requires a canonical signed-transaction identity and separate tests.

## Buffering and availability

Successful responses are buffered before settlement. Keep the per-response
limit bounded and account for concurrent in-flight responses when sizing
Traefik. Do not use this middleware for SSE, WebSocket, indefinite streams, or
large downloads.

## Test wallets

Use a dedicated Base Sepolia wallet for real tests. Never enter a mainnet wallet
private key into example scripts, and never commit a private key or `.env` file.
