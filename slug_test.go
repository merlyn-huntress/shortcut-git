package main

import (
	"strings"
	"testing"
)

func TestSlugify(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Hello World", "hello-world"},
		{"Ensure GCC High tenant accounts always route to old M365 integrations page", "ensure-gcc-high-tenant-accounts-always-route-to-old-m365-integrations-page"},
		{"  Leading/Trailing  ", "leading-trailing"},
		{"special!@#$%chars", "special-chars"},
		{"multiple---hyphens", "multiple-hyphens"},
		{"", ""},
		{"UPPERCASE", "uppercase"},
		{"already-slugified", "already-slugified"},
		{"a" + strings.Repeat("b", 100), "a" + strings.Repeat("b", 79)},
	}
	for _, tt := range tests {
		got := slugify(tt.input)
		if got != tt.want {
			t.Errorf("slugify(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestStoryFilename(t *testing.T) {
	got := storyFilename("My Test Story", 1)
	want := "001-my-test-story.md"
	if got != want {
		t.Errorf("storyFilename(\"My Test Story\", 1) = %q, want %q", got, want)
	}

	got = storyFilename("Another Story", 42)
	want = "042-another-story.md"
	if got != want {
		t.Errorf("storyFilename(\"Another Story\", 42) = %q, want %q", got, want)
	}
}

func TestStoryFilenameUnordered(t *testing.T) {
	got := storyFilenameUnordered("My Test Story")
	want := "my-test-story.md"
	if got != want {
		t.Errorf("storyFilenameUnordered(\"My Test Story\") = %q, want %q", got, want)
	}
}

func TestEpicDirname(t *testing.T) {
	got := epicDirname("My Test Epic")
	if got != "my-test-epic" {
		t.Errorf("epicDirname(\"My Test Epic\") = %q, want %q", got, "my-test-epic")
	}
}
