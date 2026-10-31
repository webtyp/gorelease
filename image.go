package gorelease

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"webtyp.com/sitec"
)

var (
	ErrNotApp       = errors.New("gorelease: not a webtyp app (no web/server/main.go)")
	ErrDirtyTree    = errors.New("gorelease: the working tree has uncommitted changes; commit or stash them first")
	ErrTagNotAtHead = errors.New("gorelease: the tag does not point at the current commit")
	ErrTestsFailed  = errors.New("gorelease: gotest failed; nothing was published")
	ErrNoDigest     = errors.New("gorelease: the registry did not report the image digest")
)

const (
	appServerEntry = "web/server/main.go"
	appServerPkg   = "./web/server"
	appMigrateDir  = "cmd/migrate"
	appMigratePkg  = "./cmd/migrate"
	appPublicDir   = "web/public"
	imageRegistry  = "ghcr.io/"
	imagePlatform  = "linux/amd64"
	ctxAppDir      = "app"
	ctxServer      = "server"
	ctxMigrate     = "migrate"
	ctxCerts       = "certs"
	dockerfileName = "Dockerfile"
	digestMark     = "@"
	digestPrefix   = "sha256:"
	inspectFormat  = "{{index .RepoDigests 0}}"
)

// dockerfile is the whole image: the built tree on a minimal non-root base.
const dockerfile = `FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --chown=65532:65532 app/ /app/
ENTRYPOINT ["/app/server"]
`

var crossEnv = []string{"CGO_ENABLED=0", "GOOS=linux", "GOARCH=amd64"}

// Image is a pushed container image, pinned by its digest.
type Image struct {
	Name   string // "ghcr.io/owner/repo", lower case
	Tag    string
	Digest string // "sha256:<hex>"
}

// Reference is what a server pulls: the image at its digest.
// Example: img.Reference() == "ghcr.io/acme/shop@sha256:…"
func (i Image) Reference() string { return i.Name + digestMark + i.Digest }

// AppDeps are the tools BuildImage runs. Tests replace them; nil uses the real ones.
type AppDeps struct {
	Run  func(dir string, env []string, name string, args ...string) (string, error)
	Site func(rootDir, outDir, appName string) error
}

// IsApp reports whether dir is a webtyp app (it has web/server/main.go).
// Example: if gorelease.IsApp(".") { … }
func IsApp(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, appServerEntry))
	return err == nil
}

// BuildImage builds the app's image for tag (latest tag when empty) and pushes it.
// It refuses a dirty tree, a tag that is not HEAD, and failing tests.
// Example: img, err := g.BuildImage("v1.2.0", gorelease.AppDeps{})
func (g *Gorelease) BuildImage(tag string, deps AppDeps) (Image, error) {
	run, site := deps.run(), deps.site()
	root := g.rootDir
	if !IsApp(root) {
		return Image{}, ErrNotApp
	}
	if tag == "" {
		t, err := g.git.GetLatestTag()
		if err != nil {
			return Image{}, err
		}
		tag = t
	}
	if out, err := run(root, nil, "git", "status", "--porcelain"); err != nil {
		return Image{}, err
	} else if strings.TrimSpace(out) != "" {
		return Image{}, ErrDirtyTree
	}
	head, err := run(root, nil, "git", "rev-parse", "HEAD")
	if err != nil {
		return Image{}, err
	}
	tagged, err := run(root, nil, "git", "rev-list", "-n", "1", tag)
	if err != nil {
		return Image{}, err
	}
	if strings.TrimSpace(head) != strings.TrimSpace(tagged) {
		return Image{}, ErrTagNotAtHead
	}
	if _, err := run(root, nil, "gotest"); err != nil {
		return Image{}, fmt.Errorf("%w: %v", ErrTestsFailed, err)
	}
	repo, err := run(root, nil, "gh", "repo", "view", "--json", "nameWithOwner", "-q", ".nameWithOwner")
	if err != nil {
		return Image{}, err
	}
	repo = strings.ToLower(strings.TrimSpace(repo))
	name := imageRegistry + repo

	ctx, err := os.MkdirTemp("", "gorelease-image-*")
	if err != nil {
		return Image{}, err
	}
	defer os.RemoveAll(ctx)
	app := filepath.Join(ctx, ctxAppDir)
	if err := os.MkdirAll(filepath.Join(app, ctxCerts), 0o755); err != nil {
		return Image{}, err
	}
	if err := site(root, filepath.Join(app, appPublicDir), filepath.Base(repo)); err != nil {
		return Image{}, err
	}
	if _, err := run(root, crossEnv, "go", "build", "-trimpath", "-ldflags", "-s -w", "-o", filepath.Join(app, ctxServer), appServerPkg); err != nil {
		return Image{}, err
	}
	if _, err := os.Stat(filepath.Join(root, appMigrateDir)); err == nil {
		if _, err := run(root, crossEnv, "go", "build", "-trimpath", "-ldflags", "-s -w", "-o", filepath.Join(app, ctxMigrate), appMigratePkg); err != nil {
			return Image{}, err
		}
	}
	if err := os.WriteFile(filepath.Join(ctx, dockerfileName), []byte(dockerfile), 0o644); err != nil {
		return Image{}, err
	}
	ref := name + ":" + tag
	if _, err := run(ctx, nil, "docker", "build", "--platform", imagePlatform, "-t", ref, ctx); err != nil {
		return Image{}, err
	}
	if _, err := run(ctx, nil, "docker", "push", ref); err != nil {
		return Image{}, err
	}
	pinned, err := run(ctx, nil, "docker", "inspect", "--format", inspectFormat, ref)
	if err != nil {
		return Image{}, err
	}
	_, digest, ok := strings.Cut(strings.TrimSpace(pinned), digestMark)
	if !ok || !strings.HasPrefix(digest, digestPrefix) {
		return Image{}, ErrNoDigest
	}
	img := Image{Name: name, Tag: tag, Digest: digest}
	g.consoleOutput("✅ Image → " + img.Reference())
	return img, nil
}

func (d AppDeps) run() func(string, []string, string, ...string) (string, error) {
	if d.Run != nil {
		return d.Run
	}
	return runCommand
}

func (d AppDeps) site() func(string, string, string) error {
	if d.Site != nil {
		return d.Site
	}
	return buildSite
}

func runCommand(dir string, env []string, name string, args ...string) (string, error) {
	var out, stderr bytes.Buffer
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return out.String(), fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out.String(), nil
}

// buildSite builds web/public in release mode with sitec (as goflare does) and copies it to outDir.
func buildSite(rootDir, outDir, appName string) error {
	site, err := sitec.BuildWithConfig(sitec.BuildConfig{
		RootDir:   rootDir,
		Mode:      sitec.ModeRelease,
		OutputDir: appPublicDir,
		AppName:   appName,
	})
	if err != nil {
		return err
	}
	if err := site.WriteTo(sitec.NewOsFS()); err != nil {
		return err
	}
	return os.CopyFS(outDir, os.DirFS(filepath.Join(rootDir, appPublicDir)))
}
