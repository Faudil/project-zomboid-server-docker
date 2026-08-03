package config

import (
	"os"
	"strings"
	"testing"
)

func TestWriteSandboxVarsDeterministic(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DataDir = t.TempDir()
	cfg.ServerName = "testworld"

	if err := cfg.WriteSandboxVars(); err != nil {
		t.Fatalf("WriteSandboxVars: %v", err)
	}
	first, _ := os.ReadFile(cfg.SandboxVarsPath())

	for i := 0; i < 5; i++ {
		if err := cfg.WriteSandboxVars(); err != nil {
			t.Fatalf("WriteSandboxVars: %v", err)
		}
		again, _ := os.ReadFile(cfg.SandboxVarsPath())
		if string(first) != string(again) {
			t.Fatalf("sandbox output not deterministic:\n%s\n---\n%s", first, again)
		}
	}
}

func TestWriteSandboxVarsContent(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DataDir = t.TempDir()
	cfg.ServerName = "testworld"
	cfg.SandboxVars = map[string]string{"Zombies": "5", "Helicopter": "false"}

	if err := cfg.WriteSandboxVars(); err != nil {
		t.Fatalf("WriteSandboxVars: %v", err)
	}

	data, _ := os.ReadFile(cfg.SandboxVarsPath())
	content := string(data)

	if !strings.Contains(content, "SandboxVars = {") || !strings.HasSuffix(strings.TrimSpace(content), "}") {
		t.Errorf("malformed sandbox file:\n%s", content)
	}
	if !strings.Contains(content, "Zombies = 5") {
		t.Errorf("override missing:\n%s", content)
	}
	if !strings.Contains(content, "Helicopter = false") {
		t.Errorf("override missing:\n%s", content)
	}
	if !strings.Contains(content, "ZombieConfig = {") {
		t.Errorf("default ZombieConfig missing:\n%s", content)
	}
	if strings.Contains(content, ",,") {
		t.Errorf("double comma in sandbox output (invalid Lua):\n%s", content)
	}
}
