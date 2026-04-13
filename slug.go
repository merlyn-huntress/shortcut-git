package main

import (
	"fmt"
	"regexp"
	"strings"
)

var nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(name string) string {
	s := strings.ToLower(name)
	s = nonAlnum.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 80 {
		s = s[:80]
		s = strings.TrimRight(s, "-")
	}
	return s
}

func epicDirname(name string) string {
	return slugify(name)
}

// storyFilename returns "NNN-slug.md" where NNN is the zero-padded position index.
func storyFilename(name string, positionIndex int) string {
	return fmt.Sprintf("%03d-%s.md", positionIndex, slugify(name))
}

// storyFilenameUnordered returns a story filename without a position prefix.
// Used when creating new stories locally (no position yet).
func storyFilenameUnordered(name string) string {
	return slugify(name) + ".md"
}
