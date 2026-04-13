package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type RemoteEntityState struct {
	ID          int       `json:"id"`
	EntityType  string    `json:"entity_type"` // "objective", "epic", "story"
	UpdatedAt   time.Time `json:"updated_at"`
	ContentHash string    `json:"content_hash"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	// Store the full raw response for diffing
	RawFields json.RawMessage `json:"raw_fields"`
}

func remoteDir(repoRoot string) string {
	return filepath.Join(repoRoot, ".shortcut-git", "remote")
}

func remoteStatePath(repoRoot, entityType string, id int) string {
	return filepath.Join(remoteDir(repoRoot), fmt.Sprintf("%s-%d.json", entityType, id))
}

func saveRemoteState(repoRoot string, state *RemoteEntityState) error {
	dir := remoteDir(repoRoot)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(remoteStatePath(repoRoot, state.EntityType, state.ID), data, 0644)
}

func loadRemoteState(repoRoot, entityType string, id int) (*RemoteEntityState, error) {
	data, err := os.ReadFile(remoteStatePath(repoRoot, entityType, id))
	if err != nil {
		return nil, err
	}
	var state RemoteEntityState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

func loadAllRemoteStates(repoRoot string) ([]*RemoteEntityState, error) {
	dir := remoteDir(repoRoot)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var states []*RemoteEntityState
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		var state RemoteEntityState
		if err := json.Unmarshal(data, &state); err != nil {
			return nil, err
		}
		states = append(states, &state)
	}
	return states, nil
}

func deleteRemoteState(repoRoot, entityType string, id int) error {
	return os.Remove(remoteStatePath(repoRoot, entityType, id))
}

// contentHash computes a SHA-256 hash of the canonical JSON representation.
func contentHash(v any) string {
	data, _ := json.Marshal(v)
	h := sha256.Sum256(data)
	return fmt.Sprintf("sha256:%x", h)
}

// stateFromObjective creates a RemoteEntityState from an Objective.
func stateFromObjective(obj *Objective) *RemoteEntityState {
	raw, _ := json.Marshal(obj)
	return &RemoteEntityState{
		ID:          obj.ID,
		EntityType:  "objective",
		UpdatedAt:   obj.UpdatedAt,
		ContentHash: contentHash(map[string]string{"name": obj.Name, "description": obj.Description}),
		Name:        obj.Name,
		Description: obj.Description,
		RawFields:   raw,
	}
}

// stateFromEpic creates a RemoteEntityState from an Epic.
func stateFromEpic(epic *Epic) *RemoteEntityState {
	raw, _ := json.Marshal(epic)
	return &RemoteEntityState{
		ID:          epic.ID,
		EntityType:  "epic",
		UpdatedAt:   epic.UpdatedAt,
		ContentHash: contentHash(map[string]string{"name": epic.Name, "description": epic.Description}),
		Name:        epic.Name,
		Description: epic.Description,
		RawFields:   raw,
	}
}

// stateFromStory creates a RemoteEntityState from a Story.
func stateFromStory(story *Story) *RemoteEntityState {
	raw, _ := json.Marshal(story)
	return &RemoteEntityState{
		ID:          story.ID,
		EntityType:  "story",
		UpdatedAt:   story.UpdatedAt,
		ContentHash: contentHash(map[string]string{"name": story.Name, "description": story.Description}),
		Name:        story.Name,
		Description: story.Description,
		RawFields:   raw,
	}
}
