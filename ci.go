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
