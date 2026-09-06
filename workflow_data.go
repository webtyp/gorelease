package gorelease

import (
	"fmt"

	"webtyp.com/ghaction"
)

// ReleaseWorkflow returns the typed release workflow for gorelease self-release.
// It is pure Go, no hard-coded YAML strings — single source for action.yml + workflows.
// Uses ghaction presets for Node24 optimal defaults.
func ReleaseWorkflow() ghaction.Workflow {
	buildRun := fmt.Sprintf(`set -euo pipefail
mkdir -p %s
for target in %s; do
  os=${target%%/*}
  arch=${target##*/}
  CGO_ENABLED=0 GOOS=$os GOARCH=$arch \
    go build -trimpath -ldflags="-s -w -X webtyp.com/gorelease.Version=${GITHUB_REF_NAME} -X main.Version=${GITHUB_REF_NAME}" \
    -o "%s/gorelease-${os}-${arch}" ./cmd/gorelease
done
# windows needs .exe
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 \
  go build -trimpath -ldflags="-s -w -X webtyp.com/gorelease.Version=${GITHUB_REF_NAME} -X main.Version=${GITHUB_REF_NAME}" \
  -o "%s/gorelease-windows-amd64.exe" ./cmd/gorelease
(cd %s && sha256sum * > %s)
ls -l %s
`, DistDir, func() string {
		targets := DefaultTargets()
		var list []string
		for _, t := range targets {
			if t.GOOS == "windows" {
				continue
			}
			list = append(list, fmt.Sprintf("%s/%s", t.GOOS, t.GOARCH))
		}
		// join without windows for loop, windows handled separately
		return fmt.Sprintf("%s", func() string {
			var s string
			for i, t := range list {
				if i > 0 {
					s += " "
				}
				s += t
			}
			return s
		}())
	}(), DistDir, DistDir, DistDir, ChecksumFile, DistDir)

	publishRun := `set -euo pipefail
tag="${GITHUB_REF_NAME}"
if gh release view "$tag" >/dev/null 2>&1; then
  gh release upload "$tag" dist/* --clobber
else
  gh release create "$tag" dist/* \
    --title "$tag" \
    --generate-notes
fi
`

	moveRun := `set -euo pipefail
git tag -f v1 "${GITHUB_SHA}"
git push origin -f v1
`

	return ghaction.Workflow{
		Name: "Release",
		On: ghaction.On{
			Push: &ghaction.Push{Tags: []string{"v*"}},
		},
		Permissions: map[string]string{"contents": "write"},
		Jobs: map[string]ghaction.Job{
			"release": {
				RunsOn: ghaction.DefaultRunsOn,
				Steps: []ghaction.WorkflowStep{
					ghaction.CheckoutWithTags(),
					ghaction.SetupGoStep(),
					{Name: "Build the release binaries", Shell: "bash", Run: buildRun},
					{Name: "Publish the release", Env: []ghaction.KeyValue{{Key: "GH_TOKEN", Value: "${{ secrets.GITHUB_TOKEN }}"}} , Shell: "bash", Run: publishRun},
					{Name: "Move the v1 tag", Shell: "bash", Run: moveRun},
				},
			},
		},
	}
}

// TestWorkflow returns the typed test workflow (PR guard, read-only).
func TestWorkflow() ghaction.Workflow {
	return ghaction.Workflow{
		Name: "Test",
		On: ghaction.On{
			Push:        &ghaction.Push{Branches: []string{"main"}},
			PullRequest: &ghaction.PullRequest{},
		},
		Jobs: map[string]ghaction.Job{
			"test": {
				RunsOn: ghaction.DefaultRunsOn,
				Steps: []ghaction.WorkflowStep{
					ghaction.CheckoutStep(),
					ghaction.SetupGoStep(),
					ghaction.GoVetStep(),
					ghaction.GoTestStep(),
				},
			},
		},
	}
}
