package main

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type epicWithStories struct {
	epic    *Epic
	stories []Story
}

func cmdClone(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: shortcut-git clone <objective_url>")
	}

	// Parse the objective URL
	workspaceSlug, objectiveID, err := parseObjectiveURL(args[0])
	if err != nil {
		return err
	}

	// Load API token
	token, err := loadAPIToken()
	if err != nil {
		return err
	}

	client := NewShortcutClient(token)

	// Fetch objective
	fmt.Fprintf(os.Stderr, "Fetching objective %d...\n", objectiveID)
	obj, err := client.GetObjective(objectiveID)
	if err != nil {
		return fmt.Errorf("fetching objective: %w", err)
	}

	// Fetch workflows (for resolving state names)
	fmt.Fprintf(os.Stderr, "Fetching workflows...\n")
	workflows, err := client.GetWorkflows()
	if err != nil {
		return fmt.Errorf("fetching workflows: %w", err)
	}
	stateMap := buildWorkflowStateMap(workflows)

	// Fetch members (for resolving owner names)
	fmt.Fprintf(os.Stderr, "Fetching members...\n")
	members, err := client.GetMembers()
	if err != nil {
		return fmt.Errorf("fetching members: %w", err)
	}
	memberMap := buildMemberMap(members)

	// Search for epics under this objective
	fmt.Fprintf(os.Stderr, "Searching epics for objective %d...\n", objectiveID)
	epics, err := client.SearchEpicsByObjective(objectiveID)
	if err != nil {
		return fmt.Errorf("searching epics: %w", err)
	}

	// Fetch full epic details and stories for each epic
	var epicData []epicWithStories
	totalStories := 0
	for i := range epics {
		// Fetch full epic (list endpoint may return slim without description)
		fmt.Fprintf(os.Stderr, "Fetching epic %d (%s)...\n", epics[i].ID, epics[i].Name)
		fullEpic, err := client.GetEpic(epics[i].ID)
		if err != nil {
			return fmt.Errorf("fetching epic %d: %w", epics[i].ID, err)
		}

		// Get story IDs from the epic's stories endpoint (slim)
		fmt.Fprintf(os.Stderr, "Fetching stories for epic %d...\n", epics[i].ID)
		slimStories, err := client.GetEpicStories(epics[i].ID)
		if err != nil {
			return fmt.Errorf("fetching stories for epic %d: %w", epics[i].ID, err)
		}

		// Fetch each story individually for full data (including description)
		var fullStories []Story
		for _, slim := range slimStories {
			fullStory, err := client.GetStory(slim.ID)
			if err != nil {
				return fmt.Errorf("fetching story %d: %w", slim.ID, err)
			}
			fullStories = append(fullStories, *fullStory)
		}

		epicData = append(epicData, epicWithStories{epic: fullEpic, stories: fullStories})
		totalStories += len(fullStories)
	}

	// Create checkout directory
	dirName := slugify(obj.Name)
	if dirName == "" {
		dirName = fmt.Sprintf("objective-%d", objectiveID)
	}
	repoRoot, err := filepath.Abs(dirName)
	if err != nil {
		return err
	}

	// B17: Refuse if directory already exists
	if _, err := os.Stat(repoRoot); err == nil {
		return fmt.Errorf("directory %s already exists. Remove it first or choose a different location", dirName)
	}

	if err := os.MkdirAll(repoRoot, 0755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}

	// B18: Clean up on failure
	cloneOK := false
	defer func() {
		if !cloneOK {
			os.RemoveAll(repoRoot)
		}
	}()

	// Initialize git repo
	fmt.Fprintf(os.Stderr, "Initializing repository...\n")
	if err := gitInit(repoRoot); err != nil {
		return fmt.Errorf("git init: %w", err)
	}

	// Write .gitignore to exclude .shortcut-git internals
	if err := os.WriteFile(filepath.Join(repoRoot, ".gitignore"), []byte(".shortcut-git/\n"), 0644); err != nil {
		return fmt.Errorf("writing .gitignore: %w", err)
	}

	// Detect team_id from stories (use the first non-empty team_id found)
	teamID := detectTeamID(epicData)

	// Save repo config
	now := time.Now().UTC()
	cfg := &RepoConfig{
		ObjectiveID:          objectiveID,
		WorkspaceSlug:        workspaceSlug,
		APIBase:              "https://api.app.shortcut.com/api/v3",
		WorkflowStates:       stateMap,
		Members:              memberMap,
		DefaultWorkflowState: findFirstUnstartedState(workflows),
		TeamID:               teamID,
		LastFetchAt:          &now,
	}
	if err := saveRepoConfig(repoRoot, cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	// Write _objective.md (underscore prefix so it sorts first)
	objFM := buildObjectiveFrontmatter(obj, cfg)
	objContent := renderFrontmatter(objFM, ensureTrailingNewline(obj.Description))
	if err := os.WriteFile(filepath.Join(repoRoot, "_objective.md"), []byte(objContent), 0644); err != nil {
		return err
	}
	if err := saveRemoteState(repoRoot, stateFromObjective(obj)); err != nil {
		return err
	}

	// Write epic dirs and story files
	for _, ed := range epicData {
		epicDir := filepath.Join(repoRoot, epicDirname(ed.epic.Name))
		if err := os.MkdirAll(epicDir, 0755); err != nil {
			return err
		}

		// Write _epic.md (underscore prefix so it sorts first in dir)
		epicFM := buildEpicFrontmatter(ed.epic, cfg)
		epicContent := renderFrontmatter(epicFM, ensureTrailingNewline(ed.epic.Description))
		if err := os.WriteFile(filepath.Join(epicDir, "_epic.md"), []byte(epicContent), 0644); err != nil {
			return err
		}
		if err := saveRemoteState(repoRoot, stateFromEpic(ed.epic)); err != nil {
			return err
		}

		// Sort stories by position (Shortcut's "User Defined" ordering)
		sort.Slice(ed.stories, func(i, j int) bool {
			return ed.stories[i].Position < ed.stories[j].Position
		})

		// Write story files with NNN- prefix to preserve ordering
		for i := range ed.stories {
			story := &ed.stories[i]
			storyFM := buildStoryFrontmatter(story, cfg)
			storyContent := renderFrontmatter(storyFM, ensureTrailingNewline(story.Description))
			filename := storyFilename(story.Name, i+1)
			if err := os.WriteFile(filepath.Join(epicDir, filename), []byte(storyContent), 0644); err != nil {
				return err
			}
			if err := saveRemoteState(repoRoot, stateFromStory(story)); err != nil {
				return err
			}
		}
	}

	// Git add and commit
	if _, err := git(repoRoot, "add", "."); err != nil {
		return fmt.Errorf("git add: %w", err)
	}
	if _, err := git(repoRoot, "commit", "-m", fmt.Sprintf("shortcut-git: initial clone of objective %d", objectiveID)); err != nil {
		return fmt.Errorf("git commit: %w", err)
	}

	cloneOK = true
	fmt.Printf("Cloned objective %d (%s) with %d epics, %d stories into %s/\n",
		objectiveID, obj.Name, len(epics), totalStories, dirName)
	return nil
}

func parseObjectiveURL(rawURL string) (workspaceSlug string, objectiveID int, err error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", 0, fmt.Errorf("invalid URL: %w", err)
	}

	if u.Host != "app.shortcut.com" {
		return "", 0, fmt.Errorf("expected app.shortcut.com URL, got %s", u.Host)
	}

	// Path: /{workspace}/objective/{id} or /{workspace}/objective/{id}/{slug}
	// Also handle legacy /milestone/ path
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 3 {
		return "", 0, fmt.Errorf("invalid objective URL: expected /{workspace}/objective/{id}, got %s", u.Path)
	}

	workspaceSlug = parts[0]
	if parts[1] != "objective" && parts[1] != "milestone" {
		return "", 0, fmt.Errorf("invalid objective URL: expected /objective/ or /milestone/ in path, got /%s/", parts[1])
	}

	objectiveID, err = strconv.Atoi(parts[2])
	if err != nil {
		return "", 0, fmt.Errorf("invalid objective ID %q: %w", parts[2], err)
	}

	return workspaceSlug, objectiveID, nil
}

func buildWorkflowStateMap(workflows []Workflow) map[string]string {
	m := make(map[string]string)
	for _, w := range workflows {
		for _, s := range w.States {
			m[strconv.Itoa(s.ID)] = s.Name
		}
	}
	return m
}

func buildMemberMap(members []Member) map[string]string {
	m := make(map[string]string)
	for _, mem := range members {
		m[mem.ID] = mem.Profile.Name
	}
	return m
}

func resolveOwnerNames(ownerIDs []string, members map[string]string) string {
	if len(ownerIDs) == 0 {
		return ""
	}
	var names []string
	for _, id := range ownerIDs {
		if name, ok := members[id]; ok {
			names = append(names, name)
		} else {
			names = append(names, id)
		}
	}
	return strings.Join(names, ", ")
}

func resolveStateName(stateID int, states map[string]string) string {
	if name, ok := states[strconv.Itoa(stateID)]; ok {
		return name
	}
	return fmt.Sprintf("unknown-%d", stateID)
}

func buildObjectiveFrontmatter(obj *Objective, cfg *RepoConfig) *Frontmatter {
	fields := map[string]any{
		"title":                         obj.Name,
		"informational_shortcut_id":     obj.ID,
		"informational_shortcut_url":    obj.AppURL,
		"informational_owner":           resolveOwnerNames(obj.OwnerIDs, cfg.Members),
		"informational_status":          obj.State,
		"informational_last_updated_at": obj.UpdatedAt.UTC().Format(time.RFC3339),
	}
	return &Frontmatter{Title: obj.Name, Fields: fields}
}

func buildEpicFrontmatter(epic *Epic, cfg *RepoConfig) *Frontmatter {
	fields := map[string]any{
		"title":                         epic.Name,
		"informational_shortcut_id":     epic.ID,
		"informational_shortcut_url":    epic.AppURL,
		"informational_owner":           resolveOwnerNames(epic.OwnerIDs, cfg.Members),
		"informational_status":          epic.State,
		"informational_last_updated_at": epic.UpdatedAt.UTC().Format(time.RFC3339),
	}
	return &Frontmatter{Title: epic.Name, Fields: fields}
}

func buildStoryFrontmatter(story *Story, cfg *RepoConfig) *Frontmatter {
	fields := map[string]any{
		"title":                         story.Name,
		"informational_shortcut_id":     story.ID,
		"informational_shortcut_url":    story.AppURL,
		"informational_owner":           resolveOwnerNames(story.OwnerIDs, cfg.Members),
		"informational_status":          resolveStateName(story.WorkflowStateID, cfg.WorkflowStates),
		"informational_type":            story.StoryType,
		"informational_last_updated_at": story.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if story.Archived {
		fields["informational_archived"] = true
	}
	// Add PR URLs if present
	var prURLs []any
	for _, pr := range story.PullRequests {
		if pr.URL != "" {
			prURLs = append(prURLs, pr.URL)
		}
	}
	if len(prURLs) > 0 {
		fields["informational_github_pr_urls"] = prURLs
	}
	return &Frontmatter{Title: story.Name, Fields: fields}
}

func detectTeamID(epicData []epicWithStories) string {
	// Check epics first
	for _, ed := range epicData {
		if ed.epic.TeamID != "" {
			return ed.epic.TeamID
		}
	}
	// Check stories (try both team_id and group_id — API uses both)
	for _, ed := range epicData {
		for _, s := range ed.stories {
			if s.TeamID != "" {
				return s.TeamID
			}
			if s.GroupID != "" {
				return s.GroupID
			}
		}
	}
	return ""
}

func findFirstUnstartedState(workflows []Workflow) int {
	for _, w := range workflows {
		for _, s := range w.States {
			if s.Type == "unstarted" {
				return s.ID
			}
		}
	}
	// Fallback: first state of first workflow
	if len(workflows) > 0 && len(workflows[0].States) > 0 {
		return workflows[0].States[0].ID
	}
	return 0
}

func ensureTrailingNewline(s string) string {
	if s == "" {
		return "\n"
	}
	if !strings.HasSuffix(s, "\n") {
		return s + "\n"
	}
	return s
}
