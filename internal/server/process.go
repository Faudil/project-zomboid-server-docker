package server

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/faudil/project-zomboid-server-docker/internal/config"
)

type Manager struct {
	cfg    *config.ServerConfig
	cmd    *exec.Cmd
	stopCh chan struct{}
	doneCh chan error
}

func NewManager(cfg *config.ServerConfig) *Manager {
	return &Manager{
		cfg:    cfg,
		stopCh: make(chan struct{}),
		doneCh: make(chan error),
	}
}

func (m *Manager) Start() error {
	startScript := m.cfg.ServerDir + "/start-server.sh"
	if _, err := os.Stat(startScript); err != nil {
		return fmt.Errorf("start-server.sh not found at %s - run steamcmd update first", startScript)
	}

	args := []string{startScript, "-servername", m.cfg.ServerName}
	if !m.cfg.UseSteam {
		args = append(args, "-nosteam")
	}

	m.cmd = exec.Command("bash", args...)
	m.cmd.Stdout = os.Stdout
	m.cmd.Stderr = os.Stderr

	env := os.Environ()
	env = append(env, fmt.Sprintf("HOME=/home/steam"))
	env = append(env, fmt.Sprintf("ADMIN_PASSWORD=%s", m.cfg.AdminPassword))
	env = append(env, fmt.Sprintf("TZ=%s", m.cfg.TZ))
	m.cmd.Env = env

	m.cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	if err := m.cmd.Start(); err != nil {
		return fmt.Errorf("starting server: %w", err)
	}

	fmt.Printf("Server started with PID: %d\n", m.cmd.Process.Pid)
	fmt.Printf("RCON Password: %s\n", m.cfg.RCONPassword)

	go func() {
		m.doneCh <- m.cmd.Wait()
	}()

	return nil
}

func (m *Manager) Wait() error {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	select {
	case err := <-m.doneCh:
		return err
	case sig := <-sigCh:
		fmt.Printf("Received signal: %v, shutting down gracefully...\n", sig)
		return m.Stop()
	}
}

func (m *Manager) Stop() error {
	if m.cmd == nil || m.cmd.Process == nil {
		return nil
	}

	client := NewRCONClient(m.cfg)
	if err := client.Connect(); err != nil {
		fmt.Printf("RCON connection failed during shutdown: %v, forcing quit\n", err)
		_ = m.cmd.Process.Signal(syscall.SIGTERM)
		return nil
	}
	defer client.Close()

	fmt.Println("Sending save command...")
	if _, err := client.SendCommand("save"); err != nil {
		fmt.Printf("Save command failed: %v\n", err)
	} else {
		fmt.Println("World saved")
	}

	fmt.Println("Sending quit command...")
	if _, err := client.SendCommand("quit"); err != nil {
		fmt.Printf("Quit command failed: %v, forcing termination\n", err)
		_ = m.cmd.Process.Signal(syscall.SIGTERM)
		return nil
	}

	select {
	case err := <-m.doneCh:
		return err
	case sig := <-m.doneCh:
		_ = sig
	}

	return nil
}

func (m *Manager) PID() int {
	if m.cmd != nil && m.cmd.Process != nil {
		return m.cmd.Process.Pid
	}
	return 0
}
