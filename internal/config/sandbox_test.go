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
	for _, want := range []string{"FoodLootNew = 0.8", "WeaponLootNew = 0.6", "OtherLootNew = 0.8", "SurvivorHouseChance = 3", "TimeSinceApo = 1", "NightDarkness = 3", "StartYear = 1", "WaterShut = 2", "ElecShut = 2", "DayLength = 4", "Alarm = 4", "LockedHouses = 6", "GeneratorSpawning = 4", "AnnotatedMapChance = 4", "SleepingEvent = 1", "ErosionSpeed = 4", "CompostTime = 2"} {
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

func writeSandboxForMode(t *testing.T, mode string) string {
	t.Helper()
	cfg := DefaultConfig()
	cfg.DataDir = t.TempDir()
	cfg.ServerName = "testworld"
	cfg.SandboxMode = mode
	if err := cfg.WriteSandboxVars(); err != nil {
		t.Fatalf("WriteSandboxVars(%s): %v", mode, err)
	}
	data, _ := os.ReadFile(cfg.SandboxVarsPath())
	return string(data)
}

func TestWriteSandboxVarsModes(t *testing.T) {
	apoc := writeSandboxForMode(t, "apocalypse")
	perf := writeSandboxForMode(t, "performance")
	maxMode := writeSandboxForMode(t, "max")

	cleanupKeys := map[string]string{
		"HoursForCorpseRemoval":    "48",
		"BloodSplatLifespanDays":   "7",
		"DaysForRottenFoodRemoval": "14",
		"MaximumRatIndex":          "0",
		"HoursForWorldItemRemoval": "12",
	}
	for k, v := range cleanupKeys {
		want := k + " = " + v
		if !strings.Contains(perf, want) || !strings.Contains(maxMode, want) {
			t.Errorf("performance/max mode missing %q:\nperf:\n%s\nmax:\n%s", want, perf, maxMode)
		}
		if strings.Contains(apoc, k+" = "+v) {
			t.Errorf("apocalypse mode must keep b42 defaults, found %s:\n%s", want, apoc)
		}
	}

	for _, want := range []string{"PopulationMultiplier = 0.35", "PopulationStartMultiplier = 0.5", "PopulationPeakMultiplier = 1.0", "RedistributeHours = 24.0", "FollowSoundDistance = 50", "RallyGroupSize = 10", "RallyGroupSizeVariance = 25"} {
		if !strings.Contains(maxMode, want) {
			t.Errorf("max mode missing %q:\n%s", want, maxMode)
		}
	}
	for _, want := range []string{"PopulationMultiplier = 0.65", "RallyGroupSize = 20"} {
		if !strings.Contains(perf, want) {
			t.Errorf("performance mode must keep Apocalypse population, missing %q:\n%s", want, perf)
		}
	}
	if strings.Contains(apoc, "PopulationMultiplier = 0.35") {
		t.Errorf("apocalypse mode must keep Apocalypse population:\n%s", apoc)
	}
}

func TestSandboxModeEnvOverrideWins(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DataDir = t.TempDir()
	cfg.ServerName = "testworld"
	cfg.SandboxMode = "performance"
	cfg.SandboxVars = map[string]string{"HoursForCorpseRemoval": "999"}

	if err := cfg.WriteSandboxVars(); err != nil {
		t.Fatalf("WriteSandboxVars: %v", err)
	}
	data, _ := os.ReadFile(cfg.SandboxVarsPath())
	if !strings.Contains(string(data), "HoursForCorpseRemoval = 999") {
		t.Errorf("SANDBOX_* override must win over SANDBOX_MODE:\n%s", data)
	}
}

func TestWriteSandboxVarsNestedOverride(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DataDir = t.TempDir()
	cfg.ServerName = "testworld"
	cfg.SandboxVars = map[string]string{
		"ZombieConfig.PopulationMultiplier": "0.2",
		"ZombieConfig.RallyGroupSize":       "5",
		"ZombieLore.Speed":                  "1",
	}

	if err := cfg.WriteSandboxVars(); err != nil {
		t.Fatalf("WriteSandboxVars: %v", err)
	}
	data, _ := os.ReadFile(cfg.SandboxVarsPath())
	content := string(data)

	for _, want := range []string{"PopulationMultiplier = 0.2", "RallyGroupSize = 5", "Speed = 1"} {
		if !strings.Contains(content, want) {
			t.Errorf("nested override missing %q:\n%s", want, content)
		}
	}
	// Nested keys must not leak to the top level of SandboxVars.
	for _, leak := range []string{"\n    PopulationMultiplier", "\n    Speed ="} {
		if strings.Contains(content, leak) {
			t.Errorf("nested key leaked to top level (%q):\n%s", leak, content)
		}
	}
	// Unknown nested tables are ignored (with a warning), not written.
	if strings.Contains(content, "Bogus") {
		t.Errorf("unknown nested table written:\n%s", content)
	}
}

func TestWriteSandboxVarsNestedOverrideWinsOverMode(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DataDir = t.TempDir()
	cfg.ServerName = "testworld"
	cfg.SandboxMode = "max"
	cfg.SandboxVars = map[string]string{"ZombieConfig.PopulationMultiplier": "0.5"}

	if err := cfg.WriteSandboxVars(); err != nil {
		t.Fatalf("WriteSandboxVars: %v", err)
	}
	data, _ := os.ReadFile(cfg.SandboxVarsPath())
	content := string(data)
	if !strings.Contains(content, "PopulationMultiplier = 0.5") {
		t.Errorf("nested SANDBOX_ZombieConfig.* must win over SANDBOX_MODE=max:\n%s", content)
	}
	if strings.Contains(content, "PopulationMultiplier = 0.35") {
		t.Errorf("max mode value leaked despite nested override:\n%s", content)
	}
}

func TestLuaValue(t *testing.T) {
	tests := []struct{ in, want string }{
		{"0.65", "0.65"},
		{"-1", "-1"},
		{"14", "14"},
		{"2.0", "2.0"},
		{"true", "true"},
		{"false", "false"},
		{`"Base.Hat, Base.Glasses"`, `"Base.Hat, Base.Glasses"`},
		{"Base.Hat", `"Base.Hat"`},
		{`a"b`, `"a\"b"`},
		{`a\b`, `"a\\b"`},
	}
	for _, tt := range tests {
		if got := luaValue(tt.in); got != tt.want {
			t.Errorf("luaValue(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestValidLuaValueRejectsControlChars(t *testing.T) {
	for _, v := range []string{"1)\nos.execute(\"rm -rf /\")", "a\x01b", "x\x7f"} {
		if validLuaValue(v) {
			t.Errorf("validLuaValue(%q) = true, want false", v)
		}
	}
}

func TestWriteSandboxVarsInjectionAttemptIgnored(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DataDir = t.TempDir()
	cfg.ServerName = "testworld"
	cfg.SandboxVars = map[string]string{
		"Zombies":                   "4",
		"Bad Key!":                  "1",
		"LootItemRemovalList":       "Base.Hat",
		"ZombieConfig.OwnerAttacks": "0\nend)",
	}

	if err := cfg.WriteSandboxVars(); err != nil {
		t.Fatalf("WriteSandboxVars: %v", err)
	}
	data, _ := os.ReadFile(cfg.SandboxVarsPath())
	content := string(data)

	if strings.Contains(content, "Bad Key!") {
		t.Errorf("invalid key written:\n%s", content)
	}
	if strings.Contains(content, "end)") || strings.Contains(content, "os.execute") {
		t.Errorf("injection value leaked into lua:\n%s", content)
	}
	if !strings.Contains(content, `LootItemRemovalList = "Base.Hat"`) {
		t.Errorf("string value not quoted:\n%s", content)
	}
	if strings.Contains(content, "ZombieConfig") && strings.Contains(content, "OwnerAttacks") {
		t.Errorf("nested injection leaked:\n%s", content)
	}
}
