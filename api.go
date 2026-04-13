package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

type ShortcutClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

func NewShortcutClient(token string) *ShortcutClient {
	return &ShortcutClient{
		baseURL:    "https://api.app.shortcut.com/api/v3",
		token:      token,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *ShortcutClient) doRequest(method, path string, body any, result any) error {
	var bodyData []byte
	if body != nil {
		var err error
		bodyData, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}

	maxRetries := 3
	for attempt := 0; attempt <= maxRetries; attempt++ {
		var bodyReader io.Reader
		if bodyData != nil {
			bodyReader = bytes.NewReader(bodyData)
		}

		req, err := http.NewRequest(method, c.baseURL+path, bodyReader)
		if err != nil {
			return err
		}
		req.Header.Set("Shortcut-Token", c.token)
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return err
		}

		if resp.StatusCode == 429 {
			resp.Body.Close()
			if attempt < maxRetries {
				wait := time.Duration(1<<uint(attempt)) * time.Second // 1s, 2s, 4s
				fmt.Fprintf(os.Stderr, "Rate limited, retrying in %v...\n", wait)
				time.Sleep(wait)
				continue
			}
			return fmt.Errorf("rate limited by Shortcut API (429) after %d retries", maxRetries)
		}

		defer resp.Body.Close()

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			respBody, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("shortcut API %s %s: %d %s", method, path, resp.StatusCode, string(respBody))
		}

		if result != nil {
			return json.NewDecoder(resp.Body).Decode(result)
		}
		return nil
	}
	return fmt.Errorf("exhausted retries for %s %s", method, path)
}

func (c *ShortcutClient) get(path string, result any) error {
	return c.doRequest("GET", path, nil, result)
}

func (c *ShortcutClient) put(path string, body any, result any) error {
	return c.doRequest("PUT", path, body, result)
}

func (c *ShortcutClient) post(path string, body any, result any) error {
	return c.doRequest("POST", path, body, result)
}

// --- API response types ---

type Objective struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	State       string    `json:"state"`
	OwnerIDs    []string  `json:"owner_ids"`
	AppURL      string    `json:"app_url"`
	UpdatedAt   time.Time `json:"updated_at"`
	Archived    bool      `json:"archived"`
}

type Epic struct {
	ID           int       `json:"id"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	State        string    `json:"state"`
	OwnerIDs     []string  `json:"owner_ids"`
	ObjectiveIDs []int     `json:"objective_ids"`
	TeamID       string    `json:"team_id"`
	AppURL       string    `json:"app_url"`
	UpdatedAt    time.Time `json:"updated_at"`
	Archived     bool      `json:"archived"`
}

type Story struct {
	ID              int       `json:"id"`
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	StoryType       string    `json:"story_type"`
	WorkflowStateID int       `json:"workflow_state_id"`
	OwnerIDs        []string  `json:"owner_ids"`
	EpicID          *int      `json:"epic_id"`
	TeamID          string    `json:"team_id"`
	GroupID         string    `json:"group_id"`
	Position        int64     `json:"position"`
	AppURL          string    `json:"app_url"`
	UpdatedAt       time.Time `json:"updated_at"`
	Archived        bool      `json:"archived"`
	PullRequests    []StoryPR `json:"pull_requests"`
}

type StoryPR struct {
	URL string `json:"url"`
}

type WorkflowState struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type Workflow struct {
	ID     int             `json:"id"`
	Name   string          `json:"name"`
	States []WorkflowState `json:"states"`
}

type Member struct {
	ID      string         `json:"id"`
	Profile MemberProfile  `json:"profile"`
}

type MemberProfile struct {
	Name string `json:"name"`
}

type EpicSearchResult struct {
	Data []Epic `json:"data"`
	Next *int   `json:"next"`
}

// --- API methods ---

func (c *ShortcutClient) GetObjective(id int) (*Objective, error) {
	var obj Objective
	err := c.get(fmt.Sprintf("/objectives/%d", id), &obj)
	return &obj, err
}

func (c *ShortcutClient) SearchEpicsByObjective(objectiveID int) ([]Epic, error) {
	// List all epics and filter by objective_ids containing our objective.
	// The Shortcut API doesn't have a direct "epics by objective" endpoint,
	// so we list all and filter client-side.
	var allEpics []Epic
	err := c.get("/epics", &allEpics)
	if err != nil {
		return nil, fmt.Errorf("listing epics: %w", err)
	}
	var matched []Epic
	for _, epic := range allEpics {
		for _, oid := range epic.ObjectiveIDs {
			if oid == objectiveID {
				matched = append(matched, epic)
				break
			}
		}
	}
	return matched, nil
}

func (c *ShortcutClient) GetEpicStories(epicID int) ([]Story, error) {
	var stories []Story
	err := c.get(fmt.Sprintf("/epics/%d/stories", epicID), &stories)
	return stories, err
}

func (c *ShortcutClient) GetWorkflows() ([]Workflow, error) {
	var workflows []Workflow
	err := c.get("/workflows", &workflows)
	return workflows, err
}

func (c *ShortcutClient) GetMembers() ([]Member, error) {
	var members []Member
	err := c.get("/members", &members)
	return members, err
}

func (c *ShortcutClient) GetStory(id int) (*Story, error) {
	var s Story
	err := c.get(fmt.Sprintf("/stories/%d", id), &s)
	return &s, err
}

func (c *ShortcutClient) GetEpic(id int) (*Epic, error) {
	var e Epic
	err := c.get(fmt.Sprintf("/epics/%d", id), &e)
	return &e, err
}

func (c *ShortcutClient) UpdateStory(id int, fields map[string]any) (*Story, error) {
	var s Story
	err := c.put(fmt.Sprintf("/stories/%d", id), fields, &s)
	return &s, err
}

func (c *ShortcutClient) UpdateEpic(id int, fields map[string]any) (*Epic, error) {
	var e Epic
	err := c.put(fmt.Sprintf("/epics/%d", id), fields, &e)
	return &e, err
}

func (c *ShortcutClient) UpdateObjective(id int, fields map[string]any) (*Objective, error) {
	var obj Objective
	err := c.put(fmt.Sprintf("/objectives/%d", id), fields, &obj)
	return &obj, err
}

func (c *ShortcutClient) CreateStory(fields map[string]any) (*Story, error) {
	var s Story
	err := c.post("/stories", fields, &s)
	return &s, err
}

func (c *ShortcutClient) CreateEpic(fields map[string]any) (*Epic, error) {
	var e Epic
	err := c.post("/epics", fields, &e)
	return &e, err
}
