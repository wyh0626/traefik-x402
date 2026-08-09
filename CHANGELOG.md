# Changelog

All notable changes to this project are documented here.

## v0.1.2

- Allow cold Go builds enough startup time in the Traefik runtime E2E test.
- Improve readiness failure diagnostics.

## v0.1.1

- Fix the Traefik binary asset name used by GitHub Actions E2E.
- Update installation examples to the latest release.

## v0.1.0

- Initial public release.
- x402 v2 `exact` payment challenge, verification, and settlement.
- EVM `eip155:*` network support; other network families are rejected in this release.
- Safe payment-header handling and verified payer injection.
- Safe `GET`/`HEAD` defaults with explicit opt-in for mutating methods.
- Per-instance in-flight payment reuse protection.
- Shared-cache prevention on settled responses.
- Bounded facilitator and upstream response bodies.
- Unit tests, mock Traefik E2E, and Base Sepolia test workflow.
