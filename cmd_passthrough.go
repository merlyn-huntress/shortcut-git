package main

import (
	"flag"
	"fmt"
	"os"
	"time"
)

func cmdAdd(args []string) error {
	root, err := findRepoRoot(".")
	if err != nil {
		return err
	}
	return gitPassthrough(root, append([]string{"add"}, args...)...)
}

func cmdCommit(args []string) error {
	root, err := findRepoRoot(".")
	if err != nil {
		return err
	}
	// Auto-rename files to match title frontmatter before committing
	if err := autoRenameAll(root); err != nil {
		return fmt.Errorf("auto-rename: %w", err)
	}
	return gitPassthrough(root, append([]string{"commit"}, args...)...)
}

func cmdDiff(args []string) error {
	root, err := findRepoRoot(".")
	if err != nil {
		return err
	}
	return gitPassthrough(root, append([]string{"diff"}, args...)...)
}

func cmdLog(args []string) error {
	root, err := findRepoRoot(".")
	if err != nil {
		return err
	}
	return gitPassthrough(root, append([]string{"log"}, args...)...)
}

func cmdCheckout(args []string) error {
	root, err := findRepoRoot(".")
	if err != nil {
		return err
	}
	// B9: Only support the file-restore form: checkout -- <path>
	hasSeparator := false
	for _, arg := range args {
		if arg == "--" {
			hasSeparator = true
			break
		}
	}
	if !hasSeparator {
		return fmt.Errorf("usage: shortcut-git checkout -- <path>\n(only file restore is supported, not branch switching)")
	}
	return gitPassthrough(root, append([]string{"checkout"}, args...)...)
}

func cmdReset(args []string) error {
	root, err := findRepoRoot(".")
	if err != nil {
		return err
	}

	fs := flag.NewFlagSet("reset", flag.ContinueOnError)
	soft := fs.Bool("soft", false, "undo last commit, keep changes staged")
	hard := fs.Bool("hard", false, "discard all local changes")
	if err := fs.Parse(args); err != nil {
		return err
	}

	switch {
	case *soft:
		return gitPassthrough(root, "reset", "--soft", "HEAD~1")
	case *hard:
		return gitPassthrough(root, "reset", "--hard", "HEAD")
	default:
		// Default: unstage (mixed reset)
		remaining := fs.Args()
		return gitPassthrough(root, append([]string{"reset", "HEAD"}, remaining...)...)
	}
}

func cmdStatus(args []string) error {
	root, err := findRepoRoot(".")
	if err != nil {
		return err
	}

	// Show fetch staleness
	cfg, err := loadRepoConfig(root)
	if err == nil && cfg.LastFetchAt != nil {
		elapsed := time.Since(*cfg.LastFetchAt)
		fmt.Fprintf(os.Stderr, "Last fetch: %s ago\n\n", formatDuration(elapsed))
	}

	return gitPassthrough(root, append([]string{"status"}, args...)...)
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}
