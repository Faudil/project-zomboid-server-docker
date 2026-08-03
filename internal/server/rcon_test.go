package server

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/faudil/project-zomboid-server-docker/internal/config"
)

// fakeRCONServer mimics the PZ RCON protocol: standard Source RCON packets.
// On auth it sends an empty acknowledgement followed by the auth result; on
// commands it replies with a single value packet, no terminator.
func fakeRCONServer(t *testing.T, password string) (addr string, stop func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		readPacket := func() (id, typ int32, body string, err error) {
			var sizeBuf [4]byte
			if _, err = io.ReadFull(conn, sizeBuf[:]); err != nil {
				return 0, 0, "", err
			}
			size := int32(binary.LittleEndian.Uint32(sizeBuf[:]))
			payload := make([]byte, size)
			if _, err = io.ReadFull(conn, payload); err != nil {
				return 0, 0, "", err
			}
			id = int32(binary.LittleEndian.Uint32(payload[0:4]))
			typ = int32(binary.LittleEndian.Uint32(payload[4:8]))
			body = strings.TrimRight(string(payload[8:]), "\x00")
			return id, typ, body, nil
		}
		writePacket := func(id, typ int32, body string) error {
			payload := make([]byte, 8+len(body)+2)
			binary.LittleEndian.PutUint32(payload[0:4], uint32(id))
			binary.LittleEndian.PutUint32(payload[4:8], uint32(typ))
			copy(payload[8:], body)
			pkt := make([]byte, 4+len(payload))
			binary.LittleEndian.PutUint32(pkt[0:4], uint32(len(payload)))
			copy(pkt[4:], payload)
			_, err := conn.Write(pkt)
			return err
		}

		// Auth: empty ack, then the result.
		authID, authType, pw, err := readPacket()
		if err != nil || authType != rconTypeAuth {
			return
		}
		writePacket(authID, rconTypeResponse, "")
		if pw != password {
			writePacket(-1, rconTypeExec, "")
			return
		}
		writePacket(authID, rconTypeExec, "")

		for {
			cmdID, cmdType, cmd, err := readPacket()
			if err != nil {
				return
			}
			if cmdType == rconTypeAuth {
				writePacket(cmdID, rconTypeResponse, "")
				if cmd != password {
					writePacket(-1, rconTypeExec, "")
					return
				}
				writePacket(cmdID, rconTypeExec, "")
				continue
			}
			if cmdType != rconTypeExec {
				continue
			}
			switch strings.TrimSpace(cmd) {
			case "quit":
				return
			case "hello":
				writePacket(cmdID, rconTypeResponse, "hello")
			case "save":
				writePacket(cmdID, rconTypeResponse, "World saved")
			default:
				writePacket(cmdID, rconTypeResponse, cmd)
			}
		}
	}()

	return ln.Addr().String(), func() { ln.Close() }
}

func newTestRCONClient(addr string) *RCONClient {
	host, portStr, _ := net.SplitHostPort(addr)
	var port int
	fmt.Sscanf(portStr, "%d", &port)

	cfg := config.DefaultConfig()
	cfg.BindIP = host
	cfg.RCONPort = port
	cfg.RCONPassword = "secret"
	return NewRCONClient(cfg)
}

func TestRCONConnectPingSend(t *testing.T) {
	addr, stop := fakeRCONServer(t, "secret")
	defer stop()

	client := newTestRCONClient(addr)
	if err := client.Connect(); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer client.Close()

	if err := client.Ping(); err != nil {
		t.Fatalf("Ping: %v", err)
	}

	resp, err := client.SendCommand("save")
	if err != nil {
		t.Fatalf("SendCommand: %v", err)
	}
	if !strings.Contains(resp, "World saved") {
		t.Errorf("save response = %q, want World saved", resp)
	}
}

func TestRCONWrongPassword(t *testing.T) {
	addr, stop := fakeRCONServer(t, "secret")
	defer stop()

	client := newTestRCONClient(addr)
	client.cfg.RCONPassword = "wrong"
	if err := client.Connect(); err == nil {
		t.Fatal("Connect succeeded with wrong password")
	}
}

func TestRCONNotConnected(t *testing.T) {
	client := NewRCONClient(config.DefaultConfig())
	if _, err := client.SendCommand("hello"); err == nil {
		t.Fatal("SendCommand without connection should fail")
	}
}

func TestRCONConnectRefused(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close() // nothing listening anymore

	client := newTestRCONClient(addr)
	client.cfg.RCONPassword = "secret"
	start := time.Now()
	if err := client.Connect(); err == nil {
		t.Fatal("Connect to closed port should fail")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("Connect took %v, dialer timeout too long", elapsed)
	}
}
