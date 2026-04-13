package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var positionPrefixRe = regexp.MustCompile(`^(\d+-)`)

// extractPositionPrefix returns the "NNN-" prefix from a filename, or "" if none.
func extractPositionPrefix(name string) string {
	if m := positionPrefixRe.FindString(name); m != "" {
		return m
	}
	return ""
}

// autoRenameAll checks all tracked markdown files and renames any whose
// filename doesn't match the slugified title from their frontmatter.
func autoRenameAll(repoRoot string) error {
	// Process objective.md (never renamed — always "objective.md")
	// Process epic directories and story files
	entries, err := os.ReadDir(repoRoot)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		epicDir := filepath.Join(repoRoot, entry.Name())

		// Check epic.md title → directory name
		epicMD := filepath.Join(epicDir, "_epic.md")
		newDirName, err := autoRenameEpicDir(repoRoot, epicDir, epicMD)
		if err != nil {
			return err
		}

		// B6: If epic dir was renamed, use the new path for story processing
		if newDirName != "" {
			epicDir = filepath.Join(repoRoot, newDirName)
		}

		// Check story files in this epic dir
		if err := autoRenameStoriesInDir(repoRoot, epicDir); err != nil {
			return err
		}
	}
	return nil
}

// autoRenameEpicDir renames the epic directory if its name doesn't match the slugified title.
// Returns the new directory name if renamed, or "" if no rename was needed.
func autoRenameEpicDir(repoRoot, epicDir, epicMDPath string) (newDirName string, err error) {
	data, err := os.ReadFile(epicMDPath)
	if err != nil {
		return "", nil // no _epic.md, skip
	}
	fm, _, err := parseFrontmatter(string(data))
	if err != nil || fm.Title == "" {
		return "", nil
	}

	expectedDirName := epicDirname(fm.Title)
	currentDirName := filepath.Base(epicDir)

	if currentDirName == expectedDirName {
		return "", nil
	}

	newDir := filepath.Join(filepath.Dir(epicDir), expectedDirName)
	if _, err := os.Stat(newDir); err == nil {
		return "", fmt.Errorf("cannot rename epic directory %q to %q: target already exists (slug collision)", currentDirName, expectedDirName)
	}

	_, err = git(repoRoot, "mv", currentDirName, expectedDirName)
	if err != nil {
		if rerr := os.Rename(epicDir, newDir); rerr != nil {
			return "", rerr
		}
	}
	return expectedDirName, nil
}

func autoRenameStoriesInDir(repoRoot, epicDir string) error {
	entries, err := os.ReadDir(epicDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == "_epic.md" || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		filePath := filepath.Join(epicDir, entry.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}
		fm, _, err := parseFrontmatter(string(data))
		if err != nil || fm.Title == "" {
			continue
		}

		// Preserve existing NNN- prefix if present, otherwise use unordered name
		existingPrefix := extractPositionPrefix(entry.Name())
		var expectedName string
		if existingPrefix != "" {
			expectedName = existingPrefix + slugify(fm.Title) + ".md"
		} else {
			expectedName = storyFilenameUnordered(fm.Title)
		}
		if entry.Name() == expectedName {
			continue
		}

		newPath := filepath.Join(epicDir, expectedName)
		if _, err := os.Stat(newPath); err == nil {
			return fmt.Errorf("cannot rename %q to %q: target already exists (slug collision)", entry.Name(), expectedName)
		}

		// Compute relative paths for git mv
		relOld, _ := filepath.Rel(repoRoot, filePath)
		relNew, _ := filepath.Rel(repoRoot, newPath)

		_, err = git(repoRoot, "mv", relOld, relNew)
		if err != nil {
			// Fallback to OS rename if not tracked yet
			if rerr := os.Rename(filePath, newPath); rerr != nil {
				return rerr
			}
		}
	}
	return nil
}
