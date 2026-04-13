package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveAndLoadRemoteState(t *testing.T) {
	dir := t.TempDir()
	state := &RemoteEntityState{
		ID:          12345,
		EntityType:  "story",
		UpdatedAt:   time.Date(2026, 4, 10, 14, 30, 0, 0, time.UTC),
		ContentHash: "sha256:abc123",
		Name:        "Test Story",
		Description: "A description.",
	}

	// Need to create .shortcut-git dir structure
	sgDir := filepath.Join(dir, ".shortcut-git")
	os.MkdirAll(sgDir, 0755)

	if err := saveRemoteState(dir, state); err != nil {
		t.Fatalf("save error: %v", err)
	}

	loaded, err := loadRemoteState(dir, "story", 12345)
	if err != nil {
		t.Fatalf("load error: %v", err)
	}

	if loaded.ID != 12345 {
		t.Errorf("ID = %d, want 12345", loaded.ID)
	}
	if loaded.Name != "Test Story" {
		t.Errorf("Name = %q, want %q", loaded.Name, "Test Story")
	}
	if loaded.ContentHash != "sha256:abc123" {
		t.Errorf("ContentHash = %q, want %q", loaded.ContentHash, "sha256:abc123")
	}
}

func TestContentHash(t *testing.T) {
	h1 := contentHash(map[string]string{"name": "foo", "description": "bar"})
	h2 := contentHash(map[string]string{"name": "foo", "description": "bar"})
	h3 := contentHash(map[string]string{"name": "foo", "description": "baz"})

	if h1 != h2 {
		t.Error("same input should produce same hash")
	}
	if h1 == h3 {
		t.Error("different input should produce different hash")
	}
	if len(h1) < 10 {
		t.Errorf("hash too short: %q", h1)
	}
}
