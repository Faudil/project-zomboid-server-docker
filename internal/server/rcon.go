package server

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/faudil/project-zomboid-server-docker/internal/config"
)

// Project Zomboid's dedicated server RCON speaks the standard Source RCON
// protocol: little-endian packets of [length][id][type][body]\x00\x00.
//
// On auth the server replies with an empty SERVERDATA_RESPONSE_VALUE
// acknowledgement followed by the auth result (SERVERDATA_AUTH_RESPONSE with
// the request id on success, -1 on failure). Commands are answered with a
// single value packet containing the output - there is no terminating packet.

const (
	rconTypeResponse = 0 // SERVERDATA_RESPONSE_VALUE
	rconTypeExec     = 2 // SERVERDATA_EXECCOMMAND / SERVERDATA_AUTH_RESPONSE
	rconTypeAuth     = 3 // SERVERDATA_AUTH

	// maxRCONPacket caps the size of an incoming packet so a corrupt or
	// malicious RCON peer cannot force a huge allocation.
	maxRCONPacket = 64 * 1024
)

type RCONClient struct {
	cfg    *config.ServerConfig
	conn   net.Conn
	nextID int32
}

func NewRCONClient(cfg *config.ServerConfig) *RCONClient {
	return &RCONClient{cfg: cfg}
}

func (r *RCONClient) newID() int32 {
	r.nextID++
	return r.nextID
}

func encodePacket(id, typ int32, body string) []byte {
	payload := make([]byte, 8+len(body)+2)
	binary.LittleEndian.PutUint32(payload[0:4], uint32(id))
	binary.LittleEndian.PutUint32(payload[4:8], uint32(typ))
	copy(payload[8:], body)

	pkt := make([]byte, 4+len(payload))
	binary.LittleEndian.PutUint32(pkt[0:4], uint32(len(payload)))
	copy(pkt[4:], payload)
	return pkt
}

func (r *RCONClient) readPacket() (id, typ int32, body string, err error) {
	var sizeBuf [4]byte
	if _, err = io.ReadFull(r.conn, sizeBuf[:]); err != nil {
		return 0, 0, "", err
	}
	size := int32(binary.LittleEndian.Uint32(sizeBuf[:]))
	if size < 8 {
		return 0, 0, "", fmt.Errorf("short RCON packet (%d bytes)", size)
	}
	if size > maxRCONPacket {
		return 0, 0, "", fmt.Errorf("RCON packet too large (%d bytes)", size)
	}
	payload := make([]byte, size)
	if _, err = io.ReadFull(r.conn, payload); err != nil {
		return 0, 0, "", err
	}
	id = int32(binary.LittleEndian.Uint32(payload[0:4]))
	typ = int32(binary.LittleEndian.Uint32(payload[4:8]))
	body = strings.TrimRight(string(payload[8:]), "\x00")
	return id, typ, body, nil
}

func (r *RCONClient) writePacket(id, typ int32, body string) error {
	r.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, err := r.conn.Write(encodePacket(id, typ, body))
	r.conn.SetWriteDeadline(time.Time{})
	return err
}

func (r *RCONClient) Connect() error {
	addr := net.JoinHostPort(r.cfg.BindIP, fmt.Sprintf("%d", r.cfg.RCONPort))

	dialer := net.Dialer{Timeout: 10 * time.Second}
	conn, err := dialer.Dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("connecting to RCON %s: %w", addr, err)
	}
	r.conn = conn

	if r.cfg.RCONPassword != "" {
		if err := r.authenticate(r.cfg.RCONPassword); err != nil {
			conn.Close()
			r.conn = nil
			return err
		}
	}
	return nil
}

// authenticate sends SERVERDATA_AUTH and waits for the auth result, skipping
// the empty acknowledgement packet the server sends first.
func (r *RCONClient) authenticate(password string) error {
	id := r.newID()
	if err := r.writePacket(id, rconTypeAuth, password); err != nil {
		return fmt.Errorf("sending RCON password: %w", err)
	}

	r.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	defer r.conn.SetReadDeadline(time.Time{})

	for {
		pktID, pktType, _, err := r.readPacket()
		if err != nil {
			return fmt.Errorf("reading RCON auth response: %w", err)
		}
		if pktType != rconTypeExec {
			continue
		}
		if pktID == -1 {
			return fmt.Errorf("RCON authentication failed")
		}
		return nil
	}
}

func (r *RCONClient) SendCommand(cmd string) (string, error) {
	if r.conn == nil {
		return "", fmt.Errorf("not connected to RCON")
	}
	// Always clear the deadline again: a stale one would break any reuse of
	// the connection (clients are single-use today, but stay safe).
	defer r.conn.SetReadDeadline(time.Time{})

	id := r.newID()
	if err := r.writePacket(id, rconTypeExec, cmd); err != nil {
		return "", fmt.Errorf("sending command: %w", err)
	}

	// PZ answers with one value packet per command and no terminator, so the
	// response ends when the read times out or the connection closes (quit).
	// The deadline shrinks to a short grace period after the first packet so
	// single-packet responses return promptly instead of eating the full
	// timeout (the Docker healthcheck allows only 10s).
	first := true
	var result strings.Builder
	for {
		r.conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		if !first {
			r.conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		}
		_, pktType, body, err := r.readPacket()
		first = false
		if err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				break // no more packets: response complete
			}
			if errors.Is(err, io.EOF) {
				break // server closed the connection (e.g. after quit)
			}
			if result.Len() > 0 {
				break
			}
			return "", fmt.Errorf("reading RCON response: %w", err)
		}
		if body == "" && pktType == rconTypeResponse {
			break
		}
		result.WriteString(body)
	}
	return strings.TrimSpace(result.String()), nil
}

func (r *RCONClient) Ping() error {
	response, err := r.SendCommand("hello")
	if err != nil {
		return err
	}
	fmt.Printf("RCON ping response: %s\n", response)
	return nil
}

// Broadcast sends a chat message visible to all players on the server.
func (r *RCONClient) Broadcast(message string) error {
	if r.conn == nil {
		return fmt.Errorf("not connected to RCON")
	}
	if _, err := r.SendCommand(fmt.Sprintf("servermsg %q", message)); err != nil {
		return fmt.Errorf("broadcasting server message: %w", err)
	}
	return nil
}

// PlayerCount returns the number of players currently online, parsed from the
// RCON "players" response. Lines that do not look like player names (headers,
// separators, "no players" notices) are ignored, so the count can only
// overestimate on exotic responses - never underestimate.
func (r *RCONClient) PlayerCount() (int, error) {
	if r.conn == nil {
		return 0, fmt.Errorf("not connected to RCON")
	}
	response, err := r.SendCommand("players")
	if err != nil {
		return 0, fmt.Errorf("querying players: %w", err)
	}
	return ParsePlayerCount(response), nil
}

// ParsePlayerCount turns the output of the RCON "players" command into a
// player count. Empty output and "no players" style responses count as zero.
func ParsePlayerCount(response string) int {
	count := 0
	for _, line := range strings.Split(response, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		if strings.Contains(lower, "no player") || strings.Contains(lower, "nobody") {
			return 0
		}
		if strings.HasPrefix(lower, "player") || strings.HasPrefix(lower, "-") ||
			strings.HasPrefix(lower, "=") || strings.HasPrefix(lower, "name") {
			continue
		}
		count++
	}
	return count
}

func (r *RCONClient) Close() {
	if r.conn != nil {
		r.conn.Close()
	}
}
