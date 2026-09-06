package gorelease_test

import (
	"os"
	"path/filepath"
	"testing"

	"webtyp.com/gorelease"
)

// testChdir changes to the specified directory and returns a cleanup function.
func testChdir(t *testing.T, dir string) func() {
	t.Helper()
	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current dir: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("failed to chdir to %s: %v", dir, err)
	}
	return func() {
		os.Chdir(oldDir)
	}
}

// testCreateCmdDirs creates a temporary Go module with cmd/ subdirectories.
func testCreateCmdDirs(t *testing.T, cmds ...string) (dir string, cleanup func()) {
	t.Helper()
	dir, cleanup = testCreateGoModule("testmodule")

	if len(cmds) > 0 {
		cmdDir := filepath.Join(dir, "cmd")
		if err := os.MkdirAll(cmdDir, 0755); err != nil {
			t.Fatalf("failed to create cmd dir: %v", err)
		}

		for _, cmd := range cmds {
			path := filepath.Join(cmdDir, cmd)
			if err := os.MkdirAll(path, 0755); err != nil {
				t.Fatalf("failed to create cmd/%s dir: %v", cmd, err)
			}
			mainFile := filepath.Join(path, "main.go")
			content := "package main\n\nfunc main() {}\n"
			if err := os.WriteFile(mainFile, []byte(content), 0644); err != nil {
				t.Fatalf("failed to write main.go for %s: %v", cmd, err)
			}
		}
	}

	return dir, cleanup
}

// testCreateGoModule creates a temporary Go module
func testCreateGoModule(moduleName string) (dir string, cleanup func()) {
	dir, _ = os.MkdirTemp("", "gitgo-gomod-")

	gomod := "module " + moduleName + "\n\ngo 1.20\n"
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0644)

	main := "package main\n\nfunc main() {}\n"
	os.WriteFile(filepath.Join(dir, "main.go"), []byte(main), 0644)

	cleanup = func() {
		os.RemoveAll(dir)
	}

	return dir, cleanup
}

// newGoHandlerWithMock creates a Gorelease handler with a mock git.
func newGoHandlerWithMock(t *testing.T, git *MockGitClient) *gorelease.Gorelease {
	t.Helper()
	h, err := gorelease.New(git)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return h
}

// MockGitClient for testing
type MockGitClient struct {
	latestTag string
}

func (m *MockGitClient) GetLatestTag() (string, error) {
	return m.latestTag, nil
}
