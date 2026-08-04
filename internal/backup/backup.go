package backup

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/faudil/project-zomboid-server-docker/internal/config"
	"github.com/faudil/project-zomboid-server-docker/internal/server"
)

type Manager struct {
	cfg *config.ServerConfig
	mu  sync.Mutex
}

func NewManager(cfg *config.ServerConfig) *Manager {
	return &Manager{cfg: cfg}
}

// Run creates a backup of the current save. A mutex ensures scheduled backups
// and the final shutdown backup never run concurrently.
func (m *Manager) Run() {
	if !m.cfg.BackupEnabled {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if err := os.MkdirAll(m.cfg.BackupPath, 0755); err != nil {
		fmt.Printf("creating backup directory: %v\n", err)
		return
	}

	// Create first, then rotate so the on-disk count never exceeds the max.
	if err := m.createBackup(); err != nil {
		fmt.Printf("backup failed: %v\n", err)
		return
	}
	m.rotate()
}

func (m *Manager) createBackup() error {
	// Nanosecond precision guarantees unique names even when backups are
	// triggered concurrently (e.g. scheduled backup during shutdown).
	timestamp := time.Now().Format("2006-01-02_15-04-05.000000000")
	filename := fmt.Sprintf("%s_backup_%s.tar.gz", m.cfg.ServerName, timestamp)
	fullPath := filepath.Join(m.cfg.BackupPath, filename)

	savePath := m.cfg.SavePath()
	if _, err := os.Stat(savePath); os.IsNotExist(err) {
		return fmt.Errorf("save directory does not exist: %s", savePath)
	}

	f, err := os.Create(fullPath)
	if err != nil {
		return fmt.Errorf("creating backup file: %w", err)
	}
	defer f.Close()

	gzw := gzip.NewWriter(f)
	defer gzw.Close()

	tw := tar.NewWriter(gzw)
	defer tw.Close()

	return filepath.Walk(savePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(filepath.Dir(savePath), path)
		if err != nil {
			return err
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = relPath

		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		if !info.Mode().IsRegular() {
			return nil
		}

		src, err := os.Open(path)
		if err != nil {
			return err
		}
		defer src.Close()

		_, err = io.Copy(tw, src)
		return err
	})
}

func (m *Manager) rotate() {
	prefix := fmt.Sprintf("%s_backup_", m.cfg.ServerName)
	suffix := ".tar.gz"

	entries, err := os.ReadDir(m.cfg.BackupPath)
	if err != nil {
		return
	}

	var backups []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), prefix) && strings.HasSuffix(e.Name(), suffix) {
			backups = append(backups, e.Name())
		}
	}

	sort.Strings(backups)

	for len(backups) > m.cfg.BackupMaxCount {
		oldest := filepath.Join(m.cfg.BackupPath, backups[0])
		fmt.Printf("Rotating old backup: %s\n", oldest)
		os.Remove(oldest)
		backups = backups[1:]
	}
}

func (m *Manager) Scheduler() {
	if !m.cfg.BackupEnabled {
		return
	}

	go func() {
		ticker := time.NewTicker(time.Duration(m.cfg.BackupInterval) * time.Minute)
		defer ticker.Stop()

		for range ticker.C {
			fmt.Println("Starting scheduled backup...")
			m.saveWorld()
			m.Run()
			fmt.Println("Scheduled backup complete")
		}
	}()
}

// saveWorld requests the server to flush its state to disk via RCON so the
// backup captures a consistent snapshot.
func (m *Manager) saveWorld() {
	client := server.NewRCONClient(m.cfg)
	if err := client.Connect(); err != nil {
		fmt.Printf("RCON connection failed before backup: %v\n", err)
		return
	}
	defer client.Close()

	if _, err := client.SendCommand("save"); err != nil {
		fmt.Printf("Save command failed before backup: %v\n", err)
		return
	}
	fmt.Println("World saved before backup")
}
