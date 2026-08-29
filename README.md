# gorelease

Create GitHub releases with cross-platform binaries for an existing tag.

`gorelease` is a release-only tool: it does not create tags or commits. It cross-compiles a Go project's `cmd/` binaries for multiple platforms and uploads them as assets to a GitHub Release.

## Installation

```bash
go install github.com/tinywasm/gorelease/cmd/gorelease@latest
```

## Usage as GitHub Action

```yaml
- uses: tinywasm/gorelease@v1
  with:
    tag: v1.2.3        # optional, defaults to latest tag
    version: v0.1.0    # optional, which gorelease version to download
```

See [`action.yml`](./action.yml) for all inputs.

## Usage as CLI

```bash
gorelease [tag]
gorelease version
```

## Documentation

- [GORELEASE.md](docs/GORELEASE.md) — detailed CLI behavior, targets, requirements
- [Flow Diagram](docs/diagrams/GORELEASE_FLOW.md) — visual representation of the process
- [`action.yml`](./action.yml) — generated composite action (do not edit manually, edit `action_data.go`)

## Development

```bash
go test ./...
# Regenerates action.yml if out of sync
```

Action generation:

- `action_data.go` defines the action
- `action.go`, `render.go`, `sync.go` render it to `action.yml`
- `tests/action_sync_test.go` keeps them in sync

## License

MIT
