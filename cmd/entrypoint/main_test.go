package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/faudil/project-zomboid-server-docker/internal/config"
)

func setEnv(t *testing.T, vals map[string]string) {
	t.Helper()
	for k, v := range vals {
		t.Setenv(k, v)
	}
}

func TestRunRejectsInvalidServerName(t *testing.T) {
	dir := t.TempDir()
	setEnv(t, map[string]string{
		"SERVER_NAME": "../../evil",
		"DATA_DIR":    dir,
		"SERVER_DIR":  t.TempDir(),
	})
	err := run()
	if err == nil || !strings.Contains(err.Error(), "configuration validation failed") {
		t.Fatalf("run() = %v, want configuration validation error", err)
	}
}

func TestRunRejectsInvalidSandboxMode(t *testing.T) {
	setEnv(t, map[string]string{
		"SERVER_NAME":  "testworld",
		"SANDBOX_MODE": "banana",
		"DATA_DIR":     t.TempDir(),
		"SERVER_DIR":   t.TempDir(),
	})
	err := run()
	if err == nil || !strings.Contains(err.Error(), "configuration validation failed") {
		t.Fatalf("run() = %v, want configuration validation error", err)
	}
}

func TestRunRejectsUnwritableVolumes(t *testing.T) {
	dir := t.TempDir()
	// A regular file where a directory is expected makes MkdirAll fail,
	// simulating an unwritable/absent mount regardless of test privileges.
	blocker := filepath.Join(dir, "blocker")
	f, err := os.Create(blocker)
	if err != nil {
		t.Fatalf("create blocker: %v", err)
	}
	f.Close()

	setEnv(t, map[string]string{
		"SERVER_NAME": "testworld",
		"DATA_DIR":    filepath.Join(blocker, "data"),
		"SERVER_DIR":  t.TempDir(),
		"BACKUP_PATH": filepath.Join(blocker, "data", "backups"),
	})
	err = run()
	if err == nil || !strings.Contains(err.Error(), "writable") {
		t.Fatalf("run() = %v, want writability error", err)
	}
}

func TestRunReturnsInstallError(t *testing.T) {
	dir := t.TempDir()
	setEnv(t, map[string]string{
		"SERVER_NAME": "testworld",
		"DATA_DIR":    dir,
		"SERVER_DIR":  t.TempDir(),
		"BACKUP_PATH": filepath.Join(dir, "backups"),
	})

	orig := installOrUpdate
	installOrUpdate = func(*config.ServerConfig) error { return fmt.Errorf("install exploded") }
	defer func() { installOrUpdate = orig }()

	err := run()
	if err == nil || !strings.Contains(err.Error(), "install exploded") {
		t.Fatalf("run() = %v, want install error", err)
	}
}

func TestRunReturnsIniWriteError(t *testing.T) {
	dir := t.TempDir()
	// A file where the Server/ config directory should be: MkdirAll fails.
	blocker := filepath.Join(dir, "Server")
	f, err := os.Create(blocker)
	if err != nil {
		t.Fatalf("create blocker: %v", err)
	}
	f.Close()

	setEnv(t, map[string]string{
		"SERVER_NAME": "testworld",
		"DATA_DIR":    dir,
		"SERVER_DIR":  t.TempDir(),
		"BACKUP_PATH": filepath.Join(dir, "backups"),
	})

	origInstall := installOrUpdate
	installOrUpdate = func(*config.ServerConfig) error { return nil }
	defer func() { installOrUpdate = origInstall }()
	origResolve := resolveModWorkshop
	resolveModWorkshop = func(*config.ServerConfig) []string { return nil }
	defer func() { resolveModWorkshop = origResolve }()

	err = run()
	if err == nil || !strings.Contains(err.Error(), "writing server.ini") {
		t.Fatalf("run() = %v, want ini write error", err)
	}
}
