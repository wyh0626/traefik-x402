# Contributing

Issues and pull requests are welcome. Please open an issue before making a
large behavioral change.

## Local checks

```bash
gofmt -w .
go vet ./...
go test -race ./...
```

To exercise the plugin through a real Traefik process without a wallet:

```bash
TRAEFIK_BIN=/absolute/path/to/traefik ./e2e/run.sh
```

Set `TRAEFIK_PORT=28080` if the default local port `18080` is already in use.

Or use Docker as described in the README.

## Pull requests

- Keep the root plugin module compatible with Traefik's Yaegi runtime.
- Do not add non-standard-library runtime dependencies without discussing it first.
- Add tests for new configuration and payment behavior.
- Never commit wallet private keys, facilitator credentials, or real secrets.
- Do not run a real blockchain payment in CI.
