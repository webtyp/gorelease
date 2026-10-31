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
	"webtyp.com/gorelease"
)

func Deploy() deploy.Plan {
	return deploy.Plan{
		Name:   "acme-shop",
		Domain: "app.example.com",
		Target: goflare.Worker,
		CI:     gorelease.Local,
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