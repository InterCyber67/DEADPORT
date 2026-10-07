package game

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// Store represents the persistence layer.
type Store interface {
	LoadPlayer(id string) (*Player, error)
	SavePlayer(p *Player) error
	MarkMissionComplete(playerID, missionID string) error
	AddItem(playerID, itemID string) error
}

// Engine holds the global state for the game.
type Engine struct {
	DB      Store
	Catalog *Catalog
	
	mu          sync.Mutex
	subscribers map[string]chan string
}

// NewEngine creates a new game engine instance.
func NewEngine(db Store, catalog *Catalog) *Engine {
	return &Engine{
		DB:          db,
		Catalog:     catalog,
		subscribers: make(map[string]chan string),
	}
}

// Subscribe returns a channel that receives broadcast messages.
func (e *Engine) Subscribe(id string) <-chan string {
	e.mu.Lock()
	defer e.mu.Unlock()
	ch := make(chan string, 100)
	e.subscribers[id] = ch
	return ch
}

// Unsubscribe removes a subscriber.
func (e *Engine) Unsubscribe(id string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if ch, ok := e.subscribers[id]; ok {
		close(ch)
		delete(e.subscribers, id)
	}
}

// Broadcast sends a message to all subscribers.
func (e *Engine) Broadcast(sender, message string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	
	msg := fmt.Sprintf("[%s] %s", sender, message)
	for _, ch := range e.subscribers {
		select {
		case ch <- msg:
		default: // drop if buffer is full
		}
	}
}

// EnsurePlayer retrieves a player from DB or creates a new one.
func (e *Engine) EnsurePlayer(id, defaultName string) (*Player, error) {
	p, err := e.DB.LoadPlayer(id)
	if err != nil {
		return nil, err
	}
	if p == nil {
		p = NewPlayer(id, defaultName)
		if err := e.DB.SavePlayer(p); err != nil {
			return nil, err
		}
	}
	return p, nil
}

// SubmitAnswer attempts to solve a mission for a player.
func (e *Engine) SubmitAnswer(p *Player, missionID, answer string) (bool, string, error) {
	mission, ok := e.Catalog.Missions[missionID]
	if !ok {
		return false, "Mission not found.", nil
	}

	if ValidateAnswer(missionID, answer) {
		// Already completed?
		if _, completed := p.CompletedMissions[missionID]; completed {
			return true, fmt.Sprintf("Correct, but you already completed [%s].", mission.Title), nil
		}

		// First time completion
		leveledUp := p.AddXP(mission.XP)
		p.CompletedMissions[missionID] = time.Now()
		if err := e.DB.MarkMissionComplete(p.ID, missionID); err != nil {
			return false, "", err
		}
		if err := e.DB.SavePlayer(p); err != nil { // Save new XP/Rank
			return false, "", err
		}
		
		msg := fmt.Sprintf("ACCEPTED. You earned %d XP.", mission.XP)
		if leveledUp {
			msg += fmt.Sprintf("\nPROMOTION: You are now rank %s.", p.Rank)
		}
		
		// Run completion triggers if any
		if len(mission.Completion) > 0 {
			msg += "\n\n" + strings.Join(mission.Completion, "\n")
		}
		return true, msg, nil
	}
	
	return false, "INCORRECT OR INVALID SUBMISSION.", nil
}
