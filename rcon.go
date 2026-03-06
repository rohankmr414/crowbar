package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	packetAuth         int32 = 3
	packetAuthResponse int32 = 2
	packetExecCommand  int32 = 2
	packetResponse     int32 = 0

	packetHeaderSize = 10
)

// RCONClient implements a raw Source Engine RCON protocol client with proper
// multi-packet response handling (which standard Go RCON libraries lack).
type RCONClient struct {
	conn             net.Conn
	mu               sync.Mutex
	addr             string
	timeout          time.Duration
	packetIDSequence int32
}

func Connect(addr, password string) (*RCONClient, error) {
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("rcon connect to %s: %w", addr, err)
	}

	c := &RCONClient{
		conn:             conn,
		addr:             addr,
		timeout:          30 * time.Second,
		packetIDSequence: 100, // start at arbitrary ID
	}

	// Authenticate
	if err := c.auth(password); err != nil {
		_ = c.conn.Close()
		return nil, fmt.Errorf("rcon auth failed: %w", err)
	}

	return c, nil
}

// readPacket reads a single RCON packet from the TCP stream.
func (c *RCONClient) readPacket() (id int32, pType int32, body string, err error) {
	_ = c.conn.SetReadDeadline(time.Now().Add(c.timeout))

	// Read packet size (4 bytes)
	var size int32
	if err := binary.Read(c.conn, binary.LittleEndian, &size); err != nil {
		return 0, 0, "", err
	}

	if size < packetHeaderSize || size > 8192 {
		return 0, 0, "", fmt.Errorf("invalid packet size: %d", size)
	}

	// Read the rest of the packet
	payload := make([]byte, size)
	if _, err := io.ReadFull(c.conn, payload); err != nil {
		return 0, 0, "", err
	}

	// Parse payload: ID (4) + Type (4) + Body (size-10) + Null x2 (2)
	id = int32(binary.LittleEndian.Uint32(payload[0:4]))
	pType = int32(binary.LittleEndian.Uint32(payload[4:8]))

	// -10 because ID(4) + Type(4) + two null bytes(2) = 10
	bodyBytes := payload[8 : size-2]
	body = string(bytes.TrimSuffix(bodyBytes, []byte{0x00}))

	return id, pType, body, nil
}

// writePacket constructs and sends an RCON packet
func (c *RCONClient) writePacket(id int32, pType int32, body string) error {
	_ = c.conn.SetWriteDeadline(time.Now().Add(c.timeout))

	bodyBytes := []byte(body)
	size := int32(len(bodyBytes) + packetHeaderSize)

	buf := new(bytes.Buffer)
	_ = binary.Write(buf, binary.LittleEndian, size)
	_ = binary.Write(buf, binary.LittleEndian, id)
	_ = binary.Write(buf, binary.LittleEndian, pType)
	buf.Write(bodyBytes)
	buf.Write([]byte{0x00, 0x00})

	_, err := c.conn.Write(buf.Bytes())
	return err
}

func (c *RCONClient) auth(password string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	id := c.packetIDSequence
	c.packetIDSequence++

	if err := c.writePacket(id, packetAuth, password); err != nil {
		return err
	}

	// Wait for response padding packet (some engines send this, some don't)
	respID, pType, _, err := c.readPacket()
	if err != nil {
		return err
	}

	if pType == packetResponse {
		// Read the actual auth response packet
		respID, pType, _, err = c.readPacket()
		if err != nil {
			return err
		}
	}

	if pType != packetAuthResponse {
		return fmt.Errorf("unexpected packet type during auth: %d", pType)
	}

	if respID == -1 {
		return fmt.Errorf("invalid password")
	}
	if respID != id {
		return fmt.Errorf("auth packet ID mismatch: got %d, want %d", respID, id)
	}

	return nil
}

// Execute sends a command and seamlessly aggregates multi-packet responses.
func (c *RCONClient) Execute(cmd string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	cmdID := c.packetIDSequence
	c.packetIDSequence++

	// 1. Send the actual command.
	if err := c.writePacket(cmdID, packetExecCommand, cmd); err != nil {
		return "", fmt.Errorf("rcon execute %q: %w", cmd, err)
	}

	// 2. Send an empty dummy packet with a known ID.
	// The Source engine will process the commands sequentially. Because large outputs
	// are split into multiple packets of ~4KB, we don't know how many packets to read.
	// However, the engine will mirror back the dummy packet ID *after* it has finished
	// sending all packets for the first command.
	dummyID := c.packetIDSequence
	c.packetIDSequence++

	// Send empty request to trigger the mirror
	if err := c.writePacket(dummyID, packetExecCommand, ""); err != nil {
		return "", fmt.Errorf("failed to send terminal packet: %w", err)
	}

	var responseBuilder strings.Builder

	// 3. Read continuously until we see the dummy packet ID echoed back.
	for {
		pID, pType, body, err := c.readPacket()
		if err != nil {
			if len(responseBuilder.String()) > 0 {
				// If we got *some* data before the error, just return what we have (best effort)
				return responseBuilder.String(), nil
			}
			return "", err
		}

		if pType == packetResponse {
			if pID == dummyID {
				// We reached the end of the multi-packet stream via the echoed dummy packet!
				break
			}
			if pID == cmdID {
				// Some Valve forks (CS:GO, TF2) don't actually mirror the dummy packet ID correctly.
				// Instead they will send an completely empty body packet with the *original* cmdID
				// to signal the end of the stream.
				if body == "" || body == "\x00\x00" {
					break
				}
				// Part of our actual response output
				responseBuilder.WriteString(body)
			}
		}
	}

	return responseBuilder.String(), nil
}

// Close closes the RCON connection.
func (c *RCONClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.Close()
}

// Addr returns the server address this client is connected to.
func (c *RCONClient) Addr() string {
	return c.addr
}

// rconFromPattern matches 'rcon from "IP:port"' in server responses.
var rconFromPattern = regexp.MustCompile(`rcon from "(\d+\.\d+\.\d+\.\d+):\d+"`)

// DetectPublicIP sends a command and parses the server's response to find
// our public IP as seen by the server (from "rcon from <ip>:<port>" log lines).
func (c *RCONClient) DetectPublicIP() (string, error) {
	resp, err := c.Execute("echo crowbar-ip-detect")
	if err != nil {
		return "", err
	}

	matches := rconFromPattern.FindStringSubmatch(resp)
	if len(matches) >= 2 {
		return matches[1], nil
	}

	return "", fmt.Errorf("could not detect public IP from server response")
}

// gamePattern matches '(game)' in the `version` command output.
// e.g., "Exe version 1.38.8.1 (csgo)" -> "csgo"
var gamePattern = regexp.MustCompile(`\(([^)]+)\)`)

// DetectGame queries the server version and extracts the engine/game identifier.
func (c *RCONClient) DetectGame() string {
	resp, err := c.Execute("version")
	if err != nil {
		return "default"
	}

	// Look for the string cleanly inside parentheses.
	matches := gamePattern.FindStringSubmatch(resp)
	if len(matches) >= 2 {
		game := strings.ToLower(strings.TrimSpace(matches[1]))
		// Map known engine identifiers to our themes
		switch game {
		case "csgo", "cs2":
			return "csgo"
		case "tf", "tf2":
			return "tf2"
		case "garrysmod", "gmod":
			return "gmod"
		default:
			return "default"
		}
	}

	return "default"
}
