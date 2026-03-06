package main

import (
	"fmt"
	"net"
	"strings"
)

// LogListener listens for UDP log packets from a Source Dedicated Server.
// The server sends log lines via `logaddress_add` to the listener's address.
type LogListener struct {
	conn    *net.UDPConn
	port    int
	logChan chan string
	done    chan struct{}
}

// NewLogListener creates a UDP listener on the specified port.
func NewLogListener(port int) (*LogListener, error) {
	addr := &net.UDPAddr{
		Port: port,
		IP:   net.IPv4zero,
	}

	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return nil, fmt.Errorf("listen udp on port %d: %w", port, err)
	}

	return &LogListener{
		conn:    conn,
		port:    port,
		logChan: make(chan string, 256),
		done:    make(chan struct{}),
	}, nil
}

// Start begins listening for log packets. It runs in the background.
// Received log lines are sent to the channel returned by Lines().
func (l *LogListener) Start() {
	go l.listen()
}

func (l *LogListener) listen() {
	defer close(l.logChan)

	buf := make([]byte, 4096)
	for {
		select {
		case <-l.done:
			return
		default:
		}

		n, _, err := l.conn.ReadFromUDP(buf)
		if err != nil {
			select {
			case <-l.done:
				return
			default:
				continue
			}
		}

		if n == 0 {
			continue
		}

		line := l.parsePacket(buf[:n])
		if line != "" {
			select {
			case l.logChan <- line:
			default:
				// Drop if channel is full to avoid blocking.
			}
		}
	}
}

// parsePacket extracts the log text from a Source engine UDP log packet.
// The packet format is: 0xFF 0xFF 0xFF 0xFF 'R' 'L' ' ' <message> '\n' '\0'
// We also handle plain text packets.
func (l *LogListener) parsePacket(data []byte) string {
	// Source engine log packets start with 4 bytes of 0xFF followed by "RL "
	if len(data) > 7 &&
		data[0] == 0xFF && data[1] == 0xFF &&
		data[2] == 0xFF && data[3] == 0xFF {
		// Skip the header (0xFFFFFFFF + "RL ")
		msg := data[4:]
		if len(msg) > 2 && msg[0] == 'R' && msg[1] == 'L' && msg[2] == ' ' {
			msg = msg[3:]
		}
		line := strings.TrimRight(string(msg), "\x00\n\r")
		return strings.TrimSpace(line)
	}

	// Fallback: treat as plain text
	line := strings.TrimRight(string(data), "\x00\n\r")
	return strings.TrimSpace(line)
}

// Lines returns a read-only channel of parsed log lines.
func (l *LogListener) Lines() <-chan string {
	return l.logChan
}

// Port returns the port the listener is bound to.
func (l *LogListener) Port() int {
	return l.port
}

// Close stops the listener and closes the UDP connection.
func (l *LogListener) Close() error {
	close(l.done)
	return l.conn.Close()
}
