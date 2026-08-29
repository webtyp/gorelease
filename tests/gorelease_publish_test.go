package gorelease_test

import (
	"errors"
	gitmod "github.com/tinywasm/git"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tinywasm/gorelease"
)

type scriptedRunner struct {
	calls   [][]string
	respond func(args []string) (string, error)
}

func (s *scriptedRunner) record(args []string) {
	cp := make([]string, len(args))
	copy(cp, args)
	s.calls = append(s.calls, cp)
}

func (s *scriptedRunner) Run(name string, args ...string) (string, error) {
	s.record(args)
	return s.respond(args)
}

func (s *scriptedRunner) RunSilent(name string, args ...string) (string, error) {
	s.record(args)
	return s.respond(args)
}

func (s *scriptedRunner) RunWithStdin(input, name string, args ...string) (string, error) {
	s.record(args)
	return s.respond(args)
}

func (s *scriptedRunner) lastReleaseCreateArgs() []string {
	for i := len(s.calls) - 1; i >= 0; i-- {
		c := s.calls[i]
		if len(c) >= 2 && c[0] == "release" && c[1] == "create" {
			return c
		}
	}
	return nil
}

func (s *scriptedRunner) countReleaseCreate() int {
	n := 0
	for _, c := range s.calls {
		if len(c) >= 2 && c[0] == "release" && c[1] == "create" {
			n++
		}
	}
	return n
}

func isRepoView(args []string) bool {
	return len(args) >= 2 && args[0] == "repo" && args[1] == "view"
}

func repoViewTarget(args []string) string {
	if len(args) >= 3 && args[2] != "--json" {
		return args[2]
	}
	return ""
}

func newGitHubWithRunner(r gitmod.SecretRunner) *gitmod.GitHub {
	gh := &gitmod.GitHub{}
	gh.SecretRunner = r
	return gh
}

func createAppDir(t *testing.T, folderName, cmd string) func() {
	t.Helper()
	base, err := os.MkdirTemp("", "release-pub-")
	if err != nil {
		t.Fatalf("mkdtemp: %v", err)
	}
	appDir := filepath.Join(base, folderName)
	cmdDir := filepath.Join(appDir, "cmd", cmd)
	if err := os.MkdirAll(cmdDir, 0755); err != nil {
		t.Fatalf("mkdir cmd: %v", err)
	}
	os.WriteFile(filepath.Join(appDir, "go.mod"), []byte("module testmodule\n\ngo 1.20\n"), 0644)
	os.WriteFile(filepath.Join(cmdDir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0644)

	old, _ := os.Getwd()
	if err := os.Chdir(appDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	return func() {
		os.Chdir(old)
		os.RemoveAll(base)
	}
}

func fakeCrossCompile(tmpDir string, cmds []string, _ []gorelease.CrossTarget, _ string) ([]string, error) {
	var assets []string
	for _, cmd := range cmds {
		p := filepath.Join(tmpDir, cmd+"-linux-amd64")
		os.WriteFile(p, []byte{}, 0644)
		assets = append(assets, p)
	}
	return assets, nil
}

func argsContainRepo(args []string, repo string) bool {
	for i, a := range args {
		if a == "--repo" && i+1 < len(args) && args[i+1] == repo {
			return true
		}
	}
	return false
}

func argsHaveRepoFlag(args []string) bool {
	for _, a := range args {
		if a == "--repo" {
			return true
		}
	}
	return false
}

func TestReleaseOnly_PrivateOrigin_PublishesToDerivedPublicRepo(t *testing.T) {
	cleanup := createAppDir(t, "myapp", "myapp")
	defer cleanup()

	runner := &scriptedRunner{respond: func(args []string) (string, error) {
		if isRepoView(args) {
			switch repoViewTarget(args) {
			case "":
				return `{"owner":{"login":"acme"},"name":"private-source","visibility":"PRIVATE"}`, nil
			case "acme/myapp":
				return `{"owner":{"login":"acme"},"name":"myapp","visibility":"PUBLIC"}`, nil
			}
		}
		return "https://github.com/acme/myapp/releases/tag/v0.3.0", nil
	}}

	goHandler, _ := gorelease.New(&MockGitClient{latestTag: "v0.3.0"})
	goHandler.SetCrossCompileFn(fakeCrossCompile)
	gh := newGitHubWithRunner(runner)

	if err := goHandler.ReleaseOnly("", gh); err != nil {
		t.Fatalf("ReleaseOnly failed: %v", err)
	}

	args := runner.lastReleaseCreateArgs()
	if args == nil {
		t.Fatal("no `gh release create` call recorded")
	}
	if !argsContainRepo(args, "acme/myapp") {
		t.Errorf("expected release published to acme/myapp via --repo, got args: %v", args)
	}
}

func TestReleaseOnly_PublicOrigin_PublishesToOrigin(t *testing.T) {
	cleanup := createAppDir(t, "mytool", "mytool")
	defer cleanup()

	runner := &scriptedRunner{respond: func(args []string) (string, error) {
		if isRepoView(args) {
			return `{"owner":{"login":"acme"},"name":"mytool","visibility":"PUBLIC"}`, nil
		}
		return "https://github.com/acme/mytool/releases/tag/v1.0.0", nil
	}}

	goHandler, _ := gorelease.New(&MockGitClient{latestTag: "v1.0.0"})
	goHandler.SetCrossCompileFn(fakeCrossCompile)
	gh := newGitHubWithRunner(runner)

	if err := goHandler.ReleaseOnly("", gh); err != nil {
		t.Fatalf("ReleaseOnly failed: %v", err)
	}

	args := runner.lastReleaseCreateArgs()
	if args == nil {
		t.Fatal("no `gh release create` call recorded")
	}
	if argsHaveRepoFlag(args) {
		t.Errorf("public origin must publish to origin without --repo, got args: %v", args)
	}
}

func TestReleaseOnly_PrivateOrigin_DerivedRepoNotPublic_Errors(t *testing.T) {
	cleanup := createAppDir(t, "myapp", "myapp")
	defer cleanup()

	runner := &scriptedRunner{respond: func(args []string) (string, error) {
		if isRepoView(args) {
			switch repoViewTarget(args) {
			case "":
				return `{"owner":{"login":"acme"},"name":"private-source","visibility":"PRIVATE"}`, nil
			case "acme/myapp":
				return `{"owner":{"login":"acme"},"name":"myapp","visibility":"PRIVATE"}`, nil
			}
		}
		return "should-not-reach-release-create", nil
	}}

	goHandler, _ := gorelease.New(&MockGitClient{latestTag: "v0.3.0"})
	goHandler.SetCrossCompileFn(fakeCrossCompile)
	gh := newGitHubWithRunner(runner)

	err := goHandler.ReleaseOnly("", gh)
	if err == nil {
		t.Fatal("expected error when derived repo is not public, got nil")
	}
	if runner.lastReleaseCreateArgs() != nil {
		t.Errorf("must not call `gh release create` when no public target exists")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "public") {
		t.Errorf("error should explain there is no public repo to publish to, got: %v", err)
	}
}

func TestReleaseOnly_VisibilityUndetermined_FallsBackToOrigin(t *testing.T) {
	cleanup := createAppDir(t, "myapp", "myapp")
	defer cleanup()

	runner := &scriptedRunner{respond: func(args []string) (string, error) {
		if isRepoView(args) {
			return "not json", nil
		}
		return "https://github.com/acme/private-source/releases/tag/v0.3.0", nil
	}}

	goHandler, _ := gorelease.New(&MockGitClient{latestTag: "v0.3.0"})
	goHandler.SetCrossCompileFn(fakeCrossCompile)
	gh := newGitHubWithRunner(runner)

	if err := goHandler.ReleaseOnly("", gh); err != nil {
		t.Fatalf("fallback to origin must not fail: %v", err)
	}

	args := runner.lastReleaseCreateArgs()
	if args == nil {
		t.Fatal("no `gh release create` call recorded")
	}
	if argsHaveRepoFlag(args) {
		t.Errorf("undetermined visibility must fall back to origin without --repo, got args: %v", args)
	}
}

func TestReleaseOnly_PrivateOrigin_DerivedRepoMissing_Errors(t *testing.T) {
	cleanup := createAppDir(t, "myapp", "myapp")
	defer cleanup()

	runner := &scriptedRunner{respond: func(args []string) (string, error) {
		if isRepoView(args) {
			switch repoViewTarget(args) {
			case "":
				return `{"owner":{"login":"acme"},"name":"private-source","visibility":"PRIVATE"}`, nil
			case "acme/myapp":
				return "", errors.New("GraphQL: Could not resolve to a Repository with the name 'acme/myapp'")
			}
		}
		return "should-not-reach-release-create", nil
	}}

	goHandler, _ := gorelease.New(&MockGitClient{latestTag: "v0.3.0"})
	goHandler.SetCrossCompileFn(fakeCrossCompile)
	gh := newGitHubWithRunner(runner)

	err := goHandler.ReleaseOnly("", gh)
	if err == nil {
		t.Fatal("expected error when derived repo does not exist, got nil")
	}
	if runner.lastReleaseCreateArgs() != nil {
		t.Errorf("must not call `gh release create` when derived repo is missing")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "public") {
		t.Errorf("error should explain there is no public repo to publish to, got: %v", err)
	}
}

func TestReleaseOnly_ReleaseCreateFails_PropagatesError(t *testing.T) {
	cleanup := createAppDir(t, "mytool", "mytool")
	defer cleanup()

	runner := &scriptedRunner{respond: func(args []string) (string, error) {
		if isRepoView(args) {
			return `{"owner":{"login":"acme"},"name":"mytool","visibility":"PUBLIC"}`, nil
		}
		return "", errors.New("HTTP 403: release already exists")
	}}

	goHandler, _ := gorelease.New(&MockGitClient{latestTag: "v1.0.0"})
	goHandler.SetCrossCompileFn(fakeCrossCompile)
	gh := newGitHubWithRunner(runner)

	err := goHandler.ReleaseOnly("", gh)
	if err == nil {
		t.Fatal("expected error when `gh release create` fails, got nil")
	}
	if runner.lastReleaseCreateArgs() == nil {
		t.Error("release create should have been attempted")
	}
}

func TestReleaseOnly_UploadsChecksums(t *testing.T) {
	cleanup := createAppDir(t, "mytool", "mytool")
	defer cleanup()

	runner := &scriptedRunner{respond: func(args []string) (string, error) {
		if isRepoView(args) {
			return `{"owner":{"login":"acme"},"name":"mytool","visibility":"PUBLIC"}`, nil
		}
		return "https://github.com/acme/mytool/releases/tag/v1.0.0", nil
	}}

	goHandler, _ := gorelease.New(&MockGitClient{latestTag: "v1.0.0"})
	goHandler.SetCrossCompileFn(fakeCrossCompile)
	gh := newGitHubWithRunner(runner)

	if err := goHandler.ReleaseOnly("", gh); err != nil {
		t.Fatalf("ReleaseOnly failed: %v", err)
	}

	args := runner.lastReleaseCreateArgs()
	checksumFound := false
	for _, a := range args {
		if strings.HasSuffix(a, "checksums.txt") {
			checksumFound = true
			break
		}
	}
	if !checksumFound {
		t.Errorf("expected a checksums.txt asset in release create, got args: %v", args)
	}
}

func TestReleaseOnly_PublishesSingleReleaseForHighestTag(t *testing.T) {
	cleanup := createAppDir(t, "mytool", "mytool")
	defer cleanup()

	runner := &scriptedRunner{respond: func(args []string) (string, error) {
		if isRepoView(args) {
			return `{"owner":{"login":"acme"},"name":"mytool","visibility":"PUBLIC"}`, nil
		}
		return "https://github.com/acme/mytool/releases/tag/v2.5.3", nil
	}}

	goHandler, _ := gorelease.New(&MockGitClient{latestTag: "v2.5.3"})
	goHandler.SetCrossCompileFn(fakeCrossCompile)
	gh := newGitHubWithRunner(runner)

	if err := goHandler.ReleaseOnly("", gh); err != nil {
		t.Fatalf("ReleaseOnly failed: %v", err)
	}

	if n := runner.countReleaseCreate(); n != 1 {
		t.Errorf("expected exactly 1 release create for the highest tag, got %d", n)
	}
	if args := runner.lastReleaseCreateArgs(); len(args) < 3 || args[2] != "v2.5.3" {
		t.Errorf("release must target the highest tag v2.5.3, got args: %v", args)
	}
}
