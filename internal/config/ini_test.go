package config

import (
	"os"
	"strings"
	"testing"
)

func TestWriteIni(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DataDir = t.TempDir()
	cfg.ServerName = "testworld"
	cfg.BindIP = "0.0.0.0"
	cfg.RCONPassword = "rcon-pass"
	cfg.AdminPassword = "admin-pass"

	if err := cfg.WriteIni(); err != nil {
		t.Fatalf("WriteIni: %v", err)
	}

	data, err := os.ReadFile(cfg.ServerIniPath())
	if err != nil {
		t.Fatalf("reading ini: %v", err)
	}
	content := string(data)

	for _, want := range []string{
		"PublicName=" + cfg.PublicName,
		"DefaultPort=16261",
		"SteamPort1=16262",
		"RCONPort=27015",
		"RCONPassword=rcon-pass",
		"BindIP=0.0.0.0",
		"MaxPlayers=16",
		"SaveWorldEveryMinutes=15",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("ini missing %q:\n%s", want, content)
		}
	}
}

func TestWriteIniWithOptionalValues(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DataDir = t.TempDir()
	cfg.ServerPassword = "server-pass"
	cfg.ModNames = "mod1; mod2"
	cfg.ModWorkshopIDs = "12345"
	cfg.PauseOnEmpty = false

	if err := cfg.WriteIni(); err != nil {
		t.Fatalf("WriteIni: %v", err)
	}

	data, _ := os.ReadFile(cfg.ServerIniPath())
	content := string(data)
	for _, want := range []string{
		"Password=server-pass",
		"Mods=mod1; mod2",
		"WorkshopItems=12345",
		"PauseEmpty=false",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("ini missing %q:\n%s", want, content)
		}
	}

	// Empty password must not produce an empty Password= line.
	cfg2 := DefaultConfig()
	cfg2.DataDir = t.TempDir()
	cfg2.ServerPassword = ""
	if err := cfg2.WriteIni(); err != nil {
		t.Fatalf("WriteIni: %v", err)
	}
	content2, _ := os.ReadFile(cfg2.ServerIniPath())
	if strings.Contains(string(content2), "\nPassword=") {
		t.Errorf("ini contains empty Password line:\n%s", content2)
	}
}

func TestParseMapNames(t *testing.T) {
	got := parseMapNames("Muldraugh, KY; Rosewood")
	if got != "Muldraugh, KY;Rosewood" {
		t.Errorf("parseMapNames = %q", got)
	}
	got = parseMapNames("  A ; ; B ")
	if got != "A;B" {
		t.Errorf("parseMapNames with empties = %q", got)
	}
}
