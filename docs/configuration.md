# Configuration reference

| Option | Required | Default | Purpose |
| --- | --- | --- | --- |
| `facilitatorURL` | no | `https://x402.org/facilitator` | Facilitator base URL. |
| `facilitatorHeaders` | no | empty | Additional facilitator authentication headers. |
| `allowInsecureFacilitator` | no | `false` | Allows HTTP only for local testing. |
| `facilitatorTimeoutSeconds` | no | `15` | Timeout for each verify or settle call. |
| `maxFacilitatorBodyBytes` | no | `1048576` | Maximum facilitator response body. |
| `scheme` | no | `exact` | Must be `exact`; other schemes are rejected. |
| `network` | yes | - | Concrete EVM CAIP-2 network such as `eip155:84532`. v0.1.0 rejects non-EVM namespaces and wildcards. |
| `asset` | yes | - | Asset identifier required by the scheme. |
| `amount` | yes | - | Positive integer in the asset's atomic unit. |
| `payTo` | yes | - | Recipient address. |
| `maxTimeoutSeconds` | no | `60` | Maximum authorization lifetime. |
| `extra` | no | empty | Scheme-specific fields such as token name and version. |
| `resourceURL` | no | request URL | Fixed public URL when proxy reconstruction is insufficient. |
| `description` | no | empty | Human-readable resource description. |
| `mimeType` | no | `application/json` | Media type advertised to clients. |
| `maxPaymentHeaderBytes` | no | `65536` | Maximum encoded payment signature header size. |
| `maxResponseBodyBytes` | no | `10485760` | Maximum buffered protected response. |
| `allowedMethods` | no | `[GET, HEAD]` | HTTP methods that may enter the payment flow. Values are normalized to uppercase. |
| `forwardPaymentSignature` | no | `false` | Forward the verified payment signature upstream. |
| `payerHeader` | no | `X-X402-Payer` | Trusted payer header injected upstream. |

## Response behavior

| Condition | Result |
| --- | --- |
| Method not in `allowedMethods` | `405` with an `Allow` header; no payment challenge or upstream call. |
| No `PAYMENT-SIGNATURE` | `402` with `PAYMENT-REQUIRED`. |
| Malformed payment payload | `400`. |
| Requirement mismatch or invalid verification | `402` with a new challenge. |
| Facilitator unavailable or malformed response | `502`. |
| Upstream HTTP 400 or later | Upstream response without settlement. |
| Protected response exceeds configured limit | `502` without settlement. |
| Settlement rejected | `402` with `PAYMENT-RESPONSE`. |
| Settlement service unavailable | `502`; protected content is discarded. |
| Settlement succeeds | Upstream response with `PAYMENT-RESPONSE` and forced `Cache-Control: private, no-store`. |

## Multiple prices or recipients

One middleware instance advertises one payment requirement. Create separate
middleware instances and attach them to separate routers for different prices,
assets, networks, or recipients.

## Middleware ordering

Place routing, IP allow-listing, and request-size controls before expensive
paid work. Avoid automatic retries around mutating upstream operations unless
the operation is idempotent. `POST`, `PUT`, `PATCH`, and `DELETE` require an
explicit `allowedMethods` opt-in. If browser CORS preflight reaches this
middleware, add `OPTIONS` deliberately or handle preflight before the paid
route. Do not place another middleware after x402 that restores public caching.
