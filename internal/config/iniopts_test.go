package config

import (
	"os"
	"strings"
	"testing"
)

func TestLoadIniEnv(t *testing.T) {
	t.Setenv("INI_SleepAllowed", "true")
	t.Setenv("INI_FastForwardMultiplier", "40.0")
	t.Setenv("UNRELATED", "ignored")

	cfg := DefaultConfig()
	if cfg.IniOptions["SleepAllowed"] != "true" {
		t.Errorf("IniOptions[SleepAllowed] = %q, want true", cfg.IniOptions["SleepAllowed"])
	}
	if cfg.IniOptions["FastForwardMultiplier"] != "40.0" {
		t.Errorf("IniOptions[FastForwardMultiplier] = %q, want 40.0", cfg.IniOptions["FastForwardMultiplier"])
	}
	if _, ok := cfg.IniOptions["UNRELATED"]; ok {
		t.Error("non-INI_ env leaked into IniOptions")
	}
}

func TestWriteIniWithIniOptions(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DataDir = t.TempDir()
	cfg.IniOptions = map[string]string{
		"SleepAllowed":                "true",
		"SleepNeeded":                 "false",
		"Faction":                     "false",
		"SafehousePreventLootRespawn": "true",
	}

	if err := cfg.WriteIni(); err != nil {
		t.Fatalf("WriteIni: %v", err)
	}

	data, err := os.ReadFile(cfg.ServerIniPath())
	if err != nil {
		t.Fatalf("reading ini: %v", err)
	}
	content := string(data)

	for _, want := range []string{
		"SleepAllowed=true",
		"SleepNeeded=false",
		"Faction=false",
		"SafehousePreventLootRespawn=true",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("ini missing %q:\n%s", want, content)
		}
	}
}

func TestWriteIniWithIniOptionsDeterministic(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DataDir = t.TempDir()
	cfg.IniOptions = map[string]string{
		"SleepNeeded":  "true",
		"SleepAllowed": "true",
		"Faction":      "false",
	}

	if err := cfg.WriteIni(); err != nil {
		t.Fatalf("WriteIni: %v", err)
	}
	first, _ := os.ReadFile(cfg.ServerIniPath())

	if err := cfg.WriteIni(); err != nil {
		t.Fatalf("WriteIni: %v", err)
	}
	again, _ := os.ReadFile(cfg.ServerIniPath())

	if string(first) != string(again) {
		t.Fatalf("ini output not deterministic:\n%s\n---\n%s", first, again)
	}
}

func TestWriteIniIniOptionsInvalidKeyIgnored(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DataDir = t.TempDir()
	cfg.IniOptions = map[string]string{
		"[GameOptions]": "true",
		"Bad Key":       "x",
		"SleepAllowed":  "true",
	}

	if err := cfg.WriteIni(); err != nil {
		t.Fatalf("WriteIni: %v", err)
	}

	data, _ := os.ReadFile(cfg.ServerIniPath())
	content := string(data)
	if !strings.Contains(content, "SleepAllowed=true") {
		t.Errorf("valid INI_ key missing:\n%s", content)
	}
	for _, line := range strings.Split(content, "\n") {
		if strings.Contains(line, "[GameOptions]") || strings.HasPrefix(line, "Bad Key") {
			t.Errorf("invalid INI_ key leaked into ini:\n%s", content)
		}
	}
}

func TestWriteIniIniOptionsNewlineStripped(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DataDir = t.TempDir()
	// A newline in the value must not inject a second directive.
	cfg.IniOptions = map[string]string{
		"SleepAllowed": "true\nMaxPlayers=1",
	}

	if err := cfg.WriteIni(); err != nil {
		t.Fatalf("WriteIni: %v", err)
	}

	data, _ := os.ReadFile(cfg.ServerIniPath())
	content := string(data)
	for _, line := range strings.Split(content, "\n") {
		if line == "MaxPlayers=1" {
			t.Errorf("newline injection leaked into ini:\n%s", content)
		}
	}
}

func TestWriteIniIniOptionsFixedKeyIgnored(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DataDir = t.TempDir()
	// MaxPlayers is already written by the image; the INI_ override must not
	// produce a duplicate line.
	cfg.IniOptions = map[string]string{
		"MaxPlayers": "1",
	}

	if err := cfg.WriteIni(); err != nil {
		t.Fatalf("WriteIni: %v", err)
	}

	data, _ := os.ReadFile(cfg.ServerIniPath())
	content := string(data)
	if strings.Count(content, "MaxPlayers=") != 1 {
		t.Errorf("fixed key duplicated by INI_ override:\n%s", content)
	}
}
