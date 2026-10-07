package game

import (
	"strings"
	"time"
)

// Rank represents a player's progression title.
type Rank struct {
	Name string
	XP   int
}

var Ranks = []Rank{
	{"NOVICE", 0},
	{"RECRUIT", 100},
	{"ANALYST", 300},
	{"OPERATOR", 750},
	{"SPECIALIST", 1500},
	{"HUNTER", 3000},
	{"BREACHER", 5000},
	{"ROOT", 8000},
	{"LEGEND", 15000},
}

// GetRank returns the rank name for a given amount of XP.
func GetRank(xp int) string {
	for i := len(Ranks) - 1; i >= 0; i-- {
		if xp >= Ranks[i].XP {
			return Ranks[i].Name
		}
	}
	return Ranks[0].Name
}

// Player represents a user's persistent state.
type Player struct {
	ID                string
	DisplayName       string
	CreatedAt         time.Time
	LastSeen          time.Time
	XP                int
	Rank              string // Derived from XP, but we can store it
	CompletedMissions map[string]time.Time
	DailyCompletions  int
	AchievementCount  int
	Items             map[string]bool
	SSHPubKeys        []string // Associated keys
}

// NewPlayer creates a new player object.
func NewPlayer(id, name string) *Player {
	now := time.Now()
	return &Player{
		ID:                id,
		DisplayName:       name,
		CreatedAt:         now,
		LastSeen:          now,
		CompletedMissions: make(map[string]time.Time),
		Items:             make(map[string]bool),
		Rank:              Ranks[0].Name,
	}
}

// AddXP adds experience points and updates the rank.
func (p *Player) AddXP(amount int) (leveledUp bool) {
	oldRank := GetRank(p.XP)
	p.XP += amount
	newRank := GetRank(p.XP)
	p.Rank = newRank
	return oldRank != newRank
}

// Answers maps mission IDs to answers for local offline validation.
// In a true online CTF this might be server-side only, but the prompt mentions:
// "DO NOT pretend local challenge answers are cryptographically secret...
// This is acceptable for a local/open-source game."
var Answers = map[string]string{
	"001": "potato-admin-1999",
	"002": "Q3-PANIC-77",
	"003": "SPAGHETTI-MONSTER",
	"004": "43",
	"005": "QUACK-7731",
	"006": "ceo",
	"007": "CAFE-BABE-99",
	"008": "BACKWARDS-IS-FORWARDS",
	"009": "1337",
	"010": "LONE-WOLF",
	"011": "TOP-DOG",
	"012": "rm -rf /production",
	"013": "ONION-ROUTER-99",
	"014": "SQUIRREL-TREE",
	"015": "DEEP-DARK-SECRET",
	"016": "HIDE-AND-SEEK",
	"017": "NEEDLE-HAYSTACK",
	"018": "BASH-MASTER",
	"019": "JULIUS-WAS-HERE",
	"020": "SYSTEM-COMPROMISED-ABORT",
}

// ValidateAnswer checks if the answer is correct for the mission.
func ValidateAnswer(missionID, answer string) bool {
	ans := strings.TrimSpace(answer)
	expected, ok := Answers[missionID]
	if !ok {
		return false // No answer defined means it's a dynamic puzzle or bug
	}
	return strings.EqualFold(ans, expected)
}
