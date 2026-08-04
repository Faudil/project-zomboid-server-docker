package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeLauncherJson(t *testing.T, vmArgs []string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "ProjectZomboid64.json")
	lj := launcherJson{
		MainClass: "zombie/network/GameServer",
		Classpath: []string{"java/.", "java/projectzomboid.jar"},
		VMArgs:    vmArgs,
	}
	data, err := json.MarshalIndent(lj, "", "\t")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return dir
}

func readLauncherJson(t *testing.T, dir string) launcherJson {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "ProjectZomboid64.json"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var lj launcherJson
	if err := json.Unmarshal(data, &lj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return lj
}

func join(v []string) string { return strings.Join(v, " ") }

func TestPatchLauncherJson(t *testing.T) {
	dir := writeLauncherJson(t, []string{
		"-Djava.awt.headless=true",
		"-Xmx8g",
		"-Dzomboid.steam=1",
		"-Djava.security.egd=file:/dev/urandom",
		"-XX:+UseZGC",
		"-XX:-OmitStackTraceInFastThrow",
	})

	cfg := DefaultConfig()
	cfg.ServerDir = dir
	cfg.MaxRam = "6144m"
	cfg.MinRam = "2048m"
	cfg.GCConfig = "G1"
	cfg.JvmExtraArgs = "-XX:MaxGCPauseMillis=100"

	if err := cfg.PatchLauncherJson(); err != nil {
		t.Fatalf("PatchLauncherJson: %v", err)
	}

	got := join(readLauncherJson(t, dir).VMArgs)
	for _, want := range []string{"-Xms2048m", "-Xmx6144m", "-XX:+UseG1GC", "-Djava.awt.headless=true", "-Dzomboid.steam=1", "-Djava.security.egd=file:/dev/urandom", "-XX:-OmitStackTraceInFastThrow", "-XX:MaxGCPauseMillis=100"} {
		if !strings.Contains(got, want) {
			t.Errorf("patched vmArgs missing %q:\n%s", want, got)
		}
	}
	for _, legacy := range []string{"-Xmx8g", "-XX:+UseZGC"} {
		if strings.Contains(got, legacy) {
			t.Errorf("patched vmArgs still contains %q:\n%s", legacy, got)
		}
	}
}

func TestPatchLauncherJsonIdempotent(t *testing.T) {
	dir := writeLauncherJson(t, []string{"-Xmx8g", "-Dzomboid.steam=1"})

	cfg := DefaultConfig()
	cfg.ServerDir = dir
	cfg.MaxRam = "8192m"
	cfg.MinRam = "8192m"
	cfg.GCConfig = "ZGC"

	if err := cfg.PatchLauncherJson(); err != nil {
		t.Fatalf("first patch: %v", err)
	}
	first := readLauncherJson(t, dir)
	if err := cfg.PatchLauncherJson(); err != nil {
		t.Fatalf("second patch: %v", err)
	}
	second := readLauncherJson(t, dir)

	if join(first.VMArgs) != join(second.VMArgs) {
		t.Errorf("not idempotent:\nfirst:  %v\nsecond: %v", first.VMArgs, second.VMArgs)
	}
	if got := join(second.VMArgs); !strings.Contains(got, "-Xmx8192m") || !strings.Contains(got, "-Xms8192m") || !strings.Contains(got, "-XX:+UseZGC") {
		t.Errorf("unexpected vmArgs after second patch: %v", second.VMArgs)
	}
}

func TestPatchLauncherJsonMissingFile(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ServerDir = t.TempDir()
	if err := cfg.PatchLauncherJson(); err != nil {
		t.Errorf("missing json should be skipped, got error: %v", err)
	}
}

func TestPatchLauncherJsonDefaultGC(t *testing.T) {
	dir := writeLauncherJson(t, []string{"-Xmx8g"})

	cfg := DefaultConfig()
	cfg.ServerDir = dir
	cfg.MaxRam = "4096m"
	cfg.MinRam = "4096m"
	cfg.GCConfig = "ZGC"

	if err := cfg.PatchLauncherJson(); err != nil {
		t.Fatalf("PatchLauncherJson: %v", err)
	}
	got := join(readLauncherJson(t, dir).VMArgs)
	if !strings.Contains(got, "-XX:+UseZGC") {
		t.Errorf("default GC_CONFIG not applied:\n%s", got)
	}
}
