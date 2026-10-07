package game

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/deadport-game/deadport/internal/terminal"
)

// Mission represents a playable challenge.
type Mission struct {
	ID            string            `json:"id"`
	Title         string            `json:"title"`
	Difficulty    int               `json:"difficulty"` // 1 (Beginner) to 5 (Boss)
	Category      string            `json:"category"`
	XP            int               `json:"xp"`
	TargetSeconds int               `json:"target_seconds"` // For speed demon achievement
	Tags          []string          `json:"tags"`
	Story         []string          `json:"story"`
	Objective     string            `json:"objective"`
	AnswerFormat  string            `json:"answer_format"`
	Commands      []string          `json:"commands"` // Optional tools to enable
	Start         string            `json:"start"`    // Initial CWD
	Files         map[string]any    `json:"files"`    // Path -> content (string or []string)
	Triggers      map[string]any    `json:"triggers"` // Path -> map with "item", "achievement"
	AnswerHashes  []string          `json:"answer_hashes"` // Unused for now, server validates via engine
	Hints         []string          `json:"hints"`
	Completion    []string          `json:"completion"`
}

// AnswerValidator is an interface for checking answers, so we can do dynamic checks.
type AnswerValidator func(answer string) bool

// Catalog holds all loaded missions.
type Catalog struct {
	Missions map[string]*Mission
	Ordered  []*Mission
}

// LoadCatalog reads all JSON files in the given directory tree.
func LoadCatalog(root string) (*Catalog, error) {
	cat := &Catalog{
		Missions: make(map[string]*Mission),
	}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(info.Name(), ".json") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var m Mission
		if err := json.Unmarshal(b, &m); err != nil {
			return err // Log and continue?
		}
		cat.Missions[m.ID] = &m
		cat.Ordered = append(cat.Ordered, &m)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(cat.Ordered, func(i, j int) bool {
		return cat.Ordered[i].ID < cat.Ordered[j].ID
	})
	return cat, nil
}

// SetupFS populates a fake filesystem with the mission's files.
func (m *Mission) SetupFS(fs *terminal.FS) error {
	for path, val := range m.Files {
		var content string
		switch v := val.(type) {
		case string:
			content = v
		case []interface{}:
			var lines []string
			for _, line := range v {
				if s, ok := line.(string); ok {
					lines = append(lines, s)
				}
			}
			content = strings.Join(lines, "\n")
		}
		if err := fs.WriteFile(path, content); err != nil {
			return err
		}
	}
	return nil
}
