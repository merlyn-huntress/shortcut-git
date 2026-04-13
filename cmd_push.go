package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func cmdPush(args []string) error {
	root, err := findRepoRoot(".")
	if err != nil {
		return err
	}

	fs := flag.NewFlagSet("push", flag.ContinueOnError)
	force := fs.Bool("force", false, "push even if remote has diverged")
	dryRun := fs.Bool("dry-run", false, "show what would be pushed without making changes")
	if err := fs.Parse(args); err != nil {
		return err
	}

	// Check for clean working tree
	status, err := git(root, "status", "--porcelain")
	if err != nil {
		return err
	}
	if strings.TrimSpace(status) != "" {
		return fmt.Errorf("working tree is dirty. Commit or discard changes before pushing.\n%s", status)
	}

	cfg, err := loadRepoConfig(root)
	if err != nil {
		return err
	}

	token, err := loadAPIToken()
	if err != nil {
		return err
	}
	client := NewShortcutClient(token)

	// Find all markdown files and check for changes
	var pushOps []pushOp
	if err := collectPushOps(root, cfg, &pushOps); err != nil {
		return err
	}

	// B25: Warn about locally deleted files that still exist on Shortcut
	warnDeletedFiles(root)

	if len(pushOps) == 0 {
		fmt.Println("Nothing to push.")
		return nil
	}

	// Dry run: show what would change
	if *dryRun {
		fmt.Println("Dry run — the following changes would be pushed:")
		fmt.Println()
		for _, op := range pushOps {
			printPushOp(op)
		}
		return nil
	}

	// Check for divergence on existing entities
	if !*force {
		for _, op := range pushOps {
			if op.isNew {
				continue
			}
			// Re-fetch remote state to check for divergence
			freshState, err := fetchFreshState(client, op.entityType, op.id)
			if err != nil {
				return fmt.Errorf("checking remote state for %s %d: %w", op.entityType, op.id, err)
			}
			oldState, _ := loadRemoteState(root, op.entityType, op.id)
			if oldState != nil {
				newHash := contentHash(map[string]string{"name": freshState.Name, "description": freshState.Description})
				if newHash != oldState.ContentHash {
					return fmt.Errorf("remote has diverged for %s %d (%s). Run `shortcut-git pull` first, or use --force",
						op.entityType, op.id, op.name)
				}
			}
		}
	}

	// Execute pushes — epics first (new epics must exist before their stories)
	// Sort: new epics first, then existing epics, then new stories, then existing stories
	sortPushOps(pushOps)

	updated := 0
	created := 0

	// B7: Process each entity and update remote state incrementally.
	// If a failure occurs, previously-pushed entities are already committed and saved.
	for _, op := range pushOps {
		if op.isNew {
			// Resolve epic ID for new stories whose parent epic was also just created
			if op.entityType == "story" && op.epicID == 0 && op.epicMDPath != "" {
				epicFMData, _ := os.ReadFile(op.epicMDPath)
				if epicFMData != nil {
					efm, _, _ := parseFrontmatter(string(epicFMData))
					if efm != nil {
						if eid, ok := efm.Fields["informational_shortcut_id"]; ok {
							op.epicID = toInt(eid)
						}
					}
				}
				if op.epicID == 0 {
					return fmt.Errorf("cannot create story %q: parent epic has no shortcut ID", op.title)
				}
			}
			if err := pushCreate(root, cfg, client, &op); err != nil {
				// Commit any IDs assigned so far before returning error
				if created > 0 {
					git(root, "add", ".")
					git(root, "commit", "-m", "shortcut-git: assign IDs from push (partial)")
				}
				return fmt.Errorf("creating %s: %w", op.entityType, err)
			}
			created++
			// Commit the ID assignment immediately and update remote state
			git(root, "add", ".")
			git(root, "commit", "-m", fmt.Sprintf("shortcut-git: assign ID for %s %q", op.entityType, op.title))
		} else {
			if err := pushUpdate(root, client, op); err != nil {
				return fmt.Errorf("updating %s %d: %w", op.entityType, op.id, err)
			}
			updated++
		}

		// Update remote state for this entity immediately
		if op.id != 0 {
			freshState, err := fetchFreshState(client, op.entityType, op.id)
			if err == nil {
				saveRemoteState(root, freshState)
			}
		}
	}

	fmt.Printf("Pushed: %d updated, %d created\n", updated, created)
	return nil
}

type pushOp struct {
	entityType string // "objective", "epic", "story"
	id         int    // 0 if new
	name       string
	isNew      bool
	filePath   string
	title      string
	body       string
	oldName    string // for diff display
	oldBody    string
	epicID     int    // parent epic ID for new stories
	epicMDPath string // path to parent _epic.md (for resolving ID after epic creation)
}

func collectPushOps(repoRoot string, cfg *RepoConfig, ops *[]pushOp) error {
	// Check objective.md
	objPath := filepath.Join(repoRoot, "_objective.md")
	if op, changed, err := checkFile(repoRoot, objPath, "objective"); err == nil && changed {
		*ops = append(*ops, op)
	}

	// Check all epic directories
	entries, _ := os.ReadDir(repoRoot)
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name()[0] == '.' {
			continue
		}
		epicDir := filepath.Join(repoRoot, entry.Name())

		// Check epic.md
		epicPath := filepath.Join(epicDir, "_epic.md")
		if op, changed, err := checkFile(repoRoot, epicPath, "epic"); err == nil && changed {
			*ops = append(*ops, op)
		}

		// Resolve this directory's epic ID
		dirEpicID := 0
		epicFMData, _ := os.ReadFile(epicPath)
		if epicFMData != nil {
			efm, _, _ := parseFrontmatter(string(epicFMData))
			if efm != nil {
				if eid, ok := efm.Fields["informational_shortcut_id"]; ok {
					dirEpicID = toInt(eid)
				}
			}
		}

		// Check story files
		files, _ := os.ReadDir(epicDir)
		for _, f := range files {
			if f.Name() == "_epic.md" || !strings.HasSuffix(f.Name(), ".md") || f.IsDir() {
				continue
			}
			storyPath := filepath.Join(epicDir, f.Name())
			op, changed, err := checkFile(repoRoot, storyPath, "story")
			if err != nil {
				continue
			}

			if op.isNew {
				op.epicMDPath = epicPath
				op.epicID = dirEpicID
				*ops = append(*ops, op)
				continue
			}

			// B26: Check if story was moved to a different epic
			if !changed && op.id != 0 && dirEpicID != 0 {
				oldState, _ := loadRemoteState(repoRoot, "story", op.id)
				if oldState != nil && oldState.RawFields != nil {
					var raw struct {
						EpicID *int `json:"epic_id"`
					}
					json.Unmarshal(oldState.RawFields, &raw)
					if raw.EpicID != nil && *raw.EpicID != dirEpicID {
						op.epicID = dirEpicID
						changed = true
					}
				}
			}

			if changed {
				*ops = append(*ops, op)
			}
		}
	}
	return nil
}

func checkFile(repoRoot, filePath, entityType string) (pushOp, bool, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return pushOp{}, false, err
	}

	fm, body, err := parseFrontmatter(string(data))
	if err != nil {
		return pushOp{}, false, err
	}

	op := pushOp{
		entityType: entityType,
		filePath:   filePath,
		title:      fm.Title,
		body:       strings.TrimSpace(body),
	}

	idVal, hasID := fm.Fields["informational_shortcut_id"]
	if !hasID || toInt(idVal) == 0 {
		// New entity
		op.isNew = true
		return op, true, nil
	}

	op.id = toInt(idVal)

	// Compare against stored remote state
	oldState, err := loadRemoteState(repoRoot, entityType, op.id)
	if err != nil {
		// No remote state — treat as changed
		return op, true, nil
	}

	op.oldName = oldState.Name
	op.oldBody = strings.TrimSpace(oldState.Description)

	// Check if title or description changed
	if op.title != oldState.Name || op.body != op.oldBody {
		return op, true, nil
	}

	return op, false, nil
}

func toInt(v any) int {
	switch val := v.(type) {
	case int:
		return val
	case float64:
		return int(val)
	case int64:
		return int(val)
	case string:
		n, _ := strconv.Atoi(val)
		return n
	}
	return 0
}

func printPushOp(op pushOp) {
	if op.isNew {
		fmt.Printf("[NEW] %s: %q\n", op.entityType, op.title)
		return
	}
	if op.title != op.oldName {
		fmt.Printf("%s %d: title: %q → %q\n", op.entityType, op.id, op.oldName, op.title)
	}
	if op.body != op.oldBody {
		oldLines := len(strings.Split(op.oldBody, "\n"))
		newLines := len(strings.Split(op.body, "\n"))
		fmt.Printf("%s %d: description changed (%d lines → %d lines)\n", op.entityType, op.id, oldLines, newLines)
	}
	if op.entityType == "story" && op.epicID != 0 {
		fmt.Printf("%s %d: moved to epic %d\n", op.entityType, op.id, op.epicID)
	}
}

func sortPushOps(ops []pushOp) {
	// Simple stable sort: new epics first, existing epics, new stories, existing stories, objectives
	priority := func(op pushOp) int {
		switch {
		case op.entityType == "epic" && op.isNew:
			return 0
		case op.entityType == "epic":
			return 1
		case op.entityType == "story" && op.isNew:
			return 2
		case op.entityType == "story":
			return 3
		default:
			return 4
		}
	}
	// Bubble sort is fine for small lists
	for i := range ops {
		for j := i + 1; j < len(ops); j++ {
			if priority(ops[j]) < priority(ops[i]) {
				ops[i], ops[j] = ops[j], ops[i]
			}
		}
	}
}

func pushUpdate(repoRoot string, client *ShortcutClient, op pushOp) error {
	fields := map[string]any{}
	if op.title != op.oldName {
		fields["name"] = op.title
	}
	if op.body != op.oldBody {
		fields["description"] = op.body
	}
	// B26: Include epic_id change if story was moved between epics
	if op.entityType == "story" && op.epicID != 0 {
		fields["epic_id"] = op.epicID
	}
	if len(fields) == 0 {
		return nil
	}

	fmt.Fprintf(os.Stderr, "Updating %s %d (%s)...\n", op.entityType, op.id, op.title)
	switch op.entityType {
	case "story":
		_, err := client.UpdateStory(op.id, fields)
		return err
	case "epic":
		_, err := client.UpdateEpic(op.id, fields)
		return err
	case "objective":
		_, err := client.UpdateObjective(op.id, fields)
		return err
	}
	return nil
}

func pushCreate(repoRoot string, cfg *RepoConfig, client *ShortcutClient, op *pushOp) error {
	fmt.Fprintf(os.Stderr, "Creating %s: %q...\n", op.entityType, op.title)

	switch op.entityType {
	case "epic":
		fields := map[string]any{
			"name":          op.title,
			"description":   op.body,
			"objective_ids": []int{cfg.ObjectiveID},
		}
		epic, err := client.CreateEpic(fields)
		if err != nil {
			return err
		}
		op.id = epic.ID
		return writeBackID(op.filePath, epic.ID, epic.AppURL)

	case "story":
		if op.epicID == 0 {
			return fmt.Errorf("story has no parent epic ID")
		}
		// Find the default workflow state (first "unstarted" state from any workflow)
		defaultStateID := findDefaultWorkflowState(cfg)
		if defaultStateID == 0 {
			return fmt.Errorf("cannot determine default workflow state for new story")
		}
		fields := map[string]any{
			"name":              op.title,
			"description":       op.body,
			"epic_id":           op.epicID,
			"workflow_state_id": defaultStateID,
		}
		if cfg.TeamID != "" {
			fields["group_id"] = cfg.TeamID
		}
		story, err := client.CreateStory(fields)
		if err != nil {
			return err
		}
		op.id = story.ID
		return writeBackID(op.filePath, story.ID, story.AppURL)

	default:
		return fmt.Errorf("cannot create %s entities", op.entityType)
	}
}

func writeBackID(filePath string, id int, appURL string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	fm, body, err := parseFrontmatter(string(data))
	if err != nil {
		return err
	}
	fm.Fields["informational_shortcut_id"] = id
	fm.Fields["informational_shortcut_url"] = appURL
	content := renderFrontmatter(fm, body)
	return os.WriteFile(filePath, []byte(content), 0644)
}

func fetchFreshState(client *ShortcutClient, entityType string, id int) (*RemoteEntityState, error) {
	switch entityType {
	case "story":
		s, err := client.GetStory(id)
		if err != nil {
			return nil, err
		}
		return stateFromStory(s), nil
	case "epic":
		e, err := client.GetEpic(id)
		if err != nil {
			return nil, err
		}
		return stateFromEpic(e), nil
	case "objective":
		o, err := client.GetObjective(id)
		if err != nil {
			return nil, err
		}
		return stateFromObjective(o), nil
	}
	return nil, fmt.Errorf("unknown entity type: %s", entityType)
}

// warnDeletedFiles checks for remote state entries whose local files are missing.
func warnDeletedFiles(repoRoot string) {
	states, err := loadAllRemoteStates(repoRoot)
	if err != nil {
		return
	}
	for _, state := range states {
		_, err := findFileForEntity(repoRoot, state.EntityType, state.ID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: %s %d (%s) was deleted locally but still exists on Shortcut. It will reappear on next pull.\n",
				state.EntityType, state.ID, state.Name)
		}
	}
}

