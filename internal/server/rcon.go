package server

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/faudil/project-zomboid-server-docker/internal/config"
)

type RCONClient struct {
	cfg  *config.ServerConfig
	conn net.Conn
}

func NewRCONClient(cfg *config.ServerConfig) *RCONClient {
	return &RCONClient{cfg: cfg}
}

func (r *RCONClient) Connect() error {
	addr := net.JoinHostPort(r.cfg.BindIP, fmt.Sprintf("%d", r.cfg.RCONPort))

	dialer := net.Dialer{Timeout: 10 * time.Second}
	conn, err := dialer.Dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("connecting to RCON %s: %w", addr, err)
	}

	if r.cfg.RCONPassword != "" {
		_, err = fmt.Fprintf(conn, "%s\n", r.cfg.RCONPassword)
		if err != nil {
			conn.Close()
			return fmt.Errorf("sending RCON password: %w", err)
		}
	}

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	reader := bufio.NewReader(conn)
	response, err := reader.ReadString('\n')
	if err != nil {
		conn.Close()
		return fmt.Errorf("reading RCON auth response: %w", err)
	}

	if strings.Contains(response, "Password incorrect") || strings.Contains(response, "Authentication failed") {
		conn.Close()
		return fmt.Errorf("RCON authentication failed")
	}

	conn.SetReadDeadline(time.Time{})
	r.conn = conn
	return nil
}

func (r *RCONClient) SendCommand(cmd string) (string, error) {
	if r.conn == nil {
		return "", fmt.Errorf("not connected to RCON")
	}

	r.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, err := fmt.Fprintf(r.conn, "%s\n", cmd)
	if err != nil {
		return "", fmt.Errorf("sending command: %w", err)
	}

	r.conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	reader := bufio.NewReader(r.conn)
	var response strings.Builder

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if response.Len() > 0 {
				break
			}
			return "", fmt.Errorf("reading RCON response: %w", err)
		}
		response.WriteString(line)
		// PZ terminates each command response with an "RCON: " prompt line.
		if strings.Contains(line, "RCON:") {
			break
		}
	}

	result := response.String()
	result = strings.TrimSpace(result)
	result = strings.TrimPrefix(result, "RCON: ")
	result = strings.TrimSuffix(result, "\nRCON: \n")
	return result, nil
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
