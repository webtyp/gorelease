package gorelease_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"webtyp.com/gorelease"
)

type imageTestDeps struct {
	statusOut  string
	statusErr  error
	headOut    string
	headErr    error
	tagOut     string
	tagErr     error
	testErr    error
	repoOut    string
	repoErr    error
	inspectOut string
	inspectErr error
}

func (d *imageTestDeps) Run(dir string, env []string, name string, args ...string) (string, error) {
	cmd := name
	if len(args) > 0 {
		cmd += " " + args[0]
	} else {
		cmd += " "
	}
	switch cmd {
	case "git status":
		return d.statusOut, d.statusErr
	case "git rev-parse":
		return d.headOut, d.headErr
	case "git rev-list":
		return d.tagOut, d.tagErr
	case "gotest ":
		return "", d.testErr
	case "gh repo":
		return d.repoOut, d.repoErr
	case "go build":
		return "", nil
	case "docker build":
		// docker build --platform linux/amd64 -t <ref> <ctx>
		app := filepath.Join(args[5], "app")
		if _, err := os.Stat(filepath.Join(app, "certs")); err != nil {
			return "", err
		}
		if _, err := os.Stat(filepath.Join(app, "web", "public", "index.html")); err != nil {
			return "", err
		}

		return "", nil
	case "docker push":
		return "", nil
	case "docker inspect":
		return d.inspectOut, d.inspectErr
	default:
		return "", errors.New("unexpected command: " + cmd)
	}
}

func (d *imageTestDeps) Site(rootDir, outDir, appName string) error {
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(outDir, "index.html"), []byte("hi"), 0644)
}

type fakeGit struct { tag string }
func (g fakeGit) GetLatestTag() (string, error) { return g.tag, nil }

func setupApp(t *testing.T, withMigrate bool) string {
	t.Helper()
	dir := t.TempDir()
	serverDir := filepath.Join(dir, "web", "server")
	if err := os.MkdirAll(serverDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(serverDir, "main.go"), []byte("package main"), 0644); err != nil {
		t.Fatal(err)
	}
	if withMigrate {
		migrateDir := filepath.Join(dir, "cmd", "migrate")
		if err := os.MkdirAll(migrateDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(migrateDir, "main.go"), []byte("package main"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestImage_NotApp(t *testing.T) {
	dir := t.TempDir() // no web/server/main.go
	g, err := gorelease.New(fakeGit{})
	if err != nil { t.Fatal(err) }
	g.SetRootDir(dir)
	_, err = g.BuildImage("v1.2.0", gorelease.AppDeps{})
	if !errors.Is(err, gorelease.ErrNotApp) {
		t.Errorf("got %v, want %v", err, gorelease.ErrNotApp)
	}
}

func TestImage_DirtyTree(t *testing.T) {
	dir := setupApp(t, true)
	g, err := gorelease.New(fakeGit{})
	if err != nil { t.Fatal(err) }
	g.SetRootDir(dir)
	deps := &imageTestDeps{statusOut: " M x.go\n"}
	_, err = g.BuildImage("v1.2.0", gorelease.AppDeps{Run: deps.Run, Site: deps.Site})
	if !errors.Is(err, gorelease.ErrDirtyTree) {
		t.Errorf("got %v, want %v", err, gorelease.ErrDirtyTree)
	}
}

func TestImage_TagNotAtHead(t *testing.T) {
	dir := setupApp(t, true)
	g, err := gorelease.New(fakeGit{})
	if err != nil { t.Fatal(err) }
	g.SetRootDir(dir)
	deps := &imageTestDeps{headOut: "abc123\n", tagOut: "def456\n"}
	_, err = g.BuildImage("v1.2.0", gorelease.AppDeps{Run: deps.Run, Site: deps.Site})
	if !errors.Is(err, gorelease.ErrTagNotAtHead) {
		t.Errorf("got %v, want %v", err, gorelease.ErrTagNotAtHead)
	}
}

func TestImage_TestsFail(t *testing.T) {
	dir := setupApp(t, true)
	g, err := gorelease.New(fakeGit{})
	if err != nil { t.Fatal(err) }
	g.SetRootDir(dir)
	deps := &imageTestDeps{headOut: "abc123\n", tagOut: "abc123\n", testErr: errors.New("boom")}
	_, err = g.BuildImage("v1.2.0", gorelease.AppDeps{Run: deps.Run, Site: deps.Site})
	if !errors.Is(err, gorelease.ErrTestsFailed) {
		t.Errorf("got %v, want ErrTestsFailed", err)
	}
}

func TestImage_Builds(t *testing.T) {
	dir := setupApp(t, true)
	g, err := gorelease.New(fakeGit{})
	if err != nil { t.Fatal(err) }
	g.SetRootDir(dir)

	deps := &imageTestDeps{
		headOut: "abc123\n",
		tagOut: "abc123\n",
		repoOut: "acme/Shop\n",
		inspectOut: "ghcr.io/acme/shop@sha256:1234567890123456789012345678901234567890123456789012345678901234\n",
	}

	img, err := g.BuildImage("v1.2.0", gorelease.AppDeps{Run: deps.Run, Site: deps.Site})
	if err != nil {
		t.Fatal(err)
	}

	wantName := "ghcr.io/acme/shop"
	wantTag := "v1.2.0"
	wantDigest := "sha256:1234567890123456789012345678901234567890123456789012345678901234"

	if img.Name != wantName { t.Errorf("Name = %q, want %q", img.Name, wantName) }
	if img.Tag != wantTag { t.Errorf("Tag = %q, want %q", img.Tag, wantTag) }
	if img.Digest != wantDigest { t.Errorf("Digest = %q, want %q", img.Digest, wantDigest) }

	wantRef := "ghcr.io/acme/shop@sha256:1234567890123456789012345678901234567890123456789012345678901234"
	if img.Reference() != wantRef {
		t.Errorf("Reference() = %q, want %q", img.Reference(), wantRef)
	}
}

func TestImage_NoMigrate(t *testing.T) {
	dir := setupApp(t, false)
	g, err := gorelease.New(fakeGit{})
	if err != nil { t.Fatal(err) }
	g.SetRootDir(dir)

	deps := &imageTestDeps{
		headOut: "abc123\n",
		tagOut: "abc123\n",
		repoOut: "acme/Shop\n",
		inspectOut: "ghcr.io/acme/shop@sha256:1234567890123456789012345678901234567890123456789012345678901234\n",
	}

	img, err := g.BuildImage("v1.2.0", gorelease.AppDeps{Run: deps.Run, Site: deps.Site})
	if err != nil {
		t.Fatal(err)
	}
	if img.Name == "" { t.Error("expected non-empty image") }
}

func TestImage_NoDigest(t *testing.T) {
	dir := setupApp(t, true)
	g, err := gorelease.New(fakeGit{})
	if err != nil { t.Fatal(err) }
	g.SetRootDir(dir)

	deps := &imageTestDeps{
		headOut: "abc123\n",
		tagOut: "abc123\n",
		repoOut: "acme/Shop\n",
		inspectOut: "ghcr.io/acme/shop:v1.2.0\n",
	}

	_, err = g.BuildImage("v1.2.0", gorelease.AppDeps{Run: deps.Run, Site: deps.Site})
	if !errors.Is(err, gorelease.ErrNoDigest) {
		t.Errorf("got %v, want %v", err, gorelease.ErrNoDigest)
	}
}

func TestImage_LatestTag(t *testing.T) {
	dir := setupApp(t, true)
	g, err := gorelease.New(fakeGit{tag: "v1.2.0"})
	if err != nil { t.Fatal(err) }
	g.SetRootDir(dir)

	deps := &imageTestDeps{
		headOut: "abc123\n",
		tagOut: "abc123\n",
		repoOut: "acme/Shop\n",
		inspectOut: "ghcr.io/acme/shop@sha256:1234567890123456789012345678901234567890123456789012345678901234\n",
	}

	img, err := g.BuildImage("", gorelease.AppDeps{Run: deps.Run, Site: deps.Site})
	if err != nil {
		t.Fatal(err)
	}
	if img.Tag != "v1.2.0" {
		t.Errorf("got tag %q, want v1.2.0", img.Tag)
	}
}
