# contributing

thanks for helping make paystable harder to break.

## local checks

run these before opening a PR:

```bash
go test ./...
go vet ./...
cd dashboard && npm ci && npm run lint && npm run build
```

for the full local stack:

```bash
cp .env.testkit.example .env.testkit
docker compose -f docker-compose.testkit.yml --env-file .env.testkit up --build
```

## pull requests

- keep changes small and explain the payment failure mode being fixed.
- add tests for state-machine, signature, delivery, and migration changes.
- do not commit `.env`, `docs-site/`, `node_modules`, generated local binaries, or local gap notes.
- prefer lowercase conventional commits, for example `fix(webhook): persist early gateway events`.

## security

do not open public issues for vulnerabilities. use the process in `SECURITY.md`.
