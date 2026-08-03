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
	if !strings.Contains(content, "ZombieLore = {") {
		t.Errorf("default ZombieLore missing:\n%s", content)
	}
	if strings.Contains(content, ",,") {
		t.Errorf("double comma in sandbox output (invalid Lua):\n%s", content)
	}
	// Build 42 sandbox keys: the old names are silently dropped by the game.
	for _, legacy := range []string{"FoodLoot =", "WeaponLoot =", "OtherLoot =", "SurvivorHouses =", "MonthsSinceApo =", "DarknessNight =", "Choppers ="} {
		if strings.Contains(content, legacy) {
			t.Errorf("legacy b41 key %q present in b42 sandbox output:\n%s", legacy, content)
		}
	}
	for _, want := range []string{"FoodLootNew = 2", "WeaponLootNew = 2", "OtherLootNew = 2", "SurvivorHouseChance = 3", "TimeSinceApo = 1", "NightDarkness = 3", "StartYear = 1", "WaterShut = 2", "ElecShut = 2"} {
		if !strings.Contains(content, want) {
			t.Errorf("missing %q in sandbox output:\n%s", want, content)
		}
	}
}

func TestWriteSandboxVarsLegacyOverrideAliased(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DataDir = t.TempDir()
	cfg.ServerName = "testworld"
	cfg.SandboxVars = map[string]string{"FoodLoot": "5"}

	if err := cfg.WriteSandboxVars(); err != nil {
		t.Fatalf("WriteSandboxVars: %v", err)
	}
	data, _ := os.ReadFile(cfg.SandboxVarsPath())
	content := string(data)
	if !strings.Contains(content, "FoodLootNew = 5") {
		t.Errorf("legacy FoodLoot override not mapped to FoodLootNew:\n%s", content)
	}
	if strings.Contains(content, "FoodLoot =") {
		t.Errorf("legacy FoodLoot key written verbatim:\n%s", content)
	}
}
