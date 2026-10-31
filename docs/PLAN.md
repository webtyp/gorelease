---
PLAN: "feat: release a self-hosted webtyp app — signed manifest, compose.yaml, Local CI ID and gorelease key"
TAG: v0.1.10
EXECUTOR: jules
REVIEWER: none
STATUS: running
SESSION: 2089433625924173637
---

> Read AGENTS.md at the repo root before any change.
> **Finish by yourself:** when the delivery check passes, commit, push and open the pull request. Never ask whether to
> proceed or finalize — nobody answers, and a session that waits ends without your work.

# S2b — release a self-hosted webtyp app

## Context

`gorelease image` (v0.1.9) builds a webtyp app's `linux/amd64` container image and pushes it to `ghcr.io` pinned by
digest. This plan completes `gorelease` for self-hosted apps:

1. An app declares `Target: selfupdate.Server` and `CI: gorelease.Local` in `config/deploy.go`.
2. Running `gorelease [tag]` in an app repository (`gorelease.IsApp(".")`) reads `config/deploy.go` with `deploy.Scan`,
   ensures the publisher's ECDSA P-256 signing key in `webtyp/keyring`, builds and pushes the image with `BuildImage`,
   generates `compose.yaml` with `selfupdate.ComposeFile`, signs `update.Manifest` (`Version`, `Image`, `Digest`,
   `ComposeDigest`) with `update.SignManifest`, and uploads `compose.yaml`, `manifest.txt` and `manifest.sig` to a
   GitHub Release in **the same repository** (`targetRepo = ""`, even when private).
3. `gorelease key` prints the publisher's public key (base64 of the PKIX DER bytes) so the operator can paste it into
   `/opt/<app>/update.pub` during `selfupdate install`.

Delivery rules: commit after every stage. Never run `git reset`, `git restore`, `git checkout -- <file>` or `git stash`
on your own work. Stuck for 30 minutes: open the PR anyway, listing what fails. A PR that does not compile, or whose
tests could not run, is a failed delivery.

## Tools

```bash
go install webtyp.com/devflow/cmd/gotest@latest
go get webtyp.com/deploy@v0.2.114
go get webtyp.com/selfupdate@v0.0.8
go get webtyp.com/update@v0.0.6
go get webtyp.com/crypto@v0.0.30
go get webtyp.com/keyring@v0.2.10
go mod tidy
```

Environment: `GOPRIVATE=webtyp.com/*,github.com/veltylabs/*`, `GOFLAGS=-mod=mod`. No test may touch Docker, GitHub, git,
the OS keyring or the network.

## Ecosystem search

- Reading `config/deploy.go`: `deploy.Scan(rootDir)` (`webtyp.com/deploy` v0.2.114) — **reuse**.
- Generating the server's `compose.yaml`: `selfupdate.ComposeFile(selfupdate.App{Name, Domain, Repo})` and
  `selfupdate.Server` (`webtyp.com/selfupdate` v0.0.8) — **reuse**.
- Signing and verifying the release manifest: `update.Manifest`, `update.Digest`, `update.SignManifest`,
  `update.ComposeAsset`, `update.ManifestAsset`, `update.SignatureAsset` (`webtyp.com/update` v0.0.6) — **reuse**.
- Key generation: `asym.GenerateKeyPair()` (`webtyp.com/crypto/asym`, in `webtyp.com/crypto` v0.0.30) — **reuse**.
- Credential storage on the developer's machine: `keyring.Keyring` and `keyring.ErrNotFound` (`webtyp.com/keyring`
  v0.2.10) — **reuse**.

## Design gate

1. **Prior art.** Sigstore `cosign` signs container digests with a local key pair
   (https://docs.sigstore.dev/cosign/signing/signing_with_containers/); GoReleaser signs checksums files and uploads
   the signature alongside the release (https://goreleaser.com/customization/sign/); Kamal deploys from the developer's
   machine using the project's config file (https://kamal-deploy.org/docs/configuration/overview/). Same here: build on
   the developer's machine, sign the image and compose digests with the developer's key in the OS keyring, upload to the
   repo's GitHub Release.
2. **Novice names.** `gorelease.Local` (`deploy.CIID`), `gorelease.SigningKey`, `gorelease.KeyStore`,
   `gorelease.EnsureSigningKey(store)`, `gorelease.PublicKeyText(store)`, `(*Gorelease).ReleaseApp(...)`, CLI
   `gorelease key`.
3. **Complexity ledger.** Concepts +2 (`Local`, `KeyStore`) · files the app touches 0 (`config/deploy.go` already
   exists) · ways to publish an app: 1 (`gorelease [tag]`).
4. **Where it belongs.** `gorelease`, the release publisher.
5. **Deletes.** Nothing.

## APIs used

| Symbol | Exact signature | Tag | File |
|---|---|---|---|
| `deploy.CIID`, `deploy.TargetID`, `deploy.Plan` | `type CIID string`, `type TargetID string`, `type Plan struct { Name, Domain string; Target TargetID; CI CIID; ... }` | `webtyp.com/deploy` v0.2.114 | `plan.go` |
| `deploy.Scan`, `deploy.ErrNoPlan` | `func Scan(rootDir string) (Plan, error)` | `webtyp.com/deploy` v0.2.114 | `scan.go`, `errors.go` |
| `selfupdate.Server` | `const Server deploy.TargetID = "webtyp.com/selfupdate.Server"` | `webtyp.com/selfupdate` v0.0.8 | `target.go` |
| `selfupdate.App`, `selfupdate.ComposeFile` | `type App struct { Name, Domain, Repo string }`; `func ComposeFile(app App) ([]byte, error)` | `webtyp.com/selfupdate` v0.0.8 | `composefile.go` |
| `update.Manifest`, `update.SignManifest`, `update.VerifyManifest`, `update.Digest` | `type Manifest struct { Version, Image, Digest, ComposeDigest string }`; `func SignManifest(m Manifest, privateKey []byte) (text, signature []byte, err error)`; `func VerifyManifest(text, signature, publicKey []byte) (Manifest, error)`; `func Digest(data []byte) string` | `webtyp.com/update` v0.0.6 | `manifest.go` |
| `update.ManifestAsset`, `update.SignatureAsset`, `update.ComposeAsset` | `"manifest.txt"`, `"manifest.sig"`, `"compose.yaml"` | `webtyp.com/update` v0.0.6 | `manifest.go` |
| `asym.GenerateKeyPair` | `func GenerateKeyPair() (publicKey []byte, privateKey []byte, err error)` — PKIX DER public key, SEC 1 DER (`x509.MarshalECPrivateKey`) private key | `webtyp.com/crypto` v0.0.30 | `asym/asym.go` |
| `keyring.ErrNotFound` | `const ErrNotFound Error = "keyring: secret not found"` | `webtyp.com/keyring` v0.2.10 | `errors.go` |

If a signature in the published tag differs from this table, stop and name the symbol in the PR; never adapt it by guessing.

## Stage 1 — diagram `docs/diagrams/APP_RELEASE_FLOW.md` (new)

````markdown
# App release flow

What `gorelease [tag]` does when run in a webtyp app (`web/server/main.go` exists). Every branch has a test in
`tests/release_app_test.go`.

```mermaid
flowchart TD
    A["gorelease [tag] in an app"] --> P{"deploy.Scan(rootDir)"}
    P -->|missing or invalid| EP["error from deploy.Scan"]
    P -->|ok| T{"plan.Target == selfupdate.Server?"}
    T -->|no| ET["ErrNotSelfHost"]
    T -->|yes| K{"EnsureSigningKey(store)"}
    K -->|nil store| EK["ErrNoKeyStore"]
    K -->|ok: pub, priv| I["BuildImage(tag, deps)"]
    I -->|error| EI["error from BuildImage"]
    I -->|ok: Image{Name, Tag, Digest}| C["selfupdate.ComposeFile(App{Name, Domain, Repo})"]
    C -->|error| EC["error from ComposeFile"]
    C -->|ok: compose.yaml| S["update.SignManifest(Manifest, priv)"]
    S --> R["gh.CreateRelease(tag, [compose.yaml, manifest.txt, manifest.sig], \"\")<br/>in the app's own repo"]
```
````

Commit.

## Stage 2 — tests `tests/release_app_test.go` (new, red)

Write `tests/release_app_test.go` in `package gorelease_test`:

```go
package gorelease_test

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"webtyp.com/deploy"
	"webtyp.com/gorelease"
	"webtyp.com/keyring"
	"webtyp.com/selfupdate"
	"webtyp.com/update"
)

type memKeyStore struct {
	data map[string]string
}

func newMemKeyStore() *memKeyStore {
	return &memKeyStore{data: make(map[string]string)}
}

func (m *memKeyStore) Get(key string) (string, error) {
	v, ok := m.data[key]
	if !ok {
		return "", keyring.ErrNotFound
	}
	return v, nil
}

func (m *memKeyStore) Set(key, value string) error {
	m.data[key] = value
	return nil
}

type captureGH struct {
	tag        string
	targetRepo string
	assets     map[string][]byte
	url        string
	err        error
}

func (c *captureGH) RepoInfo(repoRef string) (string, string, string, error) {
	return "acme", "shop", "PRIVATE", nil
}

func (c *captureGH) CreateRelease(tag string, assets []string, targetRepo string) (string, error) {
	if c.err != nil {
		return "", c.err
	}
	c.tag = tag
	c.targetRepo = targetRepo
	c.assets = make(map[string][]byte, len(assets))
	for _, p := range assets {
		b, err := os.ReadFile(p)
		if err != nil {
			return "", err
		}
		c.assets[filepath.Base(p)] = b
	}
	return c.url, nil
}

const validDeployGo = `//go:build deploy

package config

import (
	"webtyp.com/deploy"
	"webtyp.com/gorelease"
	"webtyp.com/selfupdate"
)

func Deploy() deploy.Plan {
	return deploy.Plan{
		Name:   "acme-shop",
		Domain: "app.example.com",
		Target: selfupdate.Server,
		CI:     gorelease.Local,
	}
}
`

func setupAppWithDeploy(t *testing.T, deploySrc string) string {
	t.Helper()
	dir := setupApp(t, true)
	if deploySrc != "" {
		cfgDir := filepath.Join(dir, "config")
		if err := os.MkdirAll(cfgDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(cfgDir, "deploy.go"), []byte(deploySrc), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestLocal_ID(t *testing.T) {
	if string(gorelease.Local) != "webtyp.com/gorelease.Local" {
		t.Fatalf("got %q, want webtyp.com/gorelease.Local", gorelease.Local)
	}
}

func TestEnsureSigningKey_CreatesAndReuses(t *testing.T) {
	store := newMemKeyStore()
	pub1, priv1, err := gorelease.EnsureSigningKey(store)
	if err != nil {
		t.Fatal(err)
	}
	if len(pub1) == 0 || len(priv1) == 0 {
		t.Fatal("expected non-empty key pair")
	}
	pub2, priv2, err := gorelease.EnsureSigningKey(store)
	if err != nil {
		t.Fatal(err)
	}
	if string(pub1) != string(pub2) || string(priv1) != string(priv2) {
		t.Fatal("expected EnsureSigningKey to reuse the stored key")
	}
	text, err := gorelease.PublicKeyText(store)
	if err != nil {
		t.Fatal(err)
	}
	if text != base64.StdEncoding.EncodeToString(pub1) {
		t.Fatalf("PublicKeyText = %q, want %q", text, base64.StdEncoding.EncodeToString(pub1))
	}
}

func TestEnsureSigningKey_NilStore(t *testing.T) {
	_, _, err := gorelease.EnsureSigningKey(nil)
	if !errors.Is(err, gorelease.ErrNoKeyStore) {
		t.Fatalf("got %v, want ErrNoKeyStore", err)
	}
}

func TestReleaseApp_NoDeployFile(t *testing.T) {
	dir := setupAppWithDeploy(t, "")
	g, err := gorelease.New(fakeGit{})
	if err != nil {
		t.Fatal(err)
	}
	g.SetRootDir(dir)
	err = g.ReleaseApp("v1.2.0", &captureGH{}, newMemKeyStore(), gorelease.AppDeps{})
	if !errors.Is(err, deploy.ErrNoPlan) {
		t.Fatalf("got %v, want deploy.ErrNoPlan", err)
	}
}

func TestReleaseApp_WrongTarget(t *testing.T) {
	bad := strings.ReplaceAll(validDeployGo, "selfupdate.Server", " selfupdate.Target")
	bad = `//go:build deploy

package config

import (
	"webtyp.com/deploy"
	"webtyp.com/goflare"
)

func Deploy() deploy.Plan {
	return deploy.Plan{
		Name:   "acme-shop",
		Domain: "app.example.com",
		Target: goflare.Worker,
	}
}
`
	dir := setupAppWithDeploy(t, bad)
	g, err := gorelease.New(fakeGit{})
	if err != nil {
		t.Fatal(err)
	}
	g.SetRootDir(dir)
	err = g.ReleaseApp("v1.2.0", &captureGH{}, newMemKeyStore(), gorelease.AppDeps{})
	if !errors.Is(err, gorelease.ErrNotSelfHost) {
		t.Fatalf("got %v, want ErrNotSelfHost", err)
	}
}

func TestReleaseApp_NilKeyStore(t *testing.T) {
	dir := setupAppWithDeploy(t, validDeployGo)
	g, err := gorelease.New(fakeGit{})
	if err != nil {
		t.Fatal(err)
	}
	g.SetRootDir(dir)
	err = g.ReleaseApp("v1.2.0", &captureGH{}, nil, gorelease.AppDeps{})
	if !errors.Is(err, gorelease.ErrNoKeyStore) {
		t.Fatalf("got %v, want ErrNoKeyStore", err)
	}
}

func TestReleaseApp_PublishesSignedRelease(t *testing.T) {
	dir := setupAppWithDeploy(t, validDeployGo)
	g, err := gorelease.New(fakeGit{})
	if err != nil {
		t.Fatal(err)
	}
	g.SetRootDir(dir)
	var out []string
	g.SetConsoleOutput(func(s string) { out = append(out, s) })

	store := newMemKeyStore()
	gh := &captureGH{url: "https://github.com/acme/shop/releases/tag/v1.2.0"}
	deps := &imageTestDeps{
		headOut:    "abc123\n",
		tagOut:     "abc123\n",
		repoOut:    "acme/Shop\n",
		inspectOut: "ghcr.io/acme/shop@sha256:1234567890123456789012345678901234567890123456789012345678901234\n",
	}

	if err := g.ReleaseApp("v1.2.0", gh, store, gorelease.AppDeps{Run: deps.Run, Site: deps.Site}); err != nil {
		t.Fatal(err)
	}
	if gh.tag != "v1.2.0" {
		t.Fatalf("tag = %q, want v1.2.0", gh.tag)
	}
	if gh.targetRepo != "" {
		t.Fatalf("targetRepo = %q, want empty (same private repo)", gh.targetRepo)
	}

	pub, _, err := gorelease.EnsureSigningKey(store)
	if err != nil {
		t.Fatal(err)
	}
	m, err := update.VerifyManifest(gh.assets[update.ManifestAsset], gh.assets[update.SignatureAsset], pub)
	if err != nil {
		t.Fatalf("VerifyManifest failed: %v", err)
	}
	if m.Version != "v1.2.0" || m.Image != "ghcr.io/acme/shop" {
		t.Fatalf("unexpected manifest: %+v", m)
	}
	if err := m.CheckCompose(gh.assets[update.ComposeAsset]); err != nil {
		t.Fatalf("CheckCompose failed: %v", err)
	}
	wantCompose, err := selfupdate.ComposeFile(selfupdate.App{
		Name:   "acme-shop",
		Domain: "app.example.com",
		Repo:   "acme/shop",
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(gh.assets[update.ComposeAsset]) != string(wantCompose) {
		t.Fatalf("compose.yaml mismatch")
	}
}
```

Commit (red).

## Stage 3 — implementation `release_app.go` (new)

Create `release_app.go`:

```go
package gorelease

import (
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"webtyp.com/crypto/asym"
	"webtyp.com/deploy"
	"webtyp.com/keyring"
	"webtyp.com/selfupdate"
	"webtyp.com/update"
)

// Local is the CI identity for an app published from the developer's machine with gorelease.
// Example (config/deploy.go): return deploy.Plan{Name: "shop", Domain: "app.example.com", Target: selfupdate.Server, CI: gorelease.Local}
const Local deploy.CIID = "webtyp.com/gorelease.Local"

// SigningKey is the entry in the gorelease keyring that holds the publisher's P-256 private key (base64 SEC 1 DER).
const SigningKey = "signing_key"

var (
	// ErrNotSelfHost: config/deploy.go does not target selfupdate.Server.
	ErrNotSelfHost = errors.New("gorelease: config/deploy.go Target must be selfupdate.Server")
	// ErrNoKeyStore: signing a release requires a KeyStore.
	ErrNoKeyStore = errors.New("gorelease: a key store is required to sign the manifest")
)

// KeyStore holds the publisher's private signing key; *keyring.Keyring satisfies it.
type KeyStore interface {
	Get(key string) (string, error)
	Set(key, value string) error
}

// EnsureSigningKey loads the publisher's P-256 key pair from store, creating and saving it on first use.
// Example: pub, priv, err := gorelease.EnsureSigningKey(kr)
func EnsureSigningKey(store KeyStore) (publicKey, privateKey []byte, err error) {
	if store == nil {
		return nil, nil, ErrNoKeyStore
	}
	val, err := store.Get(SigningKey)
	if errors.Is(err, keyring.ErrNotFound) {
		pub, priv, genErr := asym.GenerateKeyPair()
		if genErr != nil {
			return nil, nil, genErr
		}
		if setErr := store.Set(SigningKey, base64.StdEncoding.EncodeToString(priv)); setErr != nil {
			return nil, nil, setErr
		}
		return pub, priv, nil
	}
	if err != nil {
		return nil, nil, err
	}
	priv, err := base64.StdEncoding.DecodeString(strings.TrimSpace(val))
	if err != nil {
		return nil, nil, err
	}
	ecPriv, err := x509.ParseECPrivateKey(priv)
	if err != nil {
		return nil, nil, err
	}
	pub, err := x509.MarshalPKIXPublicKey(&ecPriv.PublicKey)
	if err != nil {
		return nil, nil, err
	}
	return pub, priv, nil
}

// PublicKeyText returns the base64-encoded PKIX public key to paste into /opt/<app>/update.pub.
// Example: text, err := gorelease.PublicKeyText(kr)
func PublicKeyText(store KeyStore) (string, error) {
	pub, _, err := EnsureSigningKey(store)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(pub), nil
}

// ReleaseApp builds and pushes the app image, writes compose.yaml, signs the manifest and uploads
// the Release to the app's own repository.
// Example: err := g.ReleaseApp("v1.2.0", gh, kr, gorelease.AppDeps{})
func (g *Gorelease) ReleaseApp(tag string, gh GitHubClient, store KeyStore, deps AppDeps) error {
	plan, err := deploy.Scan(g.rootDir)
	if err != nil {
		return err
	}
	if plan.Target != selfupdate.Server {
		return ErrNotSelfHost
	}
	pub, priv, err := EnsureSigningKey(store)
	if err != nil {
		return err
	}
	img, err := g.BuildImage(tag, deps)
	if err != nil {
		return err
	}
	repo := strings.TrimPrefix(img.Name, imageRegistry)
	compose, err := selfupdate.ComposeFile(selfupdate.App{
		Name:   plan.Name,
		Domain: plan.Domain,
		Repo:   repo,
	})
	if err != nil {
		return err
	}
	m := update.Manifest{
		Version:       img.Tag,
		Image:         img.Name,
		Digest:        img.Digest,
		ComposeDigest: update.Digest(compose),
	}
	manifestText, sig, err := update.SignManifest(m, priv)
	if err != nil {
		return err
	}

	tmpDir, err := os.MkdirTemp("", "gorelease-app-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	files := []struct {
		name string
		data []byte
	}{
		{update.ComposeAsset, compose},
		{update.ManifestAsset, manifestText},
		{update.SignatureAsset, sig},
	}
	var assets []string
	for _, f := range files {
		p := filepath.Join(tmpDir, f.name)
		if err := os.WriteFile(p, f.data, 0o600); err != nil {
			return err
		}
		assets = append(assets, p)
	}

	url, err := gh.CreateRelease(img.Tag, assets, "")
	if err != nil {
		return fmt.Errorf("failed to create GitHub release: %w", err)
	}
	g.consoleOutput("🔑 update.pub → " + base64.StdEncoding.EncodeToString(pub))
	g.consoleOutput("✅ Release → " + url)
	return nil
}
```

Run `gotest`: green. Commit.

## Stage 4 — CLI `cmd/gorelease/main.go`

1. Add subcommand `key` (right after `version` and `image`):
   ```go
   if len(os.Args) > 1 && os.Args[1] == "key" {
   	kr, err := keyring.NewKeyring("gorelease")
   	if err != nil {
   		fmt.Fprintln(os.Stderr, "Error:", err)
   		os.Exit(1)
   	}
   	pub, err := gorelease.PublicKeyText(kr)
   	if err != nil {
   		fmt.Fprintln(os.Stderr, "Error:", err)
   		os.Exit(1)
   	}
   	fmt.Println(pub)
   	os.Exit(0)
   }
   ```
2. At the bottom of `main()`, when `gorelease.IsApp(".")` is true, open `kr, err := keyring.NewKeyring("gorelease")`
   and call `goHandler.ReleaseApp(tag, gh, kr, gorelease.AppDeps{})` instead of `goHandler.ReleaseOnly(tag, gh)`.
3. Update the `usage` text to list `gorelease key` ("Print the publisher's public key (update.pub) in base64 and exit")
   and note that in a webtyp app `gorelease [tag]` builds the image, signs the manifest and `compose.yaml`, and creates
   the Release in the same repository.

Run `gotest`: green. Commit.

## Stage 5 — docs

Update `README.md` and `docs/GORELEASE.md`:
- Document `gorelease [tag]` for a self-hosted webtyp app (`config/deploy.go` with `Target: selfupdate.Server` and
  `CI: gorelease.Local`): builds and pushes the image, generates `compose.yaml` with `selfupdate.ComposeFile`, signs
  `manifest.txt` (`Version`, `Image`, `Digest`, `ComposeDigest`) with the publisher's key in `webtyp/keyring`, and
  uploads `compose.yaml`, `manifest.txt` and `manifest.sig` to the app's own repository.
- Document `gorelease key` to print the base64 public key for `/opt/<app>/update.pub`.
- Link `docs/diagrams/APP_RELEASE_FLOW.md`.

Commit.

## Delivery check

1. `gotest` passes; no test runs a real command or touches the OS keyring.
2. `git diff --stat main` lists only `release_app.go`, `cmd/gorelease/main.go`, `tests/release_app_test.go`,
   `docs/diagrams/APP_RELEASE_FLOW.md`, `README.md`, `docs/GORELEASE.md`, `go.mod`, `go.sum`.
3. `git diff --stat main --diff-filter=D` → empty.

## Stages

| # | Files | Result |
|---|---|---|
| 1 | `docs/diagrams/APP_RELEASE_FLOW.md` | App release flow diagram |
| 2 | `tests/release_app_test.go` | Red tests for every branch |
| 3 | `release_app.go`, `go.mod`, `go.sum` | `Local`, `EnsureSigningKey`, `PublicKeyText`, `ReleaseApp`; green |
| 4 | `cmd/gorelease/main.go` | `gorelease key` and app release dispatch in `gorelease [tag]` |
| 5 | `README.md`, `docs/GORELEASE.md` | Documented |
