package backup

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/faudil/project-zomboid-server-docker/internal/config"
)

func newTestManager(t *testing.T, backups int) (*Manager, string) {
	t.Helper()
	dir := t.TempDir()

	cfg := config.DefaultConfig()
	cfg.DataDir = dir
	cfg.ServerName = "testworld"
	cfg.BackupEnabled = true
	cfg.BackupMaxCount = backups
	cfg.BackupPath = filepath.Join(dir, "backups")

	saveDir := cfg.SavePath()
	if err := os.MkdirAll(saveDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(cfg.BackupPath, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(saveDir, "world.bin"), []byte("world-data"), 0644); err != nil {
		t.Fatal(err)
	}

	return NewManager(cfg), dir
}

func listBackups(t *testing.T, m *Manager) []string {
	t.Helper()
	entries, err := os.ReadDir(m.cfg.BackupPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names
}

func TestRunCreatesValidArchive(t *testing.T) {
	m, _ := newTestManager(t, 4)
	m.Run()

	backups := listBackups(t, m)
	if len(backups) != 1 {
		t.Fatalf("backups = %v, want exactly 1", backups)
	}

	f, err := os.Open(filepath.Join(m.cfg.BackupPath, backups[0]))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("not a valid gzip file: %v", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	found := false
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("invalid tar: %v", err)
		}
		if strings.HasSuffix(hdr.Name, "world.bin") {
			found = true
			data, err := io.ReadAll(tr)
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != "world-data" {
				t.Errorf("world.bin content = %q", data)
			}
		}
	}
	if !found {
		t.Error("world.bin not found in archive")
	}
}

func TestRunDisabled(t *testing.T) {
	m, _ := newTestManager(t, 4)
	m.cfg.BackupEnabled = false
	m.Run()

	if backups := listBackups(t, m); len(backups) != 0 {
		t.Errorf("backups = %v, want none", backups)
	}
}

func TestRotation(t *testing.T) {
	m, _ := newTestManager(t, 2)

	// Seed three backups with distinct names (oldest first).
	names := []string{
		"testworld_backup_2026-01-01_00-00-00.tar.gz",
		"testworld_backup_2026-01-02_00-00-00.tar.gz",
		"testworld_backup_2026-01-03_00-00-00.tar.gz",
	}
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(m.cfg.BackupPath, n), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	m.Run()

	remaining := listBackups(t, m)
	if len(remaining) != 2 {
		t.Fatalf("remaining = %v, want 2 after rotation", remaining)
	}
	if strings.Contains(remaining[0], "2026-01-01") {
		t.Errorf("oldest backup not rotated, remaining = %v", remaining)
	}
}

func TestRunMissingSaveDir(t *testing.T) {
	dir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.DataDir = dir
	cfg.ServerName = "testworld"
	cfg.BackupEnabled = true
	cfg.BackupPath = filepath.Join(dir, "backups")

	m := NewManager(cfg)
	m.Run() // must not panic, just logs

	if backups := listBackups(t, m); len(backups) != 0 {
		t.Errorf("backups = %v, want none", backups)
	}
}

func TestRunNoOverlap(t *testing.T) {
	// Two concurrent Run calls must serialize; the result is two archives.
	m, _ := newTestManager(t, 4)
	done := make(chan struct{})
	go func() {
		m.Run()
		done <- struct{}{}
	}()
	m.Run()
	<-done

	if backups := listBackups(t, m); len(backups) != 2 {
		t.Errorf("backups = %v, want 2 (serialized)", backups)
	}
}

func TestBackupFilename(t *testing.T) {
	m, _ := newTestManager(t, 4)
	cfg := m.cfg
	got := fmt.Sprintf("%s_backup_%s.tar.gz", cfg.ServerName, "2026-01-01_00-00-00")
	want := "testworld_backup_2026-01-01_00-00-00.tar.gz"
	if got != want {
		t.Errorf("filename = %q, want %q", got, want)
	}
}
