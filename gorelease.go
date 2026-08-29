package gorelease

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/tinywasm/command"
)

// Version is injected at build time via -ldflags="-X main.Version=...".
// It is also used by `gorelease version` output.
var Version = "v0.0.0"

// CrossTarget represents a compilation target platform.
type CrossTarget struct{ GOOS, GOARCH string }

// GitClient is the minimal git interface needed by ReleaseOnly.
type GitClient interface {
	GetLatestTag() (string, error)
}

// GitHubClient is the minimal GitHub interface needed by ReleaseOnly.
type GitHubClient interface {
	RepoInfo(repoRef string) (owner, name, visibility string, err error)
	CreateRelease(tag string, assets []string, targetRepo string) (string, error)
}

// Gorelease orchestrates cross-compilation and GitHub Release creation.
type Gorelease struct {
	rootDir        string
	git            GitClient
	log            func(...any)
	consoleOutput  func(string)
	crossCompileFn func(tmpDir string, cmds []string, targets []CrossTarget, repoDir string) ([]string, error)
}

// New creates a new Gorelease handler.
func New(git GitClient) (*Gorelease, error) {
	if _, err := command.Run("go", "version"); err != nil {
		return nil, fmt.Errorf("go is not installed or not in PATH: %w", err)
	}
	return &Gorelease{
		rootDir:       ".",
		git:           git,
		log:           func(...any) {},
		consoleOutput: func(s string) { fmt.Println(s) },
	}, nil
}

// SetRootDir sets the root directory for operations.
func (g *Gorelease) SetRootDir(path string) { g.rootDir = path }

// SetLog sets the logger function.
func (g *Gorelease) SetLog(fn func(...any)) {
	if fn != nil {
		g.log = fn
	}
}

// SetConsoleOutput sets the function for console output.
func (g *Gorelease) SetConsoleOutput(fn func(string)) { g.consoleOutput = fn }

// SetCrossCompileFn sets a custom cross-compile function for testing.
func (g *Gorelease) SetCrossCompileFn(fn func(tmpDir string, cmds []string, targets []CrossTarget, repoDir string) ([]string, error)) {
	g.crossCompileFn = fn
}

// ParseReleaseArgs parses gorelease CLI: gorelease [tag]
// Returns tag (may be empty) and isHelp.
func ParseReleaseArgs(args []string) (tag string, isHelp bool) {
	if len(args) > 1 {
		arg := strings.ToLower(args[1])
		switch arg {
		case "help", "-help", "--help", "h", "-h", "?", "-?":
			return "", true
		}
		tag = args[1]
	}
	return
}

// ReleaseOnly creates a GitHub Release with cross-platform binaries for an existing tag.
// If tag is empty, it uses the latest tag from git.GetLatestTag().
func (g *Gorelease) ReleaseOnly(tag string, gh GitHubClient) error {
	if tag == "" {
		var err error
		tag, err = g.git.GetLatestTag()
		if err != nil {
			return fmt.Errorf("failed to get latest tag: %w", err)
		}
		if tag == "" {
			return fmt.Errorf("no tags found in repository")
		}
	}

	absRoot, err := filepath.Abs(g.rootDir)
	if err != nil {
		return fmt.Errorf("failed to get absolute path of root dir: %w", err)
	}

	target, err := g.resolvePublishTarget(filepath.Base(absRoot), gh)
	if err != nil {
		return err
	}

	cmds, err := g.listCmdDirs(g.rootDir)
	if err != nil {
		return err
	}
	if len(cmds) == 0 {
		return fmt.Errorf("no cmd/ found in %s", g.rootDir)
	}

	tmpDir, err := os.MkdirTemp("", "gorelease-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	var assets []string
	if g.crossCompileFn != nil {
		assets, err = g.crossCompileFn(tmpDir, cmds, DefaultTargets(), g.rootDir)
	} else {
		assets, err = g.CrossCompileWithTag(tmpDir, cmds, DefaultTargets(), g.rootDir, tag)
	}
	if err != nil {
		return fmt.Errorf("cross-compilation failed: %w", err)
	}

	checksumsPath := filepath.Join(tmpDir, "checksums.txt")
	f, err := os.Create(checksumsPath)
	if err != nil {
		return fmt.Errorf("failed to create checksums.txt: %w", err)
	}
	for _, asset := range assets {
		h := sha256.New()
		af, err := os.Open(asset)
		if err != nil {
			f.Close()
			return fmt.Errorf("failed to open asset for checksum: %w", err)
		}
		if _, err := io.Copy(h, af); err != nil {
			af.Close()
			f.Close()
			return fmt.Errorf("failed to calculate checksum: %w", err)
		}
		af.Close()
		fmt.Fprintf(f, "%x  %s\n", h.Sum(nil), filepath.Base(asset))
	}
	f.Close()
	assets = append(assets, checksumsPath)

	url, err := gh.CreateRelease(tag, assets, target)
	if err != nil {
		return fmt.Errorf("failed to create GitHub release: %w", err)
	}

	g.consoleOutput(fmt.Sprintf("✅ Release → %s", url))
	return nil
}

// DefaultTargets returns the standard set of platforms for release.
func DefaultTargets() []CrossTarget {
	return []CrossTarget{
		{"linux", "amd64"},
		{"linux", "arm64"},
		{"darwin", "arm64"},
		{"darwin", "amd64"},
		{"windows", "amd64"},
	}
}

func (g *Gorelease) resolvePublishTarget(folderName string, gh GitHubClient) (string, error) {
	owner, _, visibility, err := gh.RepoInfo("")
	if err != nil {
		return "", nil
	}
	if visibility == "PUBLIC" {
		return "", nil
	}
	candidate := owner + "/" + folderName
	_, _, candidateVisibility, err := gh.RepoInfo(candidate)
	if err != nil {
		return "", fmt.Errorf("origin is private, but derived repo %s does not exist or is not accessible. A public repository is required for distribution: %w", candidate, err)
	}
	if candidateVisibility != "PUBLIC" {
		return "", fmt.Errorf("origin is private, and derived repo %s is also %s. A PUBLIC repository is required for distribution", candidate, candidateVisibility)
	}
	return candidate, nil
}

func crossBuildArgs(tag, cmd, outputPath string) []string {
	ldflags := "-s -w"
	if tag != "" {
		ldflags += fmt.Sprintf(" -X main.Version=%s", tag)
	}
	return []string{
		"build",
		"-o", outputPath,
		"-trimpath",
		"-ldflags=" + ldflags,
		"./cmd/" + cmd,
	}
}

// CrossCompileWithTag builds the specified commands for multiple platforms with version injection.
func (g *Gorelease) CrossCompileWithTag(tmpDir string, cmds []string, targets []CrossTarget, repoDir, tag string) ([]string, error) {
	var assets []string
	for _, target := range targets {
		for _, cmd := range cmds {
			name := cmd
			suffix := fmt.Sprintf("-%s-%s", target.GOOS, target.GOARCH)
			if target.GOOS == "windows" {
				suffix += ".exe"
			}
			outputName := name + suffix
			outputPath := filepath.Join(tmpDir, outputName)
			args := crossBuildArgs(tag, cmd, outputPath)
			buildCmd := command.Exec("go", args...)
			buildCmd.Dir = repoDir
			buildCmd.Env = append(os.Environ(),
				"CGO_ENABLED=0",
				"GOOS="+target.GOOS,
				"GOARCH="+target.GOARCH,
			)
			outputBytes, err := buildCmd.CombinedOutput()
			if err != nil {
				return nil, fmt.Errorf("failed to build %s for %s/%s: %w\nOutput: %s", cmd, target.GOOS, target.GOARCH, err, string(outputBytes))
			}
			assets = append(assets, outputPath)
		}
	}
	return assets, nil
}

// CrossCompile builds the specified commands for multiple platforms.
func CrossCompile(tmpDir string, cmds []string, targets []CrossTarget, repoDir string) ([]string, error) {
	g := &Gorelease{}
	return g.CrossCompileWithTag(tmpDir, cmds, targets, repoDir, "")
}

// listCmdDirs returns the names of the subdirectories in cmd/.
func (g *Gorelease) listCmdDirs(rootDir string) ([]string, error) {
	cmdDir := filepath.Join(rootDir, "cmd")
	if _, err := os.Stat(cmdDir); os.IsNotExist(err) {
		return nil, nil
	}
	entries, err := os.ReadDir(cmdDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read cmd directory: %w", err)
	}
	var commands []string
	for _, entry := range entries {
		if entry.IsDir() {
			commands = append(commands, entry.Name())
		}
	}
	return commands, nil
}
