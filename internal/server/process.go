package server

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/faudil/project-zomboid-server-docker/internal/config"
)

type Manager struct {
	cfg     *config.ServerConfig
	cmd     *exec.Cmd
	exited  chan struct{} // closed once the server process has exited
	once    sync.Once
	mu      sync.Mutex
	exitErr error
}

func NewManager(cfg *config.ServerConfig) *Manager {
	return &Manager{
		cfg:    cfg,
		exited: make(chan struct{}),
	}
}

func (m *Manager) Start() error {
	startScript := m.cfg.ServerDir + "/start-server.sh"
	if _, err := os.Stat(startScript); err != nil {
		return fmt.Errorf("start-server.sh not found at %s - run steamcmd update first", startScript)
	}

	args := []string{startScript, "-servername", m.cfg.ServerName}
	// The admin password must be passed on the command line: the b42
	// start-server.sh forwards "$@" to ProjectZomboid64 and ignores the
	// ADMIN_PASSWORD environment variable. Without it the server prompts
	// "Enter new administrator password:" on stdin on first run and crashes
	// (Scanner: No line found) because stdin is closed in Docker.
	if m.cfg.AdminPassword != "" {
		args = append(args, "-adminpassword", m.cfg.AdminPassword)
	}
	if !m.cfg.UseSteam {
		args = append(args, "-nosteam")
	}

	m.cmd = exec.Command("bash", args...)
	m.cmd.Stdout = os.Stdout
	m.cmd.Stderr = os.Stderr

	env := os.Environ()
	env = append(env, "HOME=/home/steam")
	env = append(env, fmt.Sprintf("ADMIN_PASSWORD=%s", m.cfg.AdminPassword))
	env = append(env, fmt.Sprintf("TZ=%s", m.cfg.TZ))
	env = append(env, "_JAVA_OPTIONS="+m.javaOptions())
	m.cmd.Env = env

	// Start the server in its own process group so we can signal the whole
	// group (bash wrapper + java child) on shutdown.
	m.cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	if err := m.cmd.Start(); err != nil {
		return fmt.Errorf("starting server: %w", err)
	}

	fmt.Printf("Server started with PID: %d\n", m.cmd.Process.Pid)

	go func() {
		err := m.cmd.Wait()
		m.once.Do(func() {
			m.mu.Lock()
			m.exitErr = err
			m.mu.Unlock()
			close(m.exited)
		})
	}()

	return nil
}

// javaOptions builds JVM heap/GC settings passed to the JVM via _JAVA_OPTIONS.
// The dedicated server's start-server.sh does not expose these, but every JVM
// launcher honours _JAVA_OPTIONS.
func (m *Manager) javaOptions() string {
	opts := []string{
		fmt.Sprintf("-Xms%s", m.cfg.MinRam),
		fmt.Sprintf("-Xmx%s", m.cfg.MaxRam),
	}
	if m.cfg.GCConfig != "" {
		if gc := config.NormalizeGC(m.cfg.GCConfig); gc != "" {
			opts = append(opts, fmt.Sprintf("-XX:+Use%s", gc))
		}
	}
	if m.cfg.JvmExtraArgs != "" {
		opts = append(opts, strings.Fields(m.cfg.JvmExtraArgs)...)
	}
	return strings.Join(opts, " ")
}

// Wait blocks until the server process exits and returns its exit error.
// Signal handling is owned by the caller (cmd/entrypoint), which triggers
// Stop() on SIGTERM/SIGINT. Multiple concurrent waiters are safe.
func (m *Manager) Wait() error {
	<-m.exited
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.exitErr
}

func (m *Manager) Stop() error {
	if m.cmd == nil || m.cmd.Process == nil {
		return nil
	}

	client := NewRCONClient(m.cfg)
	if err := client.Connect(); err != nil {
		fmt.Printf("RCON connection failed during shutdown: %v, forcing quit\n", err)
		return m.forceKill()
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
		return m.forceKill()
	}

	select {
	case <-m.exited:
		return nil
	case <-time.After(30 * time.Second):
		fmt.Println("Server did not exit after quit command, forcing termination")
		return m.forceKill()
	}
}

// forceKill signals the server process group, escalating to SIGKILL if it does
// not exit. The whole group is targeted because bash spawns a java child.
func (m *Manager) forceKill() error {
	if m.cmd == nil || m.cmd.Process == nil {
		return nil
	}

	pgid := m.cmd.Process.Pid
	_ = syscall.Kill(-pgid, syscall.SIGTERM)

	select {
	case <-m.exited:
		return nil
	case <-time.After(30 * time.Second):
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
		return fmt.Errorf("server process group %d did not exit after SIGTERM", pgid)
	}
}

func (m *Manager) PID() int {
	if m.cmd != nil && m.cmd.Process != nil {
		return m.cmd.Process.Pid
	}
	return 0
}
