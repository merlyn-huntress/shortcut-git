package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func cmdPull(args []string) error {
	root, err := findRepoRoot(".")
	if err != nil {
		return err
	}

	// B1: Refuse if working tree is dirty
	status, err := git(root, "status", "--porcelain")
	if err != nil {
		return err
	}
	if strings.TrimSpace(status) != "" {
		return fmt.Errorf("working tree is dirty. Commit or stash your changes before pulling.\n%s", status)
	}

	// Run fetch to update .shortcut-git/remote/ with latest API state
	if _, err := doFetch(root); err != nil {
		return err
	}

	cfg, err := loadRepoConfig(root)
	if err != nil {
		return err
	}

	// B2: Compare ALL remote states against actual local files to determine
	// what needs applying. This works even if fetch was run separately before pull.
	remoteStates, err := loadAllRemoteStates(root)
	if err != nil {
		return err
	}

	var infoFiles []string   // files with only informational changes
	var editFiles []string   // files with editable/structural changes
	infoCount := 0
	editCount := 0
	addedCount := 0
	removedCount := 0

	// Track remote entity IDs to detect local files for removed entities
	remoteIDs := map[string]bool{} // "type-id" keys

	for _, state := range remoteStates {
		remoteIDs[fmt.Sprintf("%s-%d", state.EntityType, state.ID)] = true

		filePath, err := findFileForEntity(root, state.EntityType, state.ID)
		if err != nil {
			// Entity exists in remote state but no local file — create it
			if createErr := createLocalFile(root, cfg, state); createErr != nil {
				fmt.Fprintf(os.Stderr, "warning: creating %s %d: %v\n", state.EntityType, state.ID, createErr)
				continue
			}
			// Find the newly created file for staging
			newPath, _ := findFileForEntity(root, state.EntityType, state.ID)
			if newPath != "" {
				editFiles = append(editFiles, newPath)
			}
			addedCount++
			continue
		}

		// File exists — check if it needs updating
		isInfoOnly, changed, updatedPath, err := applyRemoteChange(root, cfg, state, filePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: applying %s %d: %v\n", state.EntityType, state.ID, err)
			continue
		}
		if !changed {
			continue
		}

		// Track the file path (may have changed due to rename)
		path := filePath
		if updatedPath != "" {
			path = updatedPath
		}

		if isInfoOnly {
			infoFiles = append(infoFiles, path)
			infoCount++
		} else {
			editFiles = append(editFiles, path)
			editCount++
		}
	}

	// Detect removed entities: files on disk whose IDs are not in remote state
	removedPaths := findOrphanedFiles(root, remoteIDs)
	for _, rp := range removedPaths {
		relPath, _ := filepath.Rel(root, rp)
		if _, err := git(root, "rm", "-f", relPath); err != nil {
			fmt.Fprintf(os.Stderr, "warning: removing %s: %v\n", relPath, err)
			continue
		}
		removedCount++
	}

	// B4: Commit info-only changes with specific files (not git add .)
	if infoCount > 0 {
		for _, f := range infoFiles {
			relPath, _ := filepath.Rel(root, f)
			git(root, "add", relPath)
		}
		git(root, "commit", "-m", "shortcut-git: sync informational fields")
	}

	// Commit editable/structural changes with specific files
	if editCount > 0 || addedCount > 0 || removedCount > 0 {
		for _, f := range editFiles {
			relPath, _ := filepath.Rel(root, f)
			git(root, "add", relPath)
		}
		// Stage any removals that haven't been committed yet
		if removedCount > 0 {
			git(root, "add", "-u") // stages deletions
		}
		git(root, "commit", "-m", "shortcut-git: pull remote changes")
	}

	fmt.Printf("Pulled: %d info-only updates, %d editable updates, %d added, %d removed\n",
		infoCount, editCount, addedCount, removedCount)
	return nil
}

// applyRemoteChange updates a local file with remote data if it differs.
// Returns (infoOnly, changed, newFilePath, error).
// newFilePath is set if the file was renamed.
func applyRemoteChange(repoRoot string, cfg *RepoConfig, state *RemoteEntityState, filePath string) (infoOnly bool, changed bool, newFilePath string, err error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return false, false, "", err
	}
	fm, localBody, err := parseFrontmatter(string(data))
	if err != nil {
		return false, false, "", err
	}

	// Build new frontmatter from remote state
	newFM := buildFrontmatterFromState(state, cfg)

	// Determine what changed
	titleChanged := fm.Title != newFM.Title
	bodyChanged := strings.TrimSpace(localBody) != strings.TrimSpace(state.Description)
	infoOnly = !titleChanged && !bodyChanged

	// Check if any informational fields differ
	infoChanged := false
	for _, key := range informationalFieldOrder {
		newVal, hasNew := newFM.Fields[key]
		oldVal, hasOld := fm.Fields[key]
		if hasNew != hasOld || fmt.Sprintf("%v", newVal) != fmt.Sprintf("%v", oldVal) {
			infoChanged = true
			break
		}
	}

	if !titleChanged && !bodyChanged && !infoChanged {
		return false, false, "", nil // nothing to do
	}

	// Update informational fields
	for _, key := range informationalFieldOrder {
		if val, ok := newFM.Fields[key]; ok {
			fm.Fields[key] = val
		}
	}

	// Update editable fields if they changed remotely
	if titleChanged {
		fm.Title = newFM.Title
		fm.Fields["title"] = newFM.Title
	}
	if bodyChanged {
		localBody = ensureTrailingNewline(state.Description)
	}

	content := renderFrontmatter(fm, localBody)
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return false, false, "", err
	}

	// Handle story moved between epics
	if state.EntityType == "story" && state.RawFields != nil {
		var rawEpic struct {
			EpicID *int `json:"epic_id"`
		}
		json.Unmarshal(state.RawFields, &rawEpic)
		if rawEpic.EpicID != nil {
			correctEpicDir := findEpicDirByID(repoRoot, *rawEpic.EpicID)
			currentEpicDir := filepath.Dir(filePath)
			if correctEpicDir != "" && correctEpicDir != currentEpicDir {
				relOld, _ := filepath.Rel(repoRoot, filePath)
				movedPath := filepath.Join(correctEpicDir, filepath.Base(filePath))
				relNew, _ := filepath.Rel(repoRoot, movedPath)
				if _, mvErr := git(repoRoot, "mv", relOld, relNew); mvErr != nil {
					fmt.Fprintf(os.Stderr, "warning: moving story to epic dir: %v\n", mvErr)
				} else {
					filePath = movedPath
					infoOnly = false // epic move is a structural change
				}
			}
		}
	}

	// Handle title-driven rename (B16: check git mv errors)
	currentName := filepath.Base(filePath)
	expectedName := slugBasedName(state.EntityType, newFM.Title, currentName)
	if state.EntityType == "epic" {
		currentName = filepath.Base(filepath.Dir(filePath))
		expectedName = epicDirname(newFM.Title)
	}
	if expectedName != "" && currentName != expectedName {
		if state.EntityType == "epic" {
			oldDirBase := filepath.Base(filepath.Dir(filePath))
			if _, mvErr := git(repoRoot, "mv", oldDirBase, expectedName); mvErr != nil {
				fmt.Fprintf(os.Stderr, "warning: renaming epic dir %s → %s: %v\n", oldDirBase, expectedName, mvErr)
			} else {
				newFilePath = filepath.Join(repoRoot, expectedName, "_epic.md")
			}
		} else {
			relOld, _ := filepath.Rel(repoRoot, filePath)
			renamedPath := filepath.Join(filepath.Dir(filePath), expectedName)
			relNew, _ := filepath.Rel(repoRoot, renamedPath)
			if _, mvErr := git(repoRoot, "mv", relOld, relNew); mvErr != nil {
				fmt.Fprintf(os.Stderr, "warning: renaming %s → %s: %v\n", relOld, relNew, mvErr)
			} else {
				newFilePath = renamedPath
			}
		}
	}

	return infoOnly, true, newFilePath, nil
}

// findOrphanedFiles returns paths of .md files on disk that have an informational_shortcut_id
// not present in the remoteIDs set. These are files for entities that were removed remotely.
func findOrphanedFiles(repoRoot string, remoteIDs map[string]bool) []string {
	var orphans []string

	// Check _objective.md
	objPath := filepath.Join(repoRoot, "_objective.md")
	if id := extractEntityID(objPath); id > 0 {
		if !remoteIDs[fmt.Sprintf("objective-%d", id)] {
			orphans = append(orphans, objPath)
		}
	}

	entries, _ := os.ReadDir(repoRoot)
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		epicDir := filepath.Join(repoRoot, entry.Name())

		// Check _epic.md
		epicPath := filepath.Join(epicDir, "_epic.md")
		epicID := extractEntityID(epicPath)
		if epicID > 0 && !remoteIDs[fmt.Sprintf("epic-%d", epicID)] {
			// B5: Epic removed — collect the entire directory's .md files
			files, _ := os.ReadDir(epicDir)
			for _, f := range files {
				if strings.HasSuffix(f.Name(), ".md") {
					orphans = append(orphans, filepath.Join(epicDir, f.Name()))
				}
			}
			continue
		}

		// Check story files
		files, _ := os.ReadDir(epicDir)
		for _, f := range files {
			if f.IsDir() || f.Name() == "_epic.md" || !strings.HasSuffix(f.Name(), ".md") {
				continue
			}
			storyPath := filepath.Join(epicDir, f.Name())
			storyID := extractEntityID(storyPath)
			if storyID > 0 && !remoteIDs[fmt.Sprintf("story-%d", storyID)] {
				orphans = append(orphans, storyPath)
			}
		}
	}
	return orphans
}

func extractEntityID(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	fm, _, err := parseFrontmatter(string(data))
	if err != nil {
		return 0
	}
	idVal, ok := fm.Fields["informational_shortcut_id"]
	if !ok {
		return 0
	}
	return toInt(idVal)
}

func slugBasedName(entityType, title, currentFilename string) string {
	switch entityType {
	case "story":
		prefix := extractPositionPrefix(currentFilename)
		if prefix != "" {
			return prefix + slugify(title) + ".md"
		}
		return storyFilenameUnordered(title)
	case "epic":
		return epicDirname(title)
	default:
		return ""
	}
}

func createLocalFile(repoRoot string, cfg *RepoConfig, state *RemoteEntityState) error {
	fm := buildFrontmatterFromState(state, cfg)
	content := renderFrontmatter(fm, ensureTrailingNewline(state.Description))

	switch state.EntityType {
	case "story":
		epicDir, err := findEpicDirForStory(repoRoot, state)
		if err != nil {
			return err
		}
		nextPos := countStoriesInDir(epicDir) + 1
		filename := storyFilename(state.Name, nextPos)
		return os.WriteFile(filepath.Join(epicDir, filename), []byte(content), 0644)
	case "epic":
		dirName := epicDirname(state.Name)
		dir := filepath.Join(repoRoot, dirName)
		os.MkdirAll(dir, 0755)
		return os.WriteFile(filepath.Join(dir, "_epic.md"), []byte(content), 0644)
	case "objective":
		return os.WriteFile(filepath.Join(repoRoot, "_objective.md"), []byte(content), 0644)
	}
	return nil
}

func removeLocalFile(repoRoot, entityType string, id int) error {
	filePath, err := findFileForEntity(repoRoot, entityType, id)
	if err != nil {
		return nil
	}

	// B5: For epics, remove the entire directory
	if entityType == "epic" {
		epicDir := filepath.Dir(filePath)
		relDir, _ := filepath.Rel(repoRoot, epicDir)
		_, err = git(repoRoot, "rm", "-rf", relDir)
		return err
	}

	relPath, _ := filepath.Rel(repoRoot, filePath)
	_, err = git(repoRoot, "rm", relPath)
	return err
}

func findFileForEntity(repoRoot, entityType string, id int) (string, error) {
	idStr := strconv.Itoa(id)

	switch entityType {
	case "objective":
		path := filepath.Join(repoRoot, "_objective.md")
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	case "epic":
		entries, _ := os.ReadDir(repoRoot)
		for _, entry := range entries {
			if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			epicPath := filepath.Join(repoRoot, entry.Name(), "_epic.md")
			if matchesID(epicPath, idStr) {
				return epicPath, nil
			}
		}
	case "story":
		entries, _ := os.ReadDir(repoRoot)
		for _, entry := range entries {
			if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			dir := filepath.Join(repoRoot, entry.Name())
			files, _ := os.ReadDir(dir)
			for _, f := range files {
				if f.Name() == "_epic.md" || !hasExt(f.Name(), ".md") {
					continue
				}
				path := filepath.Join(dir, f.Name())
				if matchesID(path, idStr) {
					return path, nil
				}
			}
		}
	}
	return "", fmt.Errorf("%s %d: file not found", entityType, id)
}

func matchesID(path, idStr string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	fm, _, err := parseFrontmatter(string(data))
	if err != nil {
		return false
	}
	fmID, ok := fm.Fields["informational_shortcut_id"]
	if !ok {
		return false
	}
	return fmt.Sprintf("%v", fmID) == idStr
}

func hasExt(name, ext string) bool {
	return filepath.Ext(name) == ext
}

func findEpicDirByID(repoRoot string, epicID int) string {
	idStr := strconv.Itoa(epicID)
	entries, _ := os.ReadDir(repoRoot)
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		epicPath := filepath.Join(repoRoot, entry.Name(), "_epic.md")
		if matchesID(epicPath, idStr) {
			return filepath.Join(repoRoot, entry.Name())
		}
	}
	return ""
}

func findEpicDirForStory(repoRoot string, storyState *RemoteEntityState) (string, error) {
	var raw struct {
		EpicID *int `json:"epic_id"`
	}
	if storyState.RawFields != nil {
		if err := jsonUnmarshal(storyState.RawFields, &raw); err != nil || raw.EpicID == nil {
			return "", fmt.Errorf("story %d has no epic_id", storyState.ID)
		}
	}

	entries, _ := os.ReadDir(repoRoot)
	epicIDStr := strconv.Itoa(*raw.EpicID)
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		epicPath := filepath.Join(repoRoot, entry.Name(), "_epic.md")
		if matchesID(epicPath, epicIDStr) {
			return filepath.Join(repoRoot, entry.Name()), nil
		}
	}
	return "", fmt.Errorf("epic directory for epic %d not found", *raw.EpicID)
}

func buildFrontmatterFromState(state *RemoteEntityState, cfg *RepoConfig) *Frontmatter {
	fields := map[string]any{
		"title":                         state.Name,
		"informational_shortcut_id":     state.ID,
		"informational_last_updated_at": state.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}

	var raw struct {
		OwnerIDs        []string `json:"owner_ids"`
		WorkflowStateID int      `json:"workflow_state_id"`
		State           string   `json:"state"`
		AppURL          string   `json:"app_url"`
		StoryType       string   `json:"story_type"`
		Archived        bool     `json:"archived"`
		PullRequests    []struct {
			URL string `json:"url"`
		} `json:"pull_requests"`
	}
	if state.RawFields != nil {
		jsonUnmarshal(state.RawFields, &raw)
	}

	fields["informational_shortcut_url"] = raw.AppURL
	fields["informational_owner"] = resolveOwnerNames(raw.OwnerIDs, cfg.Members)

	if raw.WorkflowStateID != 0 {
		fields["informational_status"] = resolveStateName(raw.WorkflowStateID, cfg.WorkflowStates)
	} else if raw.State != "" {
		fields["informational_status"] = raw.State
	}

	if raw.StoryType != "" {
		fields["informational_type"] = raw.StoryType
	}
	if raw.Archived {
		fields["informational_archived"] = true
	}

	var prURLs []any
	for _, pr := range raw.PullRequests {
		if pr.URL != "" {
			prURLs = append(prURLs, pr.URL)
		}
	}
	if len(prURLs) > 0 {
		fields["informational_github_pr_urls"] = prURLs
	}

	return &Frontmatter{Title: state.Name, Fields: fields}
}

func countStoriesInDir(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	count := 0
	for _, e := range entries {
		if !e.IsDir() && e.Name() != "_epic.md" && strings.HasSuffix(e.Name(), ".md") {
			count++
		}
	}
	return count
}

func jsonUnmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}
