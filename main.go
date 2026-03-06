package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	flag "github.com/spf13/pflag"
)

func main() {
	host := flag.StringP("host", "H", "127.0.0.1", "Server host/IP address")
	port := flag.IntP("port", "p", 27015, "Server RCON port")
	password := flag.StringP("password", "P", "", "RCON password (required)")
	logPort := flag.IntP("log-port", "l", 27115, "Local UDP port for log streaming")
	themeName := flag.StringP("theme", "t", "", "Force a specific UI theme (csgo, tf2, gmod, default)")
	publicIP := flag.String("public-ip", "", "Your public IP for log streaming (auto-detected if empty)")
	flag.Parse()

	if *password == "" {
		fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444")).Bold(true).Render(
			"Error: --password / -P is required"))
		fmt.Println()
		fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("#A78BFA")).Render(
			"Usage: crowbar -H <host> -p <port> -P <password> [-l <log-port>]"))
		fmt.Println()
		fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("#6B6589")).Render(
			"Example: crowbar -H 192.168.1.100 -p 27015 -P mysecretpass"))
		os.Exit(1)
	}

	serverAddr := fmt.Sprintf("%s:%d", *host, *port)

	// Start the UDP log listener.
	logListener, err := NewLogListener(*logPort)
	if err != nil {
		fmt.Printf("Failed to start log listener on port %d: %v\n", *logPort, err)
		os.Exit(1)
	}
	defer func() {
		_ = logListener.Close()
	}()

	// Connect via RCON.
	fmt.Printf("Connecting to %s...\n", serverAddr)
	rconClient, err := Connect(serverAddr, *password)
	if err != nil {
		fmt.Printf("Failed to connect: %v\n", err)
		fmt.Println("Starting in disconnected mode. You can still view log output.")
		rconClient = nil
	} else {
		defer func() {
			_ = rconClient.Close()
		}()
	}

	// Auto-detect public IP if not specified.
	detectedIP := *publicIP
	if rconClient != nil && detectedIP == "" {
		ip, err := rconClient.DetectPublicIP()
		if err == nil {
			detectedIP = ip
			fmt.Printf("Detected public IP: %s\n", detectedIP)
		}
	}

	// Determine theme
	activeTheme := "default"
	if *themeName != "" {
		activeTheme = *themeName
	} else if rconClient != nil {
		activeTheme = rconClient.DetectGame()
	}

	// Create and run the TUI.
	m := newModel(rconClient != nil, logListener, serverAddr, *password, detectedIP, activeTheme)

	// Add initial log lines.
	m.logLines = append(m.logLines,
		lipgloss.NewStyle().Foreground(m.theme.Primary).Bold(true).Render(
			"╔══════════════════════════════════════════════╗"),
		lipgloss.NewStyle().Foreground(m.theme.Primary).Bold(true).Render(
			"║   🔧 crowbar                                 ║"),
		lipgloss.NewStyle().Foreground(m.theme.Primary).Bold(true).Render(
			"╚══════════════════════════════════════════════╝"),
		"",
	)

	if rconClient != nil {
		m.logLines = append(m.logLines,
			lipgloss.NewStyle().Foreground(m.theme.Success).Render(
				fmt.Sprintf("  ✓ Connected to %s", serverAddr)),
			lipgloss.NewStyle().Foreground(m.theme.Secondary).Render(
				fmt.Sprintf("  ✓ Log listener on UDP port %d", *logPort)),
		)
		if detectedIP != "" {
			m.logLines = append(m.logLines,
				lipgloss.NewStyle().Foreground(m.theme.Secondary).Render(
					fmt.Sprintf("  ✓ Public IP: %s (UDP log push enabled)", detectedIP)),
			)
		}
		m.logLines = append(m.logLines,
			"",
			lipgloss.NewStyle().Foreground(m.theme.Dim).Render(
				"  Type a command and press Enter. Start typing (3+ chars) for autocomplete."),
			"",
		)
	} else {
		m.logLines = append(m.logLines,
			lipgloss.NewStyle().Foreground(m.theme.Error).Render(
				fmt.Sprintf("  ✗ Could not connect to %s", serverAddr)),
			lipgloss.NewStyle().Foreground(lipgloss.Color("#F59E0B")).Render(
				"  Running in disconnected mode — log listener is still active."),
			"",
		)
	}

	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running TUI: %v\n", err)
		os.Exit(1)
	}
}
