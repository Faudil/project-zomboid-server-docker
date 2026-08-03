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

	id := r.newID()
	if err := r.writePacket(id, rconTypeExec, cmd); err != nil {
		return "", fmt.Errorf("sending command: %w", err)
	}

	// PZ answers with one value packet per command and no terminator, so the
	// response ends when the read times out or the connection closes (quit).
	r.conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	defer r.conn.SetReadDeadline(time.Time{})

	var result strings.Builder
	for {
		_, pktType, body, err := r.readPacket()
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

func (r *RCONClient) Close() {
	if r.conn != nil {
		r.conn.Close()
	}
}
