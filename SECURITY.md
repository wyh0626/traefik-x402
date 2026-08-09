# Security Policy

## Reporting a vulnerability

Please use GitHub's private vulnerability reporting feature for this repository.
Do not disclose exploitable payment bypasses or secret exposure publicly before
a fix is available.

## Supported versions

Until the first stable release, only the latest tagged version is supported.

## Secrets

The middleware never needs a payer or settlement private key. Facilitator
authentication headers may contain credentials and must be supplied through
appropriately protected Traefik dynamic configuration. Never commit them to
Docker labels or public repositories.
