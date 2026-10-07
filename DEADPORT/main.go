package main

import (
	"context"
	"errors"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"charm.land/bubbletea/v2"
	"charm.land/ssh"
	"charm.land/wish/v2"
	wtea "charm.land/wish/v2/bubbletea"
	"charm.land/wish/v2/logging"
	gossh "golang.org/x/crypto/ssh"
	
	"github.com/deadport-game/deadport/internal/app"
	"github.com/deadport-game/deadport/internal/database"
	"github.com/deadport-game/deadport/internal/game"
	"github.com/deadport-game/deadport/internal/terminal"
)

const (
	host = "0.0.0.0"
	port = 2222
)

func main() {
	// Initialize Database
	dbPath := os.Getenv("DEADPORT_DB")
	if dbPath == "" {
		dbPath = "deadport.db"
	}
	db, err := database.Open(dbPath)
	if err != nil {
		log.Fatalf("Could not open database: %v", err)
	}
	defer db.Close()

	// Load Game Catalog
	cat, err := game.LoadCatalog("challenges")
	if err != nil {
		log.Fatalf("Could not load challenges: %v", err)
	}
	log.Printf("Loaded %d missions.", len(cat.Ordered))

	// Create Game Engine
	engine := game.NewEngine(db, cat)

	// Setup Wish Server
	s, err := wish.NewServer(
		wish.WithAddress(net.JoinHostPort(host, "2222")),
		wish.WithHostKeyPath(".ssh/term_info_ed25519"),
		wish.WithPublicKeyAuth(func(ctx ssh.Context, key ssh.PublicKey) bool {
			// We can allow any public key, and we use it as their identity if present.
			return true
		}),
		wish.WithPasswordAuth(func(ctx ssh.Context, password string) bool {
			// Allow any password for players without SSH keys
			return true
		}),
		wish.WithKeyboardInteractiveAuth(func(ctx ssh.Context, challenger gossh.KeyboardInteractiveChallenge) bool {
			return true
		}),
		wish.WithMiddleware(
			wtea.Middleware(func(s ssh.Session) (tea.Model, []tea.ProgramOption) {
				pty, _, active := s.Pty()
				if !active {
					log.Println("no active terminal, exiting")
					return nil, nil
				}
				
				// Identify player
				id := s.User()
				if key := s.PublicKey(); key != nil {
					id = strings.TrimSpace(string(gossh.MarshalAuthorizedKey(key)))
				}
				
				// Ensure Player Exists
				defaultName := s.User()
				if defaultName == "" || len(defaultName) > 16 {
					defaultName = "Employee"
				}
				p, err := engine.EnsurePlayer(id, defaultName)
				if err != nil {
					log.Println("Database error creating player:", err)
					return nil, nil
				}
				
				// Build initial shell
				shell := terminal.NewShell(terminal.NewFS(), terminal.Options{User: p.DisplayName})
				
				m := &app.UIModel{
					Engine: engine,
					Player: p,
					Shell:  shell,
					Width:  pty.Window.Width,
					Height: pty.Window.Height,
					Output: []string{},
				}
				
				return m, nil
			}),
			logging.Middleware(),
		),
	)
	if err != nil {
		log.Fatal("Could not start server:", err)
	}

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	log.Printf("Starting DEADPORT on %s:%d", host, port)

	go func() {
		if err := s.ListenAndServe(); err != nil && !errors.Is(err, ssh.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	<-done
	log.Println("Stopping DEADPORT server...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil && !errors.Is(err, ssh.ErrServerClosed) {
		log.Fatal(err)
	}
}
