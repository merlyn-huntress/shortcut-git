package main

import (
	"strings"
	"testing"
)

func TestParseFrontmatter(t *testing.T) {
	input := `---
title: "My Story"
informational_shortcut_id: 12345
informational_owner: "Jane Smith"
informational_status: "In Progress"
informational_github_pr_urls:
  - https://github.com/org/repo/pull/1
  - https://github.com/org/repo/pull/2
---

This is the body.
`
	fm, body, err := parseFrontmatter(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fm.Title != "My Story" {
		t.Errorf("title = %q, want %q", fm.Title, "My Story")
	}
	if fm.Fields["informational_shortcut_id"] != 12345 {
		t.Errorf("shortcut_id = %v, want 12345", fm.Fields["informational_shortcut_id"])
	}
	if fm.Fields["informational_owner"] != "Jane Smith" {
		t.Errorf("owner = %v, want 'Jane Smith'", fm.Fields["informational_owner"])
	}
	urls, ok := fm.Fields["informational_github_pr_urls"].([]any)
	if !ok || len(urls) != 2 {
		t.Errorf("github_pr_urls = %v, want 2-element list", fm.Fields["informational_github_pr_urls"])
	}
	if !strings.HasPrefix(body, "This is the body.") {
		t.Errorf("body = %q, want to start with 'This is the body.'", body)
	}
}

func TestParseFrontmatterNoFrontmatter(t *testing.T) {
	input := "Just a plain body.\n"
	fm, body, err := parseFrontmatter(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fm.Title != "" {
		t.Errorf("title = %q, want empty", fm.Title)
	}
	if body != input {
		t.Errorf("body = %q, want %q", body, input)
	}
}

func TestParseFrontmatterNewEntityMinimal(t *testing.T) {
	input := `---
title: "New story"
---

Description here.
`
	fm, body, err := parseFrontmatter(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fm.Title != "New story" {
		t.Errorf("title = %q, want %q", fm.Title, "New story")
	}
	if _, ok := fm.Fields["informational_shortcut_id"]; ok {
		t.Error("new entity should not have informational_shortcut_id")
	}
	if !strings.HasPrefix(body, "Description here.") {
		t.Errorf("body = %q", body)
	}
}

func TestRenderFrontmatter(t *testing.T) {
	fm := &Frontmatter{
		Title: "My Story",
		Fields: map[string]any{
			"title":                        "My Story",
			"informational_shortcut_id":    12345,
			"informational_shortcut_url":   "https://app.shortcut.com/test/story/12345",
			"informational_owner":          "Jane Smith",
			"informational_status":         "In Progress",
			"informational_last_updated_at": "2026-04-10T14:30:00Z",
			"informational_github_pr_urls": []any{
				"https://github.com/org/repo/pull/1",
			},
		},
	}
	result := renderFrontmatter(fm, "Body text.\n")

	if !strings.HasPrefix(result, "---\ntitle: ") {
		t.Error("should start with ---\\ntitle:")
	}
	if !strings.Contains(result, "informational_shortcut_id: \"12345\"") {
		t.Errorf("should contain quoted informational_shortcut_id, got:\n%s", result)
	}
	if !strings.Contains(result, "informational_owner: \"Jane Smith\"") {
		t.Error("should contain informational_owner")
	}
	if !strings.Contains(result, "  - \"https://github.com/org/repo/pull/1\"") {
		t.Errorf("should contain quoted pr url list item, got:\n%s", result)
	}
	if !strings.HasSuffix(result, "Body text.\n") {
		t.Errorf("should end with body, got: %q", result[len(result)-30:])
	}
}

func TestRenderFrontmatterMinimal(t *testing.T) {
	fm := &Frontmatter{
		Title: "New story",
		Fields: map[string]any{
			"title": "New story",
		},
	}
	result := renderFrontmatter(fm, "Description.\n")
	expected := "---\ntitle: \"New story\"\n---\n\nDescription.\n"
	if result != expected {
		t.Errorf("got:\n%s\nwant:\n%s", result, expected)
	}
}

func TestRoundTrip(t *testing.T) {
	original := `---
title: "Test Story"
informational_shortcut_id: 99999
informational_status: Done
---

Some description.
`
	fm, body, err := parseFrontmatter(original)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	result := renderFrontmatter(fm, body)

	fm2, body2, err := parseFrontmatter(result)
	if err != nil {
		t.Fatalf("re-parse error: %v", err)
	}
	if fm2.Title != fm.Title {
		t.Errorf("title mismatch: %q vs %q", fm2.Title, fm.Title)
	}
	if !strings.Contains(body2, "Some description.") {
		t.Errorf("body lost: %q", body2)
	}
}
