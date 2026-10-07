package database

import (
	"database/sql"
	"time"

	"github.com/deadport-game/deadport/internal/game"
	_ "modernc.org/sqlite" // Pure Go SQLite driver
)

// DB wraps the SQLite connection.
type DB struct {
	*sql.DB
}

// Open initializes the database connection and creates tables if missing.
func Open(path string) (*DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	d := &DB{db}
	if err := d.migrate(); err != nil {
		return nil, err
	}
	return d, nil
}

func (db *DB) migrate() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS players (
			id TEXT PRIMARY KEY,
			display_name TEXT UNIQUE NOT NULL,
			created_at DATETIME NOT NULL,
			last_seen DATETIME NOT NULL,
			xp INTEGER NOT NULL DEFAULT 0,
			rank TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS mission_completions (
			player_id TEXT NOT NULL,
			mission_id TEXT NOT NULL,
			completed_at DATETIME NOT NULL,
			PRIMARY KEY (player_id, mission_id),
			FOREIGN KEY (player_id) REFERENCES players(id)
		);`,
		`CREATE TABLE IF NOT EXISTS player_items (
			player_id TEXT NOT NULL,
			item_id TEXT NOT NULL,
			PRIMARY KEY (player_id, item_id),
			FOREIGN KEY (player_id) REFERENCES players(id)
		);`,
	}
	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

// SavePlayer inserts or updates a player record.
func (db *DB) SavePlayer(p *game.Player) error {
	_, err := db.Exec(`
		INSERT INTO players (id, display_name, created_at, last_seen, xp, rank)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			display_name = excluded.display_name,
			last_seen = excluded.last_seen,
			xp = excluded.xp,
			rank = excluded.rank
	`, p.ID, p.DisplayName, p.CreatedAt.Format(time.RFC3339), p.LastSeen.Format(time.RFC3339), p.XP, p.Rank)
	return err
}

// LoadPlayer loads a player by ID. Returns nil, nil if not found.
func (db *DB) LoadPlayer(id string) (*game.Player, error) {
	p := &game.Player{
		ID:                id,
		CompletedMissions: make(map[string]time.Time),
		Items:             make(map[string]bool),
	}
	var created, seen string
	err := db.QueryRow(`SELECT display_name, created_at, last_seen, xp, rank FROM players WHERE id = ?`, id).
		Scan(&p.DisplayName, &created, &seen, &p.XP, &p.Rank)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p.CreatedAt, _ = time.Parse(time.RFC3339, created)
	p.LastSeen, _ = time.Parse(time.RFC3339, seen)

	// Load completions
	rows, err := db.Query(`SELECT mission_id, completed_at FROM mission_completions WHERE player_id = ?`, id)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var mid, cat string
			if err := rows.Scan(&mid, &cat); err == nil {
				t, _ := time.Parse(time.RFC3339, cat)
				p.CompletedMissions[mid] = t
			}
		}
	}

	// Load items
	iRows, err := db.Query(`SELECT item_id FROM player_items WHERE player_id = ?`, id)
	if err == nil {
		defer iRows.Close()
		for iRows.Next() {
			var item string
			if err := iRows.Scan(&item); err == nil {
				p.Items[item] = true
			}
		}
	}

	return p, nil
}

// MarkMissionComplete records a completed mission.
func (db *DB) MarkMissionComplete(playerID, missionID string) error {
	_, err := db.Exec(`
		INSERT INTO mission_completions (player_id, mission_id, completed_at)
		VALUES (?, ?, ?)
		ON CONFLICT(player_id, mission_id) DO NOTHING
	`, playerID, missionID, time.Now().UTC().Format(time.RFC3339))
	return err
}

// AddItem gives an item/collectible to a player.
func (db *DB) AddItem(playerID, itemID string) error {
	_, err := db.Exec(`
		INSERT INTO player_items (player_id, item_id)
		VALUES (?, ?)
		ON CONFLICT(player_id, item_id) DO NOTHING
	`, playerID, itemID)
	return err
}
