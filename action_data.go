package gorelease

import (
	"fmt"
	"strings"

	"github.com/tinywasm/git"
)

const (
	ActionFilePath     = "action.yml"
	ReleaseAssetURLFmt = "https://github.com/tinywasm/gorelease/releases/download/%s/%s"
)

// LatestReleaseTag returns the highest semver tag in the repository.
func LatestReleaseTag() (string, error) {
	g, err := git.NewGit()
	if err != nil {
		return "", fmt.Errorf("git init: %w", err)
	}
	tag, err := g.GetLatestTag()
	if err != nil {
		return "", fmt.Errorf("get latest tag: %w", err)
	}
	return strings.TrimSpace(tag), nil
}

// GoreleaseAction builds the description of the gorelease action.
func GoreleaseAction(goreleaseVersion string) Action {
	if goreleaseVersion == "" {
		goreleaseVersion = "v0.1.0"
	}

	downloadScript := fmt.Sprintf(`set -euo pipefail

version="${{ inputs.version }}"
if [ -z "$version" ]; then
  ref="${{ github.action_ref }}"
  case "$ref" in
    v[0-9]*.[0-9]*.[0-9]*) version="$ref" ;;
    *)
      # A floating ref such as v1 carries no release number, so ask GitHub
      # which release is current.
      version="$(curl -fsSL https://api.github.com/repos/tinywasm/gorelease/releases/latest \
        | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n1)"
      ;;
  esac
fi

# Last resort only. This literal is baked at generation time, so it can never
# name the tag being released right now.
if [ -z "$version" ]; then
  version="%s"
fi

case "$(uname -s)-$(uname -m)" in
  Linux-x86_64)  asset=gorelease-linux-amd64 ;;
  Linux-aarch64) asset=gorelease-linux-arm64 ;;
  Darwin-arm64)  asset=gorelease-darwin-arm64 ;;
  Darwin-x86_64) asset=gorelease-darwin-amd64 ;;
  *) echo "gorelease: unsupported platform: $(uname -s)-$(uname -m)" >&2; exit 1 ;;
esac

url="https://github.com/tinywasm/gorelease/releases/download/${version}/${asset}"
dest="${RUNNER_TEMP}/gorelease"

if ! curl -fsSL "$url" -o "$dest"; then
  echo "gorelease: no ${asset} binary in release ${version}." >&2
  echo "  URL: ${url}" >&2
  echo "  The likely cause is that gorelease never ran for that tag." >&2
  echo "  Check https://github.com/tinywasm/gorelease/releases and pin a version that does have binaries via the 'version' input." >&2
  exit 1
fi

chmod +x "$dest"
echo "$(dirname "$dest")" >> "$GITHUB_PATH"
echo "version=${version}" >> "$GITHUB_OUTPUT"`, goreleaseVersion)

	runScript := `set -euo pipefail
tag="${{ inputs.tag }}"
if [ -z "$tag" ]; then
  gorelease
else
  gorelease "$tag"
fi
`

	return Action{
		Name:        "Create GitHub Release",
		Description: "Builds Go project to cross-platform binaries and creates a GitHub Release",
		Author:      "tinywasm",
		Branding: Branding{
			Icon:  "package",
			Color: "blue",
		},
		Inputs: []Input{
			{Name: "version", Description: "which gorelease version to download; empty = resolve automatically", Default: "", Required: false},
			{Name: "tag", Description: "tag to release; empty = latest tag from git", Default: "", Required: false},
			{Name: "setup-go", Description: "run actions/setup-go with the project go.mod", Default: "true", Required: false},
		},
		Outputs: []Output{
			{Name: "gorelease-version", Description: "the version that was resolved and downloaded", Value: "${{ steps.gorelease.outputs.version }}"},
		},
		Steps: []Step{
			{
				Name:    "Setup Go",
				If:      "inputs.setup-go == 'true'",
				Uses:    "actions/setup-go@v5",
				With:    []KeyValue{{Key: "go-version-file", Value: "go.mod"}},
				Comment: "The version comes from the project go.mod, so the caller picks it, not us.",
			},
			{
				Name:  "Resolve and download the gorelease binary",
				ID:    "gorelease",
				Shell: "bash",
				Run:   downloadScript,
			},
			{
				Name:  "Create GitHub Release",
				Shell: "bash",
				Run:   runScript,
			},
		},
	}
}
