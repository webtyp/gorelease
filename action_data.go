package gorelease

import (
	"fmt"
	"strings"

	"webtyp.com/ghaction"
	"webtyp.com/git"
)

const (
	ActionFilePath = "action.yml"
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
// It is a pure-bash composite action: no binary download, no extra weight.
// All cross-compile targets, ldflags and publish logic are derived from the
// DRY constants in gorelease.go (DefaultTargets, VersionLdFlag, etc.).
// Uses ghaction typed Action with presets for Node24 optimal defaults.
func GoreleaseAction(_ string) ghaction.Action {
	// DRY: derive bash targets from DefaultTargets() instead of hardcoding.
	targets := DefaultTargets()
	var targetList []string
	for _, t := range targets {
		targetList = append(targetList, fmt.Sprintf("%s/%s", t.GOOS, t.GOARCH))
	}
	targetsStr := strings.Join(targetList, " ")

	// DRY: ldflags prefix comes from VersionLdFlag("") -> "-s -w"
	ldflagsBase := VersionLdFlag("")

	buildScript := fmt.Sprintf(`set -euo pipefail

tag="${{ inputs.tag }}"
if [ -z "$tag" ]; then
  tag=$(git tag --list 'v*' --sort=-v:refname | head -n1)
fi
if [ -z "$tag" ]; then
  echo "no tags found in repository" >&2
  exit 1
fi
echo "Releasing $tag"

# Validate cmd/ layout
if [ ! -d "cmd" ]; then
  echo "no cmd/ found in $(pwd)" >&2
  exit 1
fi
cmds=$(ls -1 cmd 2>/dev/null || true)
if [ -z "$cmds" ]; then
  echo "no cmd/ found in $(pwd)" >&2
  exit 1
fi

mkdir -p %s

for cmd in $cmds; do
  if [ ! -d "cmd/$cmd" ]; then
    continue
  fi
  for target in %s; do
    os=${target%%/*}
    arch=${target##*/}
    suffix="-$os-$arch"
    if [ "$os" = "windows" ]; then
      suffix="$suffix.exe"
    fi
    output="%s/$cmd$suffix"
    echo "Building $cmd for $os/$arch..."
    CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath -ldflags="%s -X main.Version=$tag" -o "$output" "./cmd/$cmd"
  done
done

# Generate checksums (DRY: filename from const)
(cd %s && sha256sum * > %s)
ls -l %s
`, DistDir, targetsStr, DistDir, ldflagsBase, DistDir, ChecksumFile, DistDir)

	publishScript := fmt.Sprintf(`set -euo pipefail

tag="${{ inputs.tag }}"
if [ -z "$tag" ]; then
  tag=$(git tag --list 'v*' --sort=-v:refname | head -n1)
fi

# Resolve publish target: private origin -> derived public repo, else origin
folder=$(basename "$(pwd)")
target=""
if ! repo_json=$(gh repo view --json owner,visibility 2>/dev/null); then
  target=""
else
  visibility=$(echo "$repo_json" | jq -r .visibility)
  if [ "$visibility" = "PUBLIC" ]; then
    target=""
  else
    owner=$(echo "$repo_json" | jq -r .owner.login)
    candidate="$owner/$folder"
    if ! cand_json=$(gh repo view "$candidate" --json visibility 2>/dev/null); then
      echo "origin is private, but derived repo $candidate does not exist or is not accessible. A public repository is required for distribution" >&2
      exit 1
    fi
    cand_vis=$(echo "$cand_json" | jq -r .visibility)
    if [ "$cand_vis" != "PUBLIC" ]; then
      echo "origin is private, and derived repo $candidate is also $cand_vis. A PUBLIC repository is required for distribution" >&2
      exit 1
    fi
    target="$candidate"
  fi
fi

# Create GitHub Release with assets from %s
if [ -n "$target" ]; then
  echo "Publishing to derived repo $target"
  repo_flag="--repo $target"
else
  echo "Publishing to origin"
  repo_flag=""
fi

assets=(%s/*)

if gh release view "$tag" $repo_flag >/dev/null 2>&1; then
  echo "Release $tag already exists, uploading assets..."
  gh release upload "$tag" "${assets[@]}" --clobber $repo_flag
else
  gh release create "$tag" "${assets[@]}" --title "$tag" --generate-notes $repo_flag
fi

if [ -n "$target" ]; then
  url="https://github.com/$target/releases/tag/$tag"
else
  url=$(gh release view "$tag" --json url --jq .url 2>/dev/null || echo "https://github.com/$(gh repo view --json nameWithOwner --jq .nameWithOwner)/releases/tag/$tag")
fi
echo "✅ Release → $url"
`, DistDir, DistDir)

	return ghaction.Action{
		Name:        "Create GitHub Release",
		Description: "Builds Go project to cross-platform binaries and creates a GitHub Release",
		Author:      "webtyp",
		Branding: ghaction.Branding{
			Icon:  "package",
			Color: "blue",
		},
		Inputs: []ghaction.Input{
			{Name: "tag", Description: "tag to release; empty = latest tag from git (highest semver)", Default: "", Required: false},
			{Name: "setup-go", Description: "run actions/setup-go with the project go.mod", Default: "true", Required: false},
		},
		Outputs: []ghaction.Output{
			{Name: "release-url", Description: "URL of the created GitHub Release", Value: "${{ steps.release.outputs.url }}"},
		},
		Steps: []ghaction.Step{
			{
				Name:    "Setup Go",
				If:      "inputs.setup-go == 'true'",
				Uses:    ghaction.DefaultSetupGo,
				With:    []ghaction.KeyValue{{Key: "go-version-file", Value: ghaction.DefaultGoVersionFile}},
				Comment: "The version comes from the project go.mod, so the caller picks it, not us. Updated to Node 24 via ghaction.",
			},
			{
				Name:  "Build cross-platform binaries",
				ID:    "build",
				Shell: "bash",
				Run:   buildScript,
			},
			{
				Name:  "Create GitHub Release",
				ID:    "release",
				Shell: "bash",
				Run:   publishScript,
			},
		},
	}
}
