package main

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// --- Theming ---

type Theme struct {
	Primary   lipgloss.Color
	Secondary lipgloss.Color
	Success   lipgloss.Color
	Warning   lipgloss.Color
	Error     lipgloss.Color
	Surface   lipgloss.Color
	SurfaceHL lipgloss.Color
	Border    lipgloss.Color
	Text      lipgloss.Color
	Dim       lipgloss.Color
	Accent    lipgloss.Color

	// Pre-computed styles
	Title                lipgloss.Style
	StatusBar            lipgloss.Style
	StatusConnected      lipgloss.Style
	StatusDisconnected   lipgloss.Style
	LogPanel             lipgloss.Style
	Input                lipgloss.Style
	Prompt               lipgloss.Style
	Response             lipgloss.Style
	ErrorLog             lipgloss.Style
	ServerLog            lipgloss.Style
	CmdEcho              lipgloss.Style
	AutocompleteBox      lipgloss.Style
	AutocompleteItem     lipgloss.Style
	AutocompleteSelected lipgloss.Style
	AutocompleteCmd      lipgloss.Style
	AutocompleteDesc     lipgloss.Style
	Help                 lipgloss.Style
}

// Predefined color palettes
var themes = map[string]Theme{
	"default": {
		Primary:   "#7C3AED", // Purple
		Secondary: "#06B6D4", // Cyan
		Success:   "#10B981", // Green
		Warning:   "#F59E0B", // Amber
		Error:     "#EF4444", // Red
		Surface:   "#1E1B2E",
		SurfaceHL: "#2A2640",
		Border:    "#4C3F8F",
		Text:      "#E2E0F0",
		Dim:       "#6B6589",
		Accent:    "#A78BFA",
	},
	"csgo": {
		Primary:   "#EAB308", // Yellow
		Secondary: "#3B82F6", // Blue
		Success:   "#22C55E",
		Warning:   "#F59E0B",
		Error:     "#EF4444",
		Surface:   "#171720", // Dark grey/blue
		SurfaceHL: "#232332",
		Border:    "#334155",
		Text:      "#F1F5F9",
		Dim:       "#64748B",
		Accent:    "#60A5FA",
	},
	"tf2": {
		Primary:   "#B91C1C", // Dark Red
		Secondary: "#D97700", // TF2 Orange
		Success:   "#22C55E",
		Warning:   "#FACC15",
		Error:     "#EF4444",
		Surface:   "#1C1918", // Warm dark
		SurfaceHL: "#2C2624",
		Border:    "#57534E",
		Text:      "#FFF7ED",
		Dim:       "#78716C",
		Accent:    "#FDBA74",
	},
	"gmod": {
		Primary:   "#38BDF8", // Light Blue
		Secondary: "#F8FAFC", // White
		Success:   "#4ADE80",
		Warning:   "#FBBF24",
		Error:     "#F87171",
		Surface:   "#0F172A", // Deep blue
		SurfaceHL: "#1E293B",
		Border:    "#38BDF8",
		Text:      "#F8FAFC",
		Dim:       "#94A3B8",
		Accent:    "#7DD3FC",
	},
}

func initTheme(name string) Theme {
	t, ok := themes[strings.ToLower(name)]
	if !ok {
		t = themes["default"]
	}

	t.Title = lipgloss.NewStyle().
		Bold(true).
		Foreground(t.Primary).
		Background(lipgloss.Color("#0F0D1A")).
		Padding(0, 1)

	t.StatusBar = lipgloss.NewStyle().
		Foreground(t.Text).
		Background(lipgloss.Color("#1A1730")).
		Padding(0, 1)

	t.StatusConnected = lipgloss.NewStyle().Foreground(t.Success).Bold(true)
	t.StatusDisconnected = lipgloss.NewStyle().Foreground(t.Error).Bold(true)

	t.LogPanel = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Border).
		Padding(0, 1)

	t.Input = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Secondary).
		Padding(0, 1)

	t.Prompt = lipgloss.NewStyle().Foreground(t.Secondary).Bold(true)
	t.Response = lipgloss.NewStyle().Foreground(t.Accent)
	t.ErrorLog = lipgloss.NewStyle().Foreground(t.Error)
	t.ServerLog = lipgloss.NewStyle().Foreground(t.Warning)
	t.CmdEcho = lipgloss.NewStyle().Foreground(t.Secondary).Bold(true)

	t.AutocompleteBox = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Accent).
		Background(t.Surface).
		Padding(0, 1)

	t.AutocompleteItem = lipgloss.NewStyle().Foreground(t.Text)
	t.AutocompleteSelected = lipgloss.NewStyle().
		Foreground(t.Primary).
		Background(t.SurfaceHL).
		Bold(true)
	t.AutocompleteCmd = lipgloss.NewStyle().Foreground(t.Secondary).Bold(true)
	t.AutocompleteDesc = lipgloss.NewStyle().Foreground(t.Dim)
	t.Help = lipgloss.NewStyle().Foreground(t.Dim)

	return t
}

// --- Messages ---

// logLineMsg is sent when a new log line comes from the UDP listener.
type logLineMsg string

// rconResponseMsg is sent when an RCON command response is received.
type rconResponseMsg struct {
	cmd         string
	response    string
	err         error
	reconnected bool // indicates if we had to auto-reconnect before sending
}

// connectResultMsg is returned after attempting to register log address.
type connectResultMsg struct {
	err error
}

// findResultMsg is returned after querying the server with `find <prefix>`.
type findResultMsg struct {
	prefix   string
	commands []Command
	err      error
}

// --- Model ---

const maxLogLines = 20000
const maxAutocompleteSuggestions = 8

type model struct {
	// Components
	viewport  viewport.Model
	textInput textinput.Model

	// State
	logLines     []string
	logListener  *LogListener
	serverAddr   string
	rconPassword string // stored for one-shot connections
	publicIP     string // client's public IP for logaddress_add
	connected    bool
	width        int
	height       int
	theme        Theme

	// Autocomplete state
	suggestions     []Command
	selectedIdx     int
	showSuggestions bool
	findCache       map[string][]Command // cache of find query results
	lastFindPrefix  string               // last prefix sent to server
	findInFlight    bool                 // whether a find query is pending

	// Command history
	history    []string
	historyIdx int
}

func newModel(connected bool, logListener *LogListener, serverAddr string, rconPassword string, publicIP string, themeName string) model {
	t := initTheme(themeName)

	ti := textinput.New()
	ti.Placeholder = "Type a command... (Tab to autocomplete)"
	ti.Focus()
	ti.CharLimit = 512
	ti.Prompt = "❯ "
	ti.PromptStyle = t.Prompt
	ti.TextStyle = lipgloss.NewStyle().Foreground(t.Text)
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(t.Dim)

	vp := viewport.New(80, 20)

	return model{
		viewport:     vp,
		textInput:    ti,
		logListener:  logListener,
		serverAddr:   serverAddr,
		rconPassword: rconPassword,
		publicIP:     publicIP,
		connected:    connected,
		history:      make([]string, 0),
		historyIdx:   -1,
		findCache:    make(map[string][]Command),
		theme:        t,
	}
}

// listenForLogs returns a command that waits for the next log line from the UDP listener.
func listenForLogs(logCh <-chan string) tea.Cmd {
	return func() tea.Msg {
		line, ok := <-logCh
		if !ok {
			return nil
		}
		return logLineMsg(line)
	}
}

// executeRCON sends an RCON command asynchronously using a fresh connection.
// This prevents complex commands like `cvarlist` from bleeding TCP buffer into subsequent commands.
func executeRCON(addr, password, cmd string) tea.Cmd {
	return func() tea.Msg {
		if password == "" {
			return rconResponseMsg{cmd: cmd, err: fmt.Errorf("not connected")}
		}

		// Connect inside the command worker
		c, err := Connect(addr, password)
		if err != nil {
			return rconResponseMsg{cmd: cmd, err: err}
		}
		defer c.Close()

		resp, err := c.Execute(cmd)
		return rconResponseMsg{cmd: cmd, response: resp, err: err}
	}
}

// registerLogAddress tells the server to send logs to our listener
// using our detected public IP.
func registerLogAddress(client *RCONClient, publicIP string, port int) tea.Cmd {
	return func() tea.Msg {
		cmd := fmt.Sprintf("logaddress_add %s:%d", publicIP, port)
		_, err := client.Execute(cmd)
		return connectResultMsg{err: err}
	}
}

func (m model) Init() tea.Cmd {
	var cmds []tea.Cmd

	// We must enable mouse events for scroll wheel support
	cmds = append(cmds, textinput.Blink)

	if m.logListener != nil {
		cmds = append(cmds, listenForLogs(m.logListener.Lines()))
	}

	if m.connected && m.logListener != nil && m.publicIP != "" {
		// Log address add requires an RCON connection
		cmds = append(cmds, func() tea.Msg {
			c, err := Connect(m.serverAddr, m.rconPassword)
			if err != nil {
				return connectResultMsg{err: err}
			}
			defer c.Close()
			cmd := fmt.Sprintf("logaddress_add %s:%d", m.publicIP, m.logListener.Port())
			_, err = c.Execute(cmd)
			return connectResultMsg{err: err}
		})
	}

	return tea.Batch(cmds...)
}

// findOnServer creates a fresh RCON connection, runs `find <prefix>`, and disconnects.
// Using a one-shot connection avoids TCP buffer bleed from large responses.
func findOnServer(addr, password, prefix string) tea.Cmd {
	return func() tea.Msg {
		client, err := Connect(addr, password)
		if err != nil {
			return findResultMsg{prefix: prefix, err: err}
		}
		defer client.Close()

		resp, err := client.Execute("find " + prefix)
		if err != nil {
			return findResultMsg{prefix: prefix, err: err}
		}
		cmds := ParseFindOutput(resp)
		return findResultMsg{prefix: prefix, commands: cmds}
	}
}

// isRCONNoise returns true if a response line is RCON protocol noise
// that should be filtered from display.
func isRCONNoise(line string) bool {
	// Log lines echoed by the engine about our own commands.
	if strings.HasPrefix(line, "L ") && strings.Contains(line, "rcon from") {
		return true
	}
	if strings.HasPrefix(line, "L ") && strings.Contains(line, "Log file closed") {
		return true
	}
	if strings.HasPrefix(line, "L ") && strings.Contains(line, "Log file started") {
		return true
	}
	return false
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKeyPress(msg)

	case tea.MouseMsg:
		// Basic mouse wheel scroll for viewport
		if msg.Type == tea.MouseWheelUp {
			m.viewport.LineUp(3)
		} else if msg.Type == tea.MouseWheelDown {
			m.viewport.LineDown(3)
		}
		// Return early to prevent mouse events from falling through to the text input
		// which causes raw ANSI escape sequences to be typed into the command line.
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updateLayout()
		return m, nil

	case logLineMsg:
		line := string(msg)
		m.appendLog(m.theme.ServerLog.Render("│ ") + line)
		m.viewport.SetContent(strings.Join(m.logLines, "\n"))
		m.viewport.GotoBottom()
		cmds = append(cmds, listenForLogs(m.logListener.Lines()))
		return m, tea.Batch(cmds...)

	case rconResponseMsg:
		if msg.reconnected {
			m.appendLog(lipgloss.NewStyle().Foreground(m.theme.Success).Render("↻ Auto-reconnected to server"))
		}

		if msg.err != nil {
			m.appendLog(m.theme.ErrorLog.Render("✗ Error: " + msg.err.Error()))
		} else if msg.response != "" {
			// Split response into lines and add each, filtering noise.
			for _, line := range strings.Split(strings.TrimSpace(msg.response), "\n") {
				if !isRCONNoise(line) {
					m.appendLog(m.theme.Response.Render("  " + line))
				}
			}
		}
		m.viewport.SetContent(strings.Join(m.logLines, "\n"))
		m.viewport.GotoBottom()
		return m, nil

	case connectResultMsg:
		if msg.err != nil {
			m.appendLog(m.theme.ErrorLog.Render("✗ Failed to register log address: " + msg.err.Error()))
		} else {
			m.appendLog(lipgloss.NewStyle().Foreground(m.theme.Success).Render(
				fmt.Sprintf("✓ Log streaming registered on port %d", m.logListener.Port())))
		}
		m.viewport.SetContent(strings.Join(m.logLines, "\n"))
		m.viewport.GotoBottom()
		return m, nil

	case findResultMsg:
		m.findInFlight = false
		if msg.err == nil && len(msg.commands) > 0 {
			// Cache the server results.
			m.findCache[msg.prefix] = msg.commands
			// Refresh suggestions if the user is still typing the same prefix.
			currentInput := strings.TrimSpace(m.textInput.Value())
			if strings.HasPrefix(strings.ToLower(currentInput), strings.ToLower(msg.prefix)) {
				m.refreshSuggestions()
			}
		}
		return m, nil
	}

	return m, tea.Batch(cmds...)
}

func (m *model) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC, tea.KeyEsc:
		return m, tea.Quit

	case tea.KeyEnter:
		return m.handleEnter()

	case tea.KeyCtrlL:
		m.logLines = []string{}
		m.viewport.SetContent("")
		return m, nil

	case tea.KeyTab:
		return m.handleTab()

	case tea.KeyUp:
		if m.showSuggestions && len(m.suggestions) > 0 {
			m.selectedIdx--
			if m.selectedIdx < 0 {
				m.selectedIdx = len(m.suggestions) - 1
			}
			return m, nil
		}
		// Navigate history
		if len(m.history) > 0 {
			if m.historyIdx < len(m.history)-1 {
				m.historyIdx++
			}
			m.textInput.SetValue(m.history[len(m.history)-1-m.historyIdx])
			m.textInput.CursorEnd()
			m.updateSuggestions()
		}
		return m, nil

	case tea.KeyDown:
		if m.showSuggestions && len(m.suggestions) > 0 {
			m.selectedIdx++
			if m.selectedIdx >= len(m.suggestions) {
				m.selectedIdx = 0
			}
			return m, nil
		}
		// Navigate history
		if m.historyIdx > 0 {
			m.historyIdx--
			m.textInput.SetValue(m.history[len(m.history)-1-m.historyIdx])
			m.textInput.CursorEnd()
			m.updateSuggestions()
		} else if m.historyIdx == 0 {
			m.historyIdx = -1
			m.textInput.SetValue("")
			m.showSuggestions = false
		}
		return m, nil

	case tea.KeyPgUp:
		m.viewport.HalfViewUp()
		return m, nil

	case tea.KeyPgDown:
		m.viewport.HalfViewDown()
		return m, nil

	default:
		var inputCmd tea.Cmd
		m.textInput, inputCmd = m.textInput.Update(msg)

		// Sanitize mouse event leaks. When scrolling rapidly, some terminal emulators
		// drop the \x1b byte, causing raw SGR mouse sequences to bleed into the text input.
		val := m.textInput.Value()
		if strings.Contains(val, "<") || strings.Contains(val, "M") || strings.Contains(val, "m") {
			// Matches [<65;101;49M or <65;101;49M
			re := regexp.MustCompile(`(?:\[<|<)\d+;\d+;\d+[mM]`)
			newVal := re.ReplaceAllString(val, "")

			// Also strip the rogue `)` character that sometimes prefixes these in VTE terminals
			newVal = strings.ReplaceAll(newVal, ") [<", "[<")
			newVal = strings.ReplaceAll(newVal, ") <", "<")

			if newVal != val {
				m.textInput.SetValue(newVal)
			}
		}

		m.updateSuggestions()
		findCmd := m.maybeFindOnServer()
		if findCmd != nil {
			return m, tea.Batch(inputCmd, findCmd)
		}
		return m, inputCmd
	}
}

func (m *model) handleEnter() (tea.Model, tea.Cmd) {
	input := strings.TrimSpace(m.textInput.Value())

	// Intercept local commands before autocomplete
	lowerInput := strings.ToLower(input)
	switch lowerInput {
	case "clear":
		m.textInput.SetValue("")
		m.showSuggestions = false
		m.history = append(m.history, input)
		m.historyIdx = -1

		m.logLines = []string{}
		m.viewport.SetContent("")
		return m, nil
	case "exit", "quit":
		return m, tea.Quit
	}

	// If autocomplete is showing and an item is selected, apply it.
	if m.showSuggestions && len(m.suggestions) > 0 {
		m.applySuggestion()
		return m, nil
	}

	if input == "" {
		return m, nil
	}

	m.textInput.SetValue("")
	m.showSuggestions = false
	m.history = append(m.history, input)
	m.historyIdx = -1

	// Echo the command.
	m.appendLog(m.theme.CmdEcho.Render("❯ " + input))
	m.viewport.SetContent(strings.Join(m.logLines, "\n"))
	m.viewport.GotoBottom()

	if !m.connected {
		m.appendLog(m.theme.ErrorLog.Render("✗ Not connected to any server"))
		m.viewport.SetContent(strings.Join(m.logLines, "\n"))
		return m, nil
	}

	return m, executeRCON(m.serverAddr, m.rconPassword, input)
}

func (m *model) handleTab() (tea.Model, tea.Cmd) {
	if len(m.suggestions) > 0 {
		m.applySuggestion()
	}
	return m, nil
}

func (m *model) applySuggestion() {
	if len(m.suggestions) == 0 {
		return
	}
	idx := m.selectedIdx
	if idx < 0 || idx >= len(m.suggestions) {
		idx = 0
	}
	m.textInput.SetValue(m.suggestions[idx].Name + " ")
	m.textInput.CursorEnd()
	m.showSuggestions = false
	m.selectedIdx = 0
}

func (m *model) updateSuggestions() {
	m.refreshSuggestions()
}

// refreshSuggestions updates the autocomplete list from curated + cached server commands.
func (m *model) refreshSuggestions() {
	input := m.textInput.Value()
	if input == "" {
		m.suggestions = nil
		m.showSuggestions = false
		m.selectedIdx = 0
		return
	}

	// Collect all matching commands from cached server find results.
	seen := make(map[string]bool)
	var matches []Command
	for _, cmds := range m.findCache {
		for _, c := range cmds {
			if !seen[c.Name] && strings.HasPrefix(strings.ToLower(c.Name), strings.ToLower(input)) {
				matches = append(matches, c)
				seen[c.Name] = true
			}
		}
	}

	// Don't show if it's an exact match already.
	if len(matches) == 1 && strings.EqualFold(matches[0].Name, input) {
		m.suggestions = nil
		m.showSuggestions = false
		return
	}

	if len(matches) > maxAutocompleteSuggestions {
		matches = matches[:maxAutocompleteSuggestions]
	}

	m.suggestions = matches
	m.showSuggestions = len(matches) > 0
	if m.selectedIdx >= len(matches) {
		m.selectedIdx = 0
	}
}

// maybeFindOnServer triggers an async `find` query if the input is long enough
// and we haven't already queried this prefix. Creates a one-shot RCON connection
// for each query to avoid TCP buffer bleed from large responses.
func (m *model) maybeFindOnServer() tea.Cmd {
	if m.rconPassword == "" || m.findInFlight {
		return nil
	}

	input := strings.TrimSpace(m.textInput.Value())
	if len(input) < 3 {
		return nil
	}

	// Use the input as the find prefix.
	prefix := strings.ToLower(input)

	// Check if we already have cached results for this prefix.
	if _, ok := m.findCache[prefix]; ok {
		return nil
	}

	// Don't re-query the same prefix.
	if prefix == m.lastFindPrefix {
		return nil
	}

	m.lastFindPrefix = prefix
	m.findInFlight = true
	return findOnServer(m.serverAddr, m.rconPassword, prefix)
}

func (m *model) appendLog(line string) {
	m.logLines = append(m.logLines, line)
	if len(m.logLines) > maxLogLines {
		m.logLines = m.logLines[len(m.logLines)-maxLogLines:]
	}
}

func (m *model) updateLayout() {
	// Reserve space: title(1) + status(1) + input border(3) + help(1) + autocomplete
	headerHeight := 1
	statusHeight := 1
	inputHeight := 3
	helpHeight := 1
	padding := 2

	vpHeight := m.height - headerHeight - statusHeight - inputHeight - helpHeight - padding
	if vpHeight < 3 {
		vpHeight = 3
	}

	vpWidth := m.width - 4 // border + padding
	if vpWidth < 10 {
		vpWidth = 10
	}

	m.viewport.Width = vpWidth
	m.viewport.Height = vpHeight
	m.textInput.Width = vpWidth - 4
}

func (m model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	var sections []string

	// Title bar
	title := m.theme.Title.Width(m.width).Render(
		"  🔧 crowbar")
	sections = append(sections, title)

	// Status bar
	var connStatus string
	if m.connected {
		connStatus = m.theme.StatusConnected.Render("● Connected") +
			m.theme.StatusBar.Render(" to ") +
			lipgloss.NewStyle().Foreground(m.theme.Accent).Bold(true).Render(m.serverAddr)
	} else {
		connStatus = m.theme.StatusDisconnected.Render("● Disconnected")
	}
	logInfo := lipgloss.NewStyle().Foreground(m.theme.Dim).Render(
		fmt.Sprintf("  UDP :%d", m.logListener.Port()))
	scrollInfo := lipgloss.NewStyle().Foreground(m.theme.Dim).Render(
		fmt.Sprintf("  %d lines", len(m.logLines)))

	statusLine := m.theme.StatusBar.Width(m.width).Render(
		connStatus + logInfo + scrollInfo)
	sections = append(sections, statusLine)

	// Log viewport
	logPanel := m.theme.LogPanel.Width(m.width - 2).Render(m.viewport.View())
	sections = append(sections, logPanel)

	// Autocomplete popup (rendered between log panel and input)
	if m.showSuggestions && len(m.suggestions) > 0 {
		var items []string
		for i, s := range m.suggestions {
			cmd := m.theme.AutocompleteCmd.Render(s.Name)
			desc := m.theme.AutocompleteDesc.Render(" — " + s.Description)
			line := cmd + desc

			if i == m.selectedIdx {
				line = m.theme.AutocompleteSelected.Render("▸ ") + line
			} else {
				line = m.theme.AutocompleteItem.Render("  ") + line
			}
			items = append(items, line)
		}
		acBox := m.theme.AutocompleteBox.Render(strings.Join(items, "\n"))
		sections = append(sections, acBox)
	}

	// Command input
	inputBox := m.theme.Input.Width(m.width - 2).Render(m.textInput.View())
	sections = append(sections, inputBox)

	// Help line
	help := m.theme.Help.Render(
		"  Tab: autocomplete • ↑↓: history/select • PgUp/PgDn: scroll • Enter: send • Esc: quit")
	sections = append(sections, help)

	return strings.Join(sections, "\n")
}
