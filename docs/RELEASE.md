# Release Checklist

Target version: `v1.0.0`

## Code

- [ ] `make verify` green (build + tests)
- [ ] `golangci-lint run ./...` reports 0 issues
- [ ] `go vet -tags integration ./...` green
- [ ] Integration tests pass in Docker (`make test-integration`)
- [ ] `gosec` findings triaged
- [ ] CHANGELOG updated with user-visible changes

## Documentation

- [ ] `docs/API.md` matches the routes in `cmd/server/main.go`
- [ ] `env.example` lists every variable the binary reads
- [ ] `docs/OPERATOR_MANUAL.md` matches the deployment topology
- [ ] `docs/runbooks/incident.md` reviewed by on-call

## Operations

- [ ] Load test at expected peak: `make loadtest ARGS="-integration <id> -workers 50"`
- [ ] Restore from backup rehearsed
- [ ] Dashboards and alerts imported
- [ ] Rollback path documented and tested

## Release

1. Tag: `git tag -a v1.0.0 -m "IntegrationHub v1.0.0"` and push the tag.
2. Publish container images tagged `v1.0.0` and `latest`.
3. Deploy to staging, smoke test, then promote.
4. Announce with the changelog entry.