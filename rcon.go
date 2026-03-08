package main

import (
	"bytes"
	"encoding/binary"
	"errors"
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
	// Valve docs state 4096 max packet size, which we enforce for outbound requests.
	maxRequestPacketSize = 4096
	// Some servers emit larger inbound chunks in practice (e.g. cvarlist), so be tolerant.
	maxResponsePacketSize = 8192
)

var ErrPartialResponse = errors.New("partial rcon response")

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
		packetIDSequence: 100, // start at arbitrary positive ID
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
	return c.readPacketWithTimeout(c.timeout)
}

func (c *RCONClient) readPacketWithTimeout(timeout time.Duration) (id int32, pType int32, body string, err error) {
	_ = c.conn.SetReadDeadline(time.Now().Add(timeout))

	// Read packet size (4 bytes)
	var size int32
	if err := binary.Read(c.conn, binary.LittleEndian, &size); err != nil {
		return 0, 0, "", err
	}

	if size < packetHeaderSize || size > maxResponsePacketSize {
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
	if payload[len(payload)-2] != 0x00 || payload[len(payload)-1] != 0x00 {
		return 0, 0, "", fmt.Errorf("malformed packet: missing null terminators")
	}

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
	if size > maxRequestPacketSize {
		return fmt.Errorf("packet too large: %d bytes (max %d)", size, maxRequestPacketSize)
	}

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

	id := c.nextPacketID()

	if err := c.writePacket(id, packetAuth, password); err != nil {
		return err
	}

	// Some engines send a packetResponse before packetAuthResponse.
	for {
		respID, pType, _, err := c.readPacket()
		if err != nil {
			return err
		}
		if pType == packetResponse {
			continue
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
}

// Execute sends a command and seamlessly aggregates multi-packet responses.
func (c *RCONClient) Execute(cmd string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	cmdID := c.nextPacketID()

	// 1. Send the actual command.
	if err := c.writePacket(cmdID, packetExecCommand, cmd); err != nil {
		return "", fmt.Errorf("rcon execute %q: %w", cmd, err)
	}

	// 2. Send an empty SERVERDATA_RESPONSE_VALUE (type 0) packet with a known ID.
	// This is an "erroneous" request that SRCDS mirrors back to the client rather than
	// executing. Because the server responds to requests in order, receiving this
	// mirrored packet guarantees all meaningful response packets have been received.
	// After the mirror, SRCDS also sends a follow-up packet with body 0x00000001 0x00000000.
	// Reference: https://developer.valvesoftware.com/wiki/Source_RCON_Protocol#Multiple-packet_Responses
	dummyID := c.nextPacketID()

	if err := c.writePacket(dummyID, packetResponse, ""); err != nil {
		return "", fmt.Errorf("failed to send terminal packet: %w", err)
	}

	var responseBuilder strings.Builder

	// 3. Read continuously until we see the mirrored dummy packet ID.
	for {
		pID, pType, body, err := c.readPacket()
		if err != nil {
			partial := responseBuilder.String()
			if partial != "" {
				return partial, fmt.Errorf("%w: %v", ErrPartialResponse, err)
			}
			return "", err
		}

		if pType == packetResponse {
			if pID == dummyID {
				// We received the mirrored dummy packet — all real response data is in.
				break
			}
			if pID == cmdID {
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

// nextPacketID returns the next request ID and avoids the -1 failure sentinel.
func (c *RCONClient) nextPacketID() int32 {
	c.packetIDSequence++
	if c.packetIDSequence <= 0 || c.packetIDSequence == -1 {
		c.packetIDSequence = 1
	}
	return c.packetIDSequence
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
