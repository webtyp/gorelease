package main

import (
	"fmt"
	gitmod "webtyp.com/git"
	"os"

	"webtyp.com/gorelease"
	keyring "webtyp.com/keyring/auto"
)

func main() {
	usage := func() {
		fmt.Fprintf(os.Stderr, `gorelease - Create GitHub Release with cross-platform binaries

Usage:
    gorelease [tag]
    gorelease image [tag]
    gorelease version

Arguments:
    tag        Tag name (optional, uses latest tag if not provided)

Commands:
    image [tag] — build the webtyp app's container image (linux/amd64), push it to ghcr.io and print it pinned by digest. Needs Docker and 'gh auth refresh -s write:packages' once.
    version    Print GORELEASE_VERSION and exit

Publication:
    If the current repository is PRIVATE, gorelease automatically attempts to
    publish the release to a PUBLIC repository named after the current directory.
    E.g. from ~/work/myapp with origin PRIVATE: publishes to <owner>/myapp.

Features:
    - Automatic target resolution (private source -> public distribution).
    - SHA256 checksums generation and upload.
    - Version injection (main.Version) and binary optimization (-s -w -trimpath).

Examples:
    gorelease
    gorelease v1.2.3
    gorelease version

`)
	}

	// Handle version subcommand before ParseReleaseArgs
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Printf("GORELEASE_VERSION=%s\n", gorelease.Version)
		os.Exit(0)
	}

	if len(os.Args) > 1 && os.Args[1] == "image" {
		git, err := gitmod.NewGit()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}
		g, err := gorelease.New(git)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}

		var tag string
		if len(os.Args) > 2 {
			tag = os.Args[2]
		}

		img, err := g.BuildImage(tag, gorelease.AppDeps{})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println(img.Reference())
		os.Exit(0)
	}

	tag, isHelp := gorelease.ParseReleaseArgs(os.Args)

	if isHelp {
		usage()
		os.Exit(0)
	}

	git, err := gitmod.NewGit()
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	log := func(args ...any) { fmt.Println(args...) }

	// In CI the runner provides GH_TOKEN/GITHUB_TOKEN; keyring is unavailable.
	// When a token is present we bypass the keyring-based OAuth device flow
	// and let `gh` authenticate via the environment variable.
	ghToken := os.Getenv("GH_TOKEN")
	if ghToken == "" {
		ghToken = os.Getenv("GITHUB_TOKEN")
	}

	var gh *gitmod.GitHub
	if ghToken != "" {
		// CI mode: gh CLI will pick up GH_TOKEN automatically.
		gh = &gitmod.GitHub{}
		gh.SetLog(log)
		// Still allow git auth retries to use the token-backed gh session.
		// Fallback to no-op retrier if keyring is unavailable.
		if kr, err := keyring.NewKeyring("gorelease"); err == nil {
			auth := gitmod.NewGitHubOAuth()
			auth.SetStore(kr)
			git.SetAuthRetrier(auth)
		}
	} else {
		auth := gitmod.NewGitHubOAuth()
		kr, err := keyring.NewKeyring("gorelease")
		if err != nil {
			fmt.Println("Error:", err)
			os.Exit(1)
		}
		auth.SetStore(kr)
		git.SetAuthRetrier(auth)

		gh, err = gitmod.NewGitHub(log, kr)
		if err != nil {
			fmt.Println("GitHub error:", err)
			os.Exit(1)
		}
	}

	goHandler, err := gorelease.New(git)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	goHandler.SetLog(log)
	goHandler.SetConsoleOutput(func(s string) { fmt.Println(s) })

	if err := goHandler.ReleaseOnly(tag, gh); err != nil {
		fmt.Println("Release failed:", err)
		os.Exit(1)
	}
}
