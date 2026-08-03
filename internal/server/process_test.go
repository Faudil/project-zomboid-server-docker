package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/faudil/project-zomboid-server-docker/internal/config"
)

func TestJavaOptions(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.MaxRam = "8192m"
	cfg.MinRam = "2048m"
	cfg.GCConfig = "G1"
	cfg.JvmExtraArgs = "-Dfoo=bar -XX:MaxGCPauseMillis=200"

	m := NewManager(cfg)
	opts := m.javaOptions()
	for _, want := range []string{"-Xms2048m", "-Xmx8192m", "-XX:+UseG1GC", "-Dfoo=bar"} {
		if !strings.Contains(opts, want) {
			t.Errorf("javaOptions = %q, missing %q", opts, want)
		}
	}
}

func TestJavaOptionsEmptyExtra(t *testing.T) {
	cfg := config.DefaultConfig()
	m := NewManager(cfg)
	opts := m.javaOptions()
	if strings.Contains(opts, "UseZGCGC") {
		t.Errorf("GC flag mangled: %q", opts)
	}
	if !strings.Contains(opts, "-XX:+UseZGC") {
		t.Errorf("default GC not applied: %q", opts)
	}
}

func writeStartScript(t *testing.T, dir string) {
	t.Helper()
	script := filepath.Join(dir, "start-server.sh")
	// Sleep in the background so the wrapper stays alive like the real
	// server process and the java child is part of the process group.
	content := "#!/bin/bash\nsleep 100 &\nwait\n"
	if err := os.WriteFile(script, []byte(content), 0755); err != nil {
		t.Fatal(err)
	}
}

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	dir := t.TempDir()
	writeStartScript(t, dir)

	cfg := config.DefaultConfig()
	cfg.ServerDir = dir
	cfg.DataDir = dir
	cfg.AdminPassword = "admin-pass"
	cfg.RCONPassword = "rcon-pass"
	cfg.TZ = "UTC"
	cfg.RCONPort = 1 // RCON connect will fail fast; Stop falls back to SIGTERM

	return NewManager(cfg)
}

func TestStopTerminatesProcessGroup(t *testing.T) {
	m := newTestManager(t)
	if err := m.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if m.PID() == 0 {
		t.Fatal("PID is 0")
	}

	// RCON is unreachable, so Stop must fall back to killing the process group.
	if err := m.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// Wait must return promptly once the process is gone.
	done := make(chan error, 1)
	go func() { done <- m.Wait() }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Wait did not return after Stop")
	}
}

func TestStopBeforeStart(t *testing.T) {
	m := newTestManager(t)
	if err := m.Stop(); err != nil {
		t.Fatalf("Stop before Start should be a no-op, got %v", err)
	}
}
