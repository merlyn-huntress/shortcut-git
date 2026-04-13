package main

import (
	"fmt"
	"os"
	"time"
)

func cmdFetch(args []string) error {
	root, err := findRepoRoot(".")
	if err != nil {
		return err
	}
	_, err = doFetch(root)
	return err
}

// FetchResult summarizes what changed on the remote.
type FetchResult struct {
	Updated []FetchChange
	Added   []FetchChange
	Removed []FetchChange
}

type FetchChange struct {
	EntityType string
	ID         int
	Name       string
}

func doFetch(repoRoot string) (*FetchResult, error) {
	cfg, err := loadRepoConfig(repoRoot)
	if err != nil {
		return nil, err
	}

	token, err := loadAPIToken()
	if err != nil {
		return nil, err
	}
	client := NewShortcutClient(token)

	// Fetch current remote state
	fmt.Fprintf(os.Stderr, "Fetching objective %d...\n", cfg.ObjectiveID)
	obj, err := client.GetObjective(cfg.ObjectiveID)
	if err != nil {
		return nil, fmt.Errorf("fetching objective: %w", err)
	}

	epics, err := client.SearchEpicsByObjective(cfg.ObjectiveID)
	if err != nil {
		return nil, fmt.Errorf("searching epics: %w", err)
	}

	type epicStories struct {
		epic    *Epic
		stories []Story
	}
	var allData []epicStories
	for i := range epics {
		// Fetch full epic data (list endpoint returns slim without description)
		fullEpic, err := client.GetEpic(epics[i].ID)
		if err != nil {
			return nil, fmt.Errorf("fetching epic %d: %w", epics[i].ID, err)
		}

		slimStories, err := client.GetEpicStories(epics[i].ID)
		if err != nil {
			return nil, fmt.Errorf("fetching stories for epic %d: %w", epics[i].ID, err)
		}

		// Fetch each story individually for full data (including description)
		var fullStories []Story
		for _, slim := range slimStories {
			fullStory, err := client.GetStory(slim.ID)
			if err != nil {
				return nil, fmt.Errorf("fetching story %d: %w", slim.ID, err)
			}
			fullStories = append(fullStories, *fullStory)
		}

		allData = append(allData, epicStories{epic: fullEpic, stories: fullStories})
	}

	// Refresh workflows and members
	workflows, err := client.GetWorkflows()
	if err == nil {
		cfg.WorkflowStates = buildWorkflowStateMap(workflows)
	}
	members, err := client.GetMembers()
	if err == nil {
		cfg.Members = buildMemberMap(members)
	}

	result := &FetchResult{}

	// Compare objective
	oldObj, _ := loadRemoteState(repoRoot, "objective", obj.ID)
	newObjState := stateFromObjective(obj)
	if oldObj == nil {
		result.Added = append(result.Added, FetchChange{"objective", obj.ID, obj.Name})
	} else if oldObj.ContentHash != newObjState.ContentHash || oldObj.UpdatedAt != newObjState.UpdatedAt {
		result.Updated = append(result.Updated, FetchChange{"objective", obj.ID, obj.Name})
	}
	if err := saveRemoteState(repoRoot, newObjState); err != nil {
		return nil, err
	}

	// Track all current remote IDs for removal detection
	remoteEpicIDs := map[int]bool{}
	remoteStoryIDs := map[int]bool{}

	for _, ed := range allData {
		remoteEpicIDs[ed.epic.ID] = true

		oldEpic, _ := loadRemoteState(repoRoot, "epic", ed.epic.ID)
		newEpicState := stateFromEpic(ed.epic)
		if oldEpic == nil {
			result.Added = append(result.Added, FetchChange{"epic", ed.epic.ID, ed.epic.Name})
		} else if oldEpic.ContentHash != newEpicState.ContentHash || oldEpic.UpdatedAt != newEpicState.UpdatedAt {
			result.Updated = append(result.Updated, FetchChange{"epic", ed.epic.ID, ed.epic.Name})
		}
		if err := saveRemoteState(repoRoot, newEpicState); err != nil {
			return nil, err
		}

		for i := range ed.stories {
			story := &ed.stories[i]
			remoteStoryIDs[story.ID] = true

			oldStory, _ := loadRemoteState(repoRoot, "story", story.ID)
			newStoryState := stateFromStory(story)
			if oldStory == nil {
				result.Added = append(result.Added, FetchChange{"story", story.ID, story.Name})
			} else if oldStory.ContentHash != newStoryState.ContentHash || oldStory.UpdatedAt != newStoryState.UpdatedAt {
				result.Updated = append(result.Updated, FetchChange{"story", story.ID, story.Name})
			}
			if err := saveRemoteState(repoRoot, newStoryState); err != nil {
				return nil, err
			}
		}
	}

	// Detect removals (entities in local remote/ that no longer exist remotely)
	oldStates, _ := loadAllRemoteStates(repoRoot)
	for _, old := range oldStates {
		switch old.EntityType {
		case "epic":
			if !remoteEpicIDs[old.ID] {
				result.Removed = append(result.Removed, FetchChange{"epic", old.ID, old.Name})
				deleteRemoteState(repoRoot, "epic", old.ID)
			}
		case "story":
			if !remoteStoryIDs[old.ID] {
				result.Removed = append(result.Removed, FetchChange{"story", old.ID, old.Name})
				deleteRemoteState(repoRoot, "story", old.ID)
			}
		}
	}

	// Update last fetch time
	now := time.Now().UTC()
	cfg.LastFetchAt = &now
	if err := saveRepoConfig(repoRoot, cfg); err != nil {
		return nil, err
	}

	// Print summary
	fmt.Printf("Fetched: %d updated, %d added, %d removed\n",
		len(result.Updated), len(result.Added), len(result.Removed))

	return result, nil
}
