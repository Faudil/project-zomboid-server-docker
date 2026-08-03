package server

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/faudil/project-zomboid-server-docker/internal/config"
)

// fakeRCONServer mimics the PZ RCON protocol: a line-based password handshake
// followed by commands answered with output lines terminated by an "RCON: "
// prompt line. The connection stays open between commands.
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

		reader := bufio.NewReader(conn)
		auth, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		if strings.TrimSpace(auth) != password {
			fmt.Fprintf(conn, "Password incorrect\n")
			return
		}
		fmt.Fprintf(conn, "Password correct\n")

		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			cmd := strings.TrimSpace(line)
			switch cmd {
			case "quit":
				return
			case "hello":
				fmt.Fprintf(conn, "RCON: hello\n")
			case "save":
				fmt.Fprintf(conn, "World saved\nRCON: \n")
			default:
				fmt.Fprintf(conn, "RCON: %s\n", cmd)
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
