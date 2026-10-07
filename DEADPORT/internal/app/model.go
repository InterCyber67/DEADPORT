package app

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/deadport-game/deadport/internal/game"
	"github.com/deadport-game/deadport/internal/terminal"
)

type UIModel struct {
	Engine   *game.Engine
	Player   *game.Player
	Shell    *terminal.Shell
	Width      int
	Height     int
	Input      string
	Output     []string
	ChatOutput []string
	ScrollOffset int
	
	chatSub <-chan string
	blink   bool
}

type chatMsg string
type blinkMsg time.Time

func tickBlink() tea.Cmd {
	return tea.Tick(time.Millisecond*500, func(t time.Time) tea.Msg {
		return blinkMsg(t)
	})
}

func listenForChat(sub <-chan string) tea.Cmd {
	return func() tea.Msg {
		if msg, ok := <-sub; ok {
			return chatMsg(msg)
		}
		return nil
	}
}

var (
	StylePrompt = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Bold(true)
	StyleDir    = lipgloss.NewStyle().Foreground(lipgloss.Color("#00AAAA"))
	StyleError  = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF4444")).Bold(true)
	StyleInfo   = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FFFF"))
	StyleInput  = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF"))
	StyleDim    = lipgloss.NewStyle().Foreground(lipgloss.Color("#444444"))

	HeaderStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#00FF00")).
		Background(lipgloss.Color("#111111")).
		Bold(true).
		Padding(0, 1).
		Border(lipgloss.NormalBorder(), false, false, true, false).
		BorderForeground(lipgloss.Color("#00FF00"))

	FooterStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#888888")).
		Background(lipgloss.Color("#111111")).
		Padding(0, 1).
		Border(lipgloss.NormalBorder(), true, false, false, false).
		BorderForeground(lipgloss.Color("#444444"))
		
	StyleTermBox = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#00FF00")).
		Padding(0, 1)

	StyleChatBox = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#00AAAA")).
		Padding(0, 1)

	StyleChatHeader = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#000000")).
		Background(lipgloss.Color("#00AAAA")).
		Bold(true).
		Padding(0, 1)
)

const asciiArt = `
   .---.
  /   _ \
  |  ( ) |
  \   _ /
   '---'
`

// Init returns the initial command to run.
func (m *UIModel) Init() tea.Cmd {
	for _, line := range strings.Split(strings.TrimSpace(asciiArt), "\n") {
		m.Output = append(m.Output, StyleError.Render(line))
	}
	m.Output = append(m.Output, StylePrompt.Render("=== WELCOME TO DEADPORT ==="))
	m.Output = append(m.Output, "1. To see all missions, type: " + StyleInfo.Render("missions"))
	m.Output = append(m.Output, "2. To start a mission, type: " + StyleInfo.Render("start <id>"))
	m.Output = append(m.Output, "3. To chat with others, type: " + StyleInfo.Render("/chat <message>"))
	m.Output = append(m.Output, "4. To get a hint, type: " + StyleInfo.Render("help"))
	m.Output = append(m.Output, "")
	
	m.chatSub = m.Engine.Subscribe(m.Player.ID)
	return tea.Batch(
		listenForChat(m.chatSub),
		tickBlink(),
	)
}

// Update handles incoming messages.
func (m *UIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case blinkMsg:
		m.blink = !m.blink
		return m, tickBlink()
		
	case chatMsg:
		m.ChatOutput = append(m.ChatOutput, string(msg))
		if len(m.ChatOutput) > 100 {
			m.ChatOutput = m.ChatOutput[len(m.ChatOutput)-100:]
		}
		return m, listenForChat(m.chatSub)
		
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
		
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "ctrl+d":
			m.Engine.Unsubscribe(m.Player.ID)
			return m, tea.Quit
		case "up":
			// Scroll up
			if m.ScrollOffset < len(m.Output) {
				m.ScrollOffset++
			}
		case "down":
			// Scroll down
			if m.ScrollOffset > 0 {
				m.ScrollOffset--
			}
		case "backspace":
			if len(m.Input) > 0 {
				m.Input = m.Input[:len(m.Input)-1]
			}
		case "enter":
			// Process command
			m.processCommand()
		default:
			// Just a simple alphanumeric check
			k := msg.Key()
			if k.Text != "" {
				m.Input += k.Text
				m.ScrollOffset = 0 // jump to bottom on typing
			} else if msg.String() == "space" {
				m.Input += " "
			}
		}
	}
	return m, nil
}

func (m *UIModel) processCommand() {
	cmdStr := strings.TrimSpace(m.Input)
	if cmdStr == "" {
		m.Output = append(m.Output, m.getPrompt()+" ")
		return
	}
	
	// Echo the command
	m.Output = append(m.Output, m.getPrompt()+" "+cmdStr)
	
	// Handle game-specific outer commands
	parts := strings.Fields(cmdStr)
	switch parts[0] {
	case "exit":
		m.Output = append(m.Output, "Connection closed by user.")
		// We could send a tea.Quit here, but for now we just let the ssh session hang or we return tea.Quit cmd.
		// We'll let Update handle it if we want to quit. Actually we can't return a cmd here without modifying signature.
	case "whoami":
		m.Output = append(m.Output, m.Player.DisplayName+" ["+m.Player.Rank+"]")
	case "submit":
		if len(parts) < 3 {
			m.Output = append(m.Output, StyleError.Render("Usage: submit <mission_id> <answer>"))
		} else {
			ans := strings.Join(parts[2:], " ")
			ok, msg, err := m.Engine.SubmitAnswer(m.Player, parts[1], ans)
			if err != nil {
				m.Output = append(m.Output, StyleError.Render("Error: "+err.Error()))
			} else if ok {
				m.Output = append(m.Output, StyleInfo.Render(msg))
			} else {
				m.Output = append(m.Output, StyleError.Render(msg))
			}
		}
	case "missions":
		m.Output = append(m.Output, "--- AVAILABLE MISSIONS ---")
		for _, miss := range m.Engine.Catalog.Ordered {
			status := "[ ]"
			if _, done := m.Player.CompletedMissions[miss.ID]; done {
				status = "[X]"
			}
			line := status + " " + miss.ID + " - " + miss.Title + " (Diff " + string(rune('0'+miss.Difficulty)) + ")"
			if status == "[X]" {
				line = StyleInfo.Render(line)
			}
			m.Output = append(m.Output, line)
		}
	case "start":
		if len(parts) < 2 {
			m.Output = append(m.Output, StyleError.Render("Usage: start <mission_id>"))
		} else {
			mid := parts[1]
			miss, ok := m.Engine.Catalog.Missions[mid]
			if !ok {
				m.Output = append(m.Output, StyleError.Render("Mission not found."))
			} else {
				m.Output = append(m.Output, StyleInfo.Render("Starting mission: "+miss.Title))
				m.Output = append(m.Output, strings.Join(miss.Story, "\n"))
				m.Output = append(m.Output, StyleInfo.Render("Objective: "+miss.Objective))
				
				// Re-init shell for this mission
				m.Shell = terminal.NewShell(terminal.NewFS(), terminal.Options{
					User: m.Player.DisplayName,
					Tools: miss.Commands,
				})
				if err := miss.SetupFS(m.Shell.FS()); err != nil {
					m.Output = append(m.Output, StyleError.Render("FS Error: "+err.Error()))
				} else {
					if miss.Start != "" {
						m.Shell.Exec("cd " + miss.Start)
					}
				}
			}
		}
	default:
		// Execute in shell
		res := m.Shell.Exec(cmdStr)
		if res.Error {
			m.Output = append(m.Output, StyleError.Render(res.Output))
		} else if res.Output != "" {
			for _, line := range strings.Split(res.Output, "\n") {
				m.Output = append(m.Output, line)
			}
		}
		
		switch res.Action {
		case terminal.ActClear:
			m.Output = nil
		case terminal.ActExit:
			m.Output = append(m.Output, "Connection closed.")
			m.Engine.Unsubscribe(m.Player.ID)
			// Returning tea.Quit doesn't work directly here without changing signature,
			// but we can just append it to output and let them ctrl+c, or we can add a quit flag.
			// Actually, just let them ctrl+c for now, or add a special command return in processCommand.
		case terminal.ActChat:
			m.Engine.Broadcast(m.Player.DisplayName, res.Arg)
		}
	}
	
	m.Input = ""
}

func (m *UIModel) getPrompt() string {
	cwd := "~"
	if m.Shell != nil {
		cwd = m.Shell.Cwd()
	}
	return StylePrompt.Render(m.Player.DisplayName+"@deadport") + ":" + StyleDir.Render(cwd) + "$ "
}

// View renders the UI.
func (m *UIModel) View() tea.View {
	if m.Width == 0 || m.Height == 0 {
		return tea.NewView("")
	}

	headerText := fmt.Sprintf("💀 DEADPORT MAINFRAME  |  Operator: %s  |  Rank: %s  |  XP: %d", m.Player.DisplayName, m.Player.Rank, m.Player.XP)
	header := HeaderStyle.Width(m.Width).Render(headerText)
	footer := FooterStyle.Width(m.Width).Render(" [Up/Down] Scroll History  |  [Ctrl+C] Disconnect ")
	
	headerHeight := lipgloss.Height(header)
	footerHeight := lipgloss.Height(footer)
	
	contentHeight := m.Height - headerHeight - footerHeight
	if contentHeight < 1 {
		contentHeight = 1
	}
	
	termWidth := (m.Width * 7) / 10
	chatWidth := m.Width - termWidth

	// Terminal Output Calculation
	termInnerHeight := contentHeight - 2 // For borders
	termInnerWidth := termWidth - 4
	
	start := len(m.Output) - termInnerHeight + 1 - m.ScrollOffset
	if start < 0 { start = 0 }
	end := start + termInnerHeight - 1
	if end > len(m.Output) { end = len(m.Output) }
	
	var termContent strings.Builder
	for i := start; i < end; i++ {
		// Just truncate strings for now to fit inside border
		line := m.Output[i]
		termContent.WriteString(line + "\n")
	}
	termStr := termContent.String()
	lines := strings.Split(termStr, "\n")
	if len(lines) <= termInnerHeight - 1 {
		termStr += strings.Repeat("\n", (termInnerHeight - 1)-len(lines)+1)
	}
	
	// Autocomplete hint
	hint := ""
	cmds := []string{"missions", "start ", "submit ", "whoami", "help", "clear", "exit", "/chat "}
	for _, c := range cmds {
		if strings.HasPrefix(c, m.Input) && len(m.Input) > 0 {
			hint = c[len(m.Input):]
			break
		}
	}
	
	cursor := "█"
	if !m.blink {
		cursor = " "
	}
	
	promptTxt := m.getPrompt() + StyleInput.Render(m.Input) + StyleDim.Render(hint) + cursor
	termFull := lipgloss.JoinVertical(lipgloss.Left, termStr, promptTxt)
	termBox := StyleTermBox.Width(termInnerWidth).Height(termInnerHeight).Render(termFull)

	// Chat Output Calculation
	chatInnerHeight := contentHeight - 2 // For borders
	chatInnerWidth := chatWidth - 4
	chatHeader := StyleChatHeader.Render(" GLOBAL CHAT ")
	chatContentHeight := chatInnerHeight - lipgloss.Height(chatHeader)
	
	chatStart := len(m.ChatOutput) - chatContentHeight
	if chatStart < 0 { chatStart = 0 }
	
	var chatContent strings.Builder
	for i := chatStart; i < len(m.ChatOutput); i++ {
		chatContent.WriteString(m.ChatOutput[i] + "\n")
	}
	chatStr := chatContent.String()
	chatStr = lipgloss.PlaceVertical(chatContentHeight, lipgloss.Bottom, chatStr)
	
	chatFull := lipgloss.JoinVertical(lipgloss.Left, chatHeader, chatStr)
	chatBox := StyleChatBox.Width(chatInnerWidth).Height(chatInnerHeight).Render(chatFull)

	// Join all
	middle := lipgloss.JoinHorizontal(lipgloss.Top, termBox, chatBox)
	
	finalView := lipgloss.JoinVertical(lipgloss.Left,
		header,
		middle,
		footer,
	)
	
	v := tea.NewView(finalView)
	v.WindowTitle = "DEADPORT"
	return v
}
