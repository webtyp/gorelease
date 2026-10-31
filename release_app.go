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
