# gorelease

Create GitHub Release with cross-platform binaries for an existing tag.

`gorelease` is a release-only tool: it does not create tags or commits. It cross-compiles
a Go project's `cmd/` binaries for multiple platforms and uploads them as assets to a
GitHub Release.

The CLI (`cmd/gorelease`) and the GitHub Action (`webtyp/gorelease@v1`) share the same DRY
constants (`DefaultTargets`, `VersionLdFlag`, etc.). The action is pure bash — no binary download.

## Installation

```bash
go install webtyp.com/gorelease/cmd/gorelease@latest
```

## Usage

```bash
gorelease [tag]
gorelease image [tag]
gorelease version
```

### Arguments

* **tag**: An optional explicit version tag (e.g., `v1.2.3`). If not provided, it is read
  from `git.GetLatestTag()` (the highest semver tag in the repository).

### Commands

* **image**: Builds a self-hosted webtyp app's container image (`linux/amd64`) and pushes it to `ghcr.io`, pinned by digest. Needs Docker and a one-time setup: `gh auth refresh -s write:packages` and `gh auth token | docker login ghcr.io -u <user> --password-stdin`. See the [Image Flow](diagrams/IMAGE_FLOW.md).
* **version**: Prints `GORELEASE_VERSION=<version>` and exits.

### Behavior

1. **Validation**: Verifies that the repository has a `cmd/` directory with at least one subdirectory.
2. **Tag Resolution**: If no tag is provided, reads the latest tag from git (`git tag --list 'v*' --sort=-v:refname | head -n1` in the bash action, `GetLatestTag()` in the Go CLI).
3. **Cross-Compilation**: Compiles all commands found in `cmd/` for the following platforms:
    * Linux (amd64, arm64)
    * macOS (arm64, amd64)
    * Windows (amd64)

   It injects the version into `main.Version` and uses optimization flags (`-s -w -trimpath`, via `VersionLdFlag`).

4. **Checksums**: Generates a `checksums.txt` file (SHA256) for all compiled binaries and includes it as a release asset (`DistDir`/`ChecksumFile`).

5. **Target Resolution**: Automatically decides where to publish the release:
    * If `origin` is **PUBLIC**, it publishes to `origin` (classic behavior).
    * If `origin` is **PRIVATE**, it derives a public repository name: `<owner>/<folder-name>`.
      If that repository exists and is public, it publishes there using the `--repo` flag.
      This allows maintaining private source code while distributing public binaries. Bash action uses `gh repo view --json owner,visibility` with `jq`.

6. **GitHub Release**: Creates a GitHub Release using the `gh` CLI, with the tag name as
   the title, and uploads the cross-compiled binaries and checksums as release assets.
7. **Cleanup**: Automatically removes the temporary directory (Go CLI, `os.MkdirTemp gorelease-*`) or `dist/` (bash action, created fresh).

### Targets

The default compilation targets are (single source of truth: `DefaultTargets()`):

| OS | Architecture | Artifact Name |
|---|---|---|
| linux | amd64 | `<cmd>-linux-amd64` |
| linux | arm64 | `<cmd>-linux-arm64` |
| darwin | arm64 | `<cmd>-darwin-arm64` |
| darwin | amd64 | `<cmd>-darwin-amd64` |
| windows | amd64 | `<cmd>-windows-amd64.exe` |

### Requirements

* `go` installed and in PATH.
* `gh` (GitHub CLI) installed, authenticated via `GH_TOKEN`/`GITHUB_TOKEN` in CI or via `webtyp/keyring` locally, and in PATH. Also `jq` for JSON parsing in the bash action.
* A standard Go project structure with a `go.mod` file and a `cmd/` directory.
* An existing tag in the git repository.

## Examples

```bash
# Release the latest tag (highest semver)
gorelease

# Release an explicit tag
gorelease v1.2.3

# Build and push an app's container image for a specific tag
gorelease image v1.2.3

# Print version
gorelease version
```

## GitHub Action (pure bash)

Use as a composite action in workflows. No binary is downloaded.

```yaml
- uses: webtyp/gorelease@v1
  with:
    tag: v1.2.3     # optional, defaults to latest tag
    setup-go: 'true' # optional, run actions/setup-go@v6 (Node 24)
  env:
    GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

Inputs: `tag`, `setup-go`. Outputs: `release-url`. The action builds directly with `go build` on the runner (uses `actions/checkout@v5`, `actions/setup-go@v6` — Node 24).

## Flow Diagram

See [GORELEASE_FLOW.md](diagrams/GORELEASE_FLOW.md) — Visual representation of the `gorelease` process.
