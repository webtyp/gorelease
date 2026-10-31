---
PLAN: "feat: gorelease.NewCI implements deploy.CI"
TAG: v0.1.11
EXECUTOR: jules
REVIEWER: none
STATUS: running
SESSION: 1664364813568236283
---

> Read AGENTS.md at the repo root before any change.
> **Finish by yourself:** when the delivery check passes, commit, push and open the pull request. Never ask whether to
> proceed or finalize — nobody answers, and a session that waits ends without your work.

## Tools

| Tool | Install |
|---|---|
| `gotest` | `go install webtyp.com/devflow/cmd/gotest@latest` |

## Ecosystem search

1. Existing code: `webtyp.com/gorelease` exports constant `Local deploy.CIID = "webtyp.com/gorelease.Local"` in `release_app.go`.
2. Sibling prior art: `webtyp.com/github` exports `New(cfg Config) (*CI, error)` in `github/github.go`, implementing `deploy.CI`.
3. Decision: **extend** `webtyp.com/gorelease` with `CI` and `NewCI(cfg CIConfig) (*CI, error)`.

## Design gate

1. **Prior art:**
   - [`webtyp.com/github.New`](https://github.com/webtyp/github): implements `deploy.CI` for GitHub Actions.
   - [Fly.io / Kamal](https://kamal-deploy.org/): local deployment runners that build, tag, and push container images directly from developer machines.
2. **Novice-name test:** `gorelease.NewCI(cfg)` mirrors `github.New(cfg)`.
3. **Complexity ledger:** Zero new dependencies (`webtyp.com/deploy` and `webtyp.com/lang` are already imported).
4. **Where it belongs:** `webtyp.com/gorelease` in `ci.go`.
5. **What it deletes:** Eliminates the missing CI error in `webtyp/app` when a project declares `CI: gorelease.Local`.

## Stages

### Stage 1: Implement `deploy.CI` in `ci.go`

Create `ci.go` with:
- `CIConfig`:
  ```go
  type CIConfig struct {
      RootDir  string
      KeyStore KeyStore
      GitHub   GitHubClient
      GitCmd   func(dir string, args ...string) (string, error) // nil = exec.Command("git", ...)
      AppDeps  AppDeps
  }
  ```
- `CI` struct implementing `deploy.CI`:
  - `ID() deploy.CIID`: returns `Local`.
  - `Check(p deploy.Plan, job deploy.Job) ([]deploy.Missing, error)`:
    1. If `c.keyStore == nil`: returns error `ErrNoKeyStore`.
    2. Check signing key in `keyStore`:
       If `_, err := c.keyStore.Get(SigningKey); errors.Is(err, keyring.ErrNotFound)`:
       appends `deploy.Missing{Item: lang.Text("Signing key"), Why: lang.Text("P-256 release signing key"), Fix: deploy.FixAutomatic}`.
    3. Git status checks via `c.gitCmd`:
       - `git status --porcelain`: if output is non-empty, appends `deploy.Missing{Item: lang.Text("Uncommitted changes"), Why: lang.Text("Release requires a clean working tree"), Fix: deploy.FixManual, Steps: []lang.Text{lang.Text("Run gotest and commit your changes with gopush")}}`.
       - `git tag --points-at HEAD`: if output is empty, appends `deploy.Missing{Item: lang.Text("Git tag on HEAD"), Why: lang.Text("Release requires a version tag pointing to HEAD"), Fix: deploy.FixManual, Steps: []lang.Text{lang.Text("Run gopush to commit and tag the new version")}}`.
    4. Returns `missing, nil`.
  - `Provision(p deploy.Plan, job deploy.Job) error`:
    Calls `EnsureSigningKey(c.keyStore)`.
  - `Secrets(p deploy.Plan) deploy.Destination`:
    Returns nil.
  - `Release(p deploy.Plan) (deploy.Run, error)`:
    1. Reads tag from `git tag --points-at HEAD`. If empty, returns error `ErrTagNotAtHead`.
    2. Constructs `g := &Gorelease{rootDir: c.rootDir}`.
    3. Calls `g.ReleaseApp(tag, c.gh, c.keyStore, c.appDeps)`. If error, returns `deploy.Run{}, err`.
    4. Determines repo from `git config --get remote.origin.url` (or `ghcr.io/owner/repo`). Returns `deploy.Run{ID: tag, URL: fmt.Sprintf("https://github.com/%s/releases/tag/%s", repo, tag)}`.
  - `Status(r deploy.Run) (deploy.RunState, error)`:
    Returns `deploy.RunSucceeded, nil`.

Verbatim `ci.go`:

```go
package gorelease

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"webtyp.com/deploy"
	"webtyp.com/keyring"
	"webtyp.com/lang"
)

// CIConfig configures CI.
type CIConfig struct {
	RootDir  string
	KeyStore KeyStore
	GitHub   GitHubClient
	GitCmd   func(dir string, args ...string) (string, error)
	AppDeps  AppDeps
}

// CI implements deploy.CI for publishing releases from the local developer machine.
type CI struct {
	rootDir  string
	keyStore KeyStore
	gh       GitHubClient
	gitCmd   func(dir string, args ...string) (string, error)
	appDeps  AppDeps
}

var _ deploy.CI = (*CI)(nil)

// NewCI creates a new CI handler.
// Example: ci, err := gorelease.NewCI(gorelease.CIConfig{RootDir: ".", KeyStore: kr, GitHub: gh})
func NewCI(cfg CIConfig) (*CI, error) {
	if cfg.RootDir == "" {
		return nil, errors.New("gorelease: RootDir is required")
	}
	if cfg.KeyStore == nil {
		return nil, ErrNoKeyStore
	}
	gitCmd := cfg.GitCmd
	if gitCmd == nil {
		gitCmd = defaultGitCmd
	}
	return &CI{
		rootDir:  cfg.RootDir,
		keyStore: cfg.KeyStore,
		gh:       cfg.GitHub,
		gitCmd:   gitCmd,
		appDeps:  cfg.AppDeps,
	}, nil
}

func (c *CI) ID() deploy.CIID { return Local }

func (c *CI) Check(p deploy.Plan, job deploy.Job) ([]deploy.Missing, error) {
	var missing []deploy.Missing

	_, err := c.keyStore.Get(SigningKey)
	if errors.Is(err, keyring.ErrNotFound) {
		missing = append(missing, deploy.Missing{
			Item: lang.Text("Signing key"),
			Why:  lang.Text("P-256 release signing key"),
			Fix:  deploy.FixAutomatic,
		})
	} else if err != nil {
		return nil, err
	}

	status, err := c.gitCmd(c.rootDir, "status", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("git status: %w", err)
	}
	if strings.TrimSpace(status) != "" {
		missing = append(missing, deploy.Missing{
			Item:  lang.Text("Uncommitted changes"),
			Why:   lang.Text("Release requires a clean working tree"),
			Fix:   deploy.FixManual,
			Steps: []lang.Text{lang.Text("Run gotest and commit your changes with gopush")},
		})
	}

	tags, err := c.gitCmd(c.rootDir, "tag", "--points-at", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("git tag: %w", err)
	}
	if strings.TrimSpace(tags) == "" {
		missing = append(missing, deploy.Missing{
			Item:  lang.Text("Git tag on HEAD"),
			Why:   lang.Text("Release requires a version tag pointing to HEAD"),
			Fix:   deploy.FixManual,
			Steps: []lang.Text{lang.Text("Run gopush to commit and tag the new version")},
		})
	}

	return missing, nil
}

func (c *CI) Provision(p deploy.Plan, job deploy.Job) error {
	_, _, err := EnsureSigningKey(c.keyStore)
	return err
}

func (c *CI) Secrets(p deploy.Plan) deploy.Destination { return nil }

func (c *CI) Release(p deploy.Plan) (deploy.Run, error) {
	tags, err := c.gitCmd(c.rootDir, "tag", "--points-at", "HEAD")
	if err != nil {
		return deploy.Run{}, err
	}
	tag := strings.TrimSpace(strings.Split(strings.TrimSpace(tags), "\n")[0])
	if tag == "" {
		return deploy.Run{}, ErrTagNotAtHead
	}

	g := &Gorelease{rootDir: c.rootDir}
	if err := g.ReleaseApp(tag, c.gh, c.keyStore, c.appDeps); err != nil {
		return deploy.Run{}, err
	}

	remote, _ := c.gitCmd(c.rootDir, "config", "--get", "remote.origin.url")
	repo := parseRepoURL(remote)
	url := ""
	if repo != "" {
		url = fmt.Sprintf("https://github.com/%s/releases/tag/%s", repo, tag)
	}
	return deploy.Run{ID: tag, URL: url}, nil
}

func (c *CI) Status(r deploy.Run) (deploy.RunState, error) {
	if r.ID == "" {
		return deploy.RunFailed, nil
	}
	return deploy.RunSucceeded, nil
}

func defaultGitCmd(dir string, args ...string) (string, error) {
	var out, stderr bytes.Buffer
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s: %w", strings.TrimSpace(stderr.String()), err)
	}
	return out.String(), nil
}

func parseRepoURL(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimSuffix(raw, ".git")
	if strings.HasPrefix(raw, "git@github.com:") {
		return strings.TrimPrefix(raw, "git@github.com:")
	}
	if strings.HasPrefix(raw, "https://github.com/") {
		return strings.TrimPrefix(raw, "https://github.com/")
	}
	return ""
}
```

### Stage 2: Tests in `tests/ci_test.go`

Create `tests/ci_test.go` verifying:
1. `TestCI_Validation`: RootDir required, KeyStore required.
2. `TestCI_Check_MissingKey`: When `keyStore` lacks `SigningKey`, returns `FixAutomatic` item.
3. `TestCI_Check_DirtyAndNoTag`: Dirty git tree returns `Uncommitted changes` manual missing item; no tag returns `Git tag on HEAD` manual missing item.
4. `TestCI_Provision`: Creates and saves key pair to `KeyStore`.
5. `TestCI_Status`: Status returns Succeeded.

Verbatim test code:

```go
package tests

import (
	"errors"
	"testing"

	"webtyp.com/deploy"
	"webtyp.com/gorelease"
	"webtyp.com/keyring"
)

type mapKeyStore map[string]string

func (m mapKeyStore) Get(key string) (string, error) {
	v, ok := m[key]
	if !ok {
		return "", keyring.ErrNotFound
	}
	return v, nil
}

func (m mapKeyStore) Set(key, value string) error {
	m[key] = value
	return nil
}

func TestCI_Validation(t *testing.T) {
	store := make(mapKeyStore)
	_, err := gorelease.NewCI(gorelease.CIConfig{RootDir: "", KeyStore: store})
	if err == nil {
		t.Fatal("expected error on empty RootDir")
	}

	_, err = gorelease.NewCI(gorelease.CIConfig{RootDir: ".", KeyStore: nil})
	if err == nil {
		t.Fatal("expected error on nil KeyStore")
	}
}

func TestCI_Check_KeyAndGit(t *testing.T) {
	store := make(mapKeyStore)
	ci, err := gorelease.NewCI(gorelease.CIConfig{
		RootDir:  ".",
		KeyStore: store,
		GitCmd: func(dir string, args ...string) (string, error) {
			if len(args) > 0 && args[0] == "status" {
				return " M file.go\n", nil
			}
			if len(args) > 0 && args[0] == "tag" {
				return "", nil
			}
			return "", nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	missing, err := ci.Check(deploy.Plan{}, deploy.Job{})
	if err != nil {
		t.Fatal(err)
	}
	// Expected: 1 key missing (FixAutomatic), 1 dirty tree (FixManual), 1 no tag (FixManual)
	if len(missing) != 3 {
		t.Fatalf("expected 3 missing items, got %d: %+v", len(missing), missing)
	}

	// Run Provision: creates signing key
	if err := ci.Provision(deploy.Plan{}, deploy.Job{}); err != nil {
		t.Fatal(err)
	}
	if _, ok := store[gorelease.SigningKey]; !ok {
		t.Fatal("signing key was not provisioned in store")
	}

	// Clean git status & valid tag
	ciClean, _ := gorelease.NewCI(gorelease.CIConfig{
		RootDir:  ".",
		KeyStore: store,
		GitCmd: func(dir string, args ...string) (string, error) {
			if len(args) > 0 && args[0] == "status" {
				return "", nil
			}
			if len(args) > 0 && args[0] == "tag" {
				return "v1.0.0\n", nil
			}
			return "", nil
		},
	})
	missing, err = ciClean.Check(deploy.Plan{}, deploy.Job{})
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) != 0 {
		t.Fatalf("expected 0 missing items on clean tree with key and tag, got %+v", missing)
	}
}

func TestCI_Status(t *testing.T) {
	store := make(mapKeyStore)
	ci, _ := gorelease.NewCI(gorelease.CIConfig{RootDir: ".", KeyStore: store})
	st, err := ci.Status(deploy.Run{ID: "v1.0.0"})
	if err != nil || st != deploy.RunSucceeded {
		t.Fatalf("expected RunSucceeded, got %v, err: %v", st, err)
	}
}
```

### Stage 3: Documentation in `README.md`

Update `README.md` to document `gorelease.NewCI` and `CIConfig` in conformance with the documentation standard (concise, ≤ 150 lines).

## Delivery check

1. `gotest` passes cleanly:
   ```bash
   GOPRIVATE=webtyp.com/*,github.com/veltylabs/* GOFLAGS=-mod=mod gotest
   ```
2. Commit and open PR.
