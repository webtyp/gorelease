# gorelease
<img src="docs/img/badges.svg">

Create GitHub releases with cross-platform binaries for an existing tag.

`gorelease` is a release-only tool: it does not create tags or commits. It cross-compiles a Go project's `cmd/` binaries for multiple platforms and uploads them as assets to a GitHub Release.

The CLI binary (`cmd/gorelease`) is for local developer use. The GitHub Action (`webtyp/gorelease@v1`) is a **pure-bash composite action** generated from Go — no binary download, ~0MB overhead — that reuses the same DRY constants (`DefaultTargets`, `VersionLdFlag`, `DistDir`, etc.).

## Installation (CLI)

```bash
go install webtyp.com/gorelease/cmd/gorelease@latest
```

## Usage as GitHub Action (pure bash)

```yaml
- uses: webtyp/gorelease@v1
  with:
    tag: v1.2.3        # optional, defaults to latest tag (highest semver)
  env:
    GH_TOKEN: ${{ secrets.GITHUB_TOKEN }} # required for gh CLI
```

Inputs: `tag` (optional), `setup-go` (default `'true'`, runs `actions/setup-go@v6` Node 24). No `version` input — the action is bash and does not download a binary.

See [`action.yml`](./action.yml) for all inputs/outputs.

## Usage as CLI

```bash
gorelease [tag]
gorelease image [tag]
gorelease version  # prints GORELEASE_VERSION
```

### Apps: `gorelease image`

Builds a self-hosted webtyp app's container image (`linux/amd64`) and pushes it to `ghcr.io`, pinned by digest. Needs Docker and a one-time setup:
```bash
gh auth refresh -s write:packages
gh auth token | docker login ghcr.io -u <user> --password-stdin
```

It refuses to build if the working tree is dirty, the tag is not at HEAD, or tests fail. The resulting image holds:

| Path in the image | Built from |
|---|---|
| `/app/server` | `./web/server` — the entrypoint |
| `/app/migrate` | `./cmd/migrate`, only if that folder exists |
| `/app/web/public` | the site, built in release mode by `webtyp.com/sitec` |
| `/app/certs` | an empty folder for the TLS certificate volume |

See the [Image Flow](docs/diagrams/IMAGE_FLOW.md).

## Documentation

- [GORELEASE.md](docs/GORELEASE.md) — detailed CLI behavior, targets, requirements
- [Flow Diagram](docs/diagrams/GORELEASE_FLOW.md) — visual representation of the process
- [`action.yml`](./action.yml) — generated composite action (do not edit manually, edit `action_data.go`)

## Development

```bash
go test ./...
# Regenerates action.yml if out of sync (DRY: targets/ldflags from gorelease.go)
```

Action generation:
- `gorelease.go` — DRY source of truth (`DefaultTargets`, `VersionLdFlag`, `DistDir`, `ChecksumFile`)
- `action_data.go` — generates bash steps from those constants (no hard-coded strings)
- `action.go`, `render.go`, `sync.go` render to `action.yml`
- `tests/action_sync_test.go` keeps them in sync

Actions use Node 24: `actions/checkout@v5`, `actions/setup-go@v6`.

## License

MIT
