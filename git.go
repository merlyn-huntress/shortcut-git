package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func gitCmd(repoRoot string, args ...string) *exec.Cmd {
	gitDir := filepath.Join(repoRoot, ".shortcut-git", "repo")
	fullArgs := append([]string{"--git-dir=" + gitDir, "--work-tree=" + repoRoot}, args...)
	cmd := exec.Command("git", fullArgs...)
	cmd.Dir = repoRoot
	return cmd
}

// git runs a git command and captures stdout. Returns stderr in error on failure.
func git(repoRoot string, args ...string) (string, error) {
	cmd := gitCmd(repoRoot, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("git %s: %s", args[0], strings.TrimSpace(stderr.String()))
	}
	return strings.TrimRight(stdout.String(), "\n"), nil
}

// gitPassthrough runs a git command with stdout/stderr piped to the terminal.
func gitPassthrough(repoRoot string, args ...string) error {
	cmd := gitCmd(repoRoot, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// gitInit creates a bare git repo at .shortcut-git/repo and configures the work tree.
func gitInit(repoRoot string) error {
	gitDir := filepath.Join(repoRoot, ".shortcut-git", "repo")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		return err
	}

	cmd := exec.Command("git", "init", "--bare", gitDir)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git init --bare: %s", strings.TrimSpace(stderr.String()))
	}

	// Configure work tree to point to the repo root
	_, err := git(repoRoot, "config", "core.worktree", repoRoot)
	return err
}

// findRepoRoot walks up from dir looking for .shortcut-git/config.json.
func findRepoRoot(dir string) (string, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		configPath := filepath.Join(dir, ".shortcut-git", "config.json")
		if _, err := os.Stat(configPath); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("not a shortcut-git repository (no .shortcut-git/config.json found)")
		}
		dir = parent
	}
}
