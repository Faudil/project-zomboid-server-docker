package steam

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/faudil/project-zomboid-server-docker/internal/config"
)

func TestLoadUpdateStateMissing(t *testing.T) {
	state, err := LoadUpdateState(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatalf("LoadUpdateState: %v", err)
	}
	if len(state.Mods) != 0 || state.GameBuildID != "" {
		t.Fatalf("state = %+v, want empty", state)
	}
}

func TestSaveAndLoadUpdateState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update-state.json")
	in := UpdateState{Mods: map[string]int64{"111": 100, "222": 200}, GameBuildID: "12345"}
	if err := SaveUpdateState(path, in); err != nil {
		t.Fatalf("SaveUpdateState: %v", err)
	}

	out, err := LoadUpdateState(path)
	if err != nil {
		t.Fatalf("LoadUpdateState: %v", err)
	}
	if out.GameBuildID != "12345" || out.Mods["111"] != 100 || out.Mods["222"] != 200 {
		t.Fatalf("round trip mismatch: %+v", out)
	}
}

func TestLoadUpdateStateNilMods(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update-state.json")
	if err := os.WriteFile(path, []byte(`{"gameBuildId":"9"}`), 0644); err != nil {
		t.Fatal(err)
	}
	state, err := LoadUpdateState(path)
	if err != nil {
		t.Fatalf("LoadUpdateState: %v", err)
	}
	if state.Mods == nil {
		t.Fatal("Mods map must not be nil")
	}
}

func TestFetchModUpdateTimes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		if r.Form.Get("itemcount") != "2" {
			t.Errorf("itemcount = %q", r.Form.Get("itemcount"))
		}
		fmt.Fprint(w, `{"response":{"result":1,"publishedfiledetails":[
			{"publishedfileid":"111","time_updated":100},
			{"publishedfileid":"222","time_updated":200}]}}`)
	}))
	defer server.Close()

	orig := publishedFileDetailsAPI
	publishedFileDetailsAPI = server.URL
	defer func() { publishedFileDetailsAPI = orig }()

	times, err := FetchModUpdateTimes([]string{"111", "222"})
	if err != nil {
		t.Fatalf("FetchModUpdateTimes: %v", err)
	}
	if times["111"] != 100 || times["222"] != 200 {
		t.Fatalf("times = %v", times)
	}
}

func TestFetchModUpdateTimesHTTPError(t *testing.T) {
	orig := publishedFileDetailsAPI
	publishedFileDetailsAPI = "http://127.0.0.1:1/unreachable"
	defer func() { publishedFileDetailsAPI = orig }()

	if _, err := FetchModUpdateTimes([]string{"111"}); err == nil {
		t.Fatal("want error for unreachable API")
	}
}

func TestFetchGameBuildID(t *testing.T) {
	orig := runSteamCmdCapture
	runSteamCmdCapture = func(args ...string) (string, error) {
		joined := strings.Join(args, " ")
		if !strings.Contains(joined, "app_info_print 380870") {
			t.Errorf("args = %s, want app_info_print 380870", joined)
		}
		return `"380870"
{
	"depots" { }
	"branches"
	{
		"public"
		{
			"description" "public"
			"buildid" "13123456"
		}
	}
}`, nil
	}
	defer func() { runSteamCmdCapture = orig }()

	buildID, err := FetchGameBuildID()
	if err != nil {
		t.Fatalf("FetchGameBuildID: %v", err)
	}
	if buildID != "13123456" {
		t.Fatalf("buildID = %q", buildID)
	}
}

func TestFetchGameBuildIDNoMatch(t *testing.T) {
	orig := runSteamCmdCapture
	runSteamCmdCapture = func(args ...string) (string, error) { return "garbage", nil }
	defer func() { runSteamCmdCapture = orig }()

	if _, err := FetchGameBuildID(); err == nil {
		t.Fatal("want error when output has no public buildid")
	}
}

// checkState returns the state file the config would use, with a fresh temp dir.
func checkConfig(t *testing.T) *config.ServerConfig {
	t.Helper()
	cfg := config.DefaultConfig()
	cfg.DataDir = t.TempDir()
	return cfg
}

func TestCheckForUpdatesFirstRunBaseline(t *testing.T) {
	cfg := checkConfig(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"response":{"result":1,"publishedfiledetails":[
			{"publishedfileid":"111","time_updated":100}]}}`)
	}))
	defer server.Close()
	origAPI := publishedFileDetailsAPI
	publishedFileDetailsAPI = server.URL
	defer func() { publishedFileDetailsAPI = origAPI }()

	orig := runSteamCmdCapture
	runSteamCmdCapture = func(args ...string) (string, error) {
		return `"380870" { "branches" { "public" { "buildid" "9" } } }`, nil
	}
	defer func() { runSteamCmdCapture = orig }()

	updated, game, err := CheckForUpdates(cfg, []string{"111"})
	if err != nil {
		t.Fatalf("CheckForUpdates: %v", err)
	}
	if len(updated) != 0 || game {
		t.Fatalf("first run must not report updates: mods=%v game=%v", updated, game)
	}

	state, err := LoadUpdateState(UpdateStatePath(cfg))
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if state.Mods["111"] != 100 || state.GameBuildID != "9" {
		t.Fatalf("baseline not persisted: %+v", state)
	}
}

func TestCheckForUpdatesDetectsModAndGameUpdate(t *testing.T) {
	cfg := checkConfig(t)
	statePath := UpdateStatePath(cfg)
	if err := SaveUpdateState(statePath, UpdateState{
		Mods:        map[string]int64{"111": 100, "222": 200},
		GameBuildID: "9",
	}); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"response":{"result":1,"publishedfiledetails":[
			{"publishedfileid":"111","time_updated":100},
			{"publishedfileid":"222","time_updated":999}]}}`)
	}))
	defer server.Close()
	origAPI := publishedFileDetailsAPI
	publishedFileDetailsAPI = server.URL
	defer func() { publishedFileDetailsAPI = origAPI }()

	orig := runSteamCmdCapture
	runSteamCmdCapture = func(args ...string) (string, error) {
		return `"380870" { "branches" { "public" { "buildid" "42" } } }`, nil
	}
	defer func() { runSteamCmdCapture = orig }()

	updated, game, err := CheckForUpdates(cfg, []string{"111", "222"})
	if err != nil {
		t.Fatalf("CheckForUpdates: %v", err)
	}
	if !game || len(updated) != 1 || updated[0] != "222" {
		t.Fatalf("updates = mods %v game %v, want [222] true", updated, game)
	}

	state, err := LoadUpdateState(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if state.Mods["222"] != 999 || state.GameBuildID != "42" {
		t.Fatalf("state not advanced: %+v", state)
	}
}

func TestCheckForUpdatesPrunesRemovedMods(t *testing.T) {
	cfg := checkConfig(t)
	statePath := UpdateStatePath(cfg)
	if err := SaveUpdateState(statePath, UpdateState{
		Mods:        map[string]int64{"111": 100, "222": 200},
		GameBuildID: "9",
	}); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"response":{"result":1,"publishedfiledetails":[
			{"publishedfileid":"111","time_updated":100}]}}`)
	}))
	defer server.Close()
	origAPI := publishedFileDetailsAPI
	publishedFileDetailsAPI = server.URL
	defer func() { publishedFileDetailsAPI = origAPI }()

	orig := runSteamCmdCapture
	runSteamCmdCapture = func(args ...string) (string, error) {
		return `"380870" { "branches" { "public" { "buildid" "9" } } }`, nil
	}
	defer func() { runSteamCmdCapture = orig }()

	if _, _, err := CheckForUpdates(cfg, []string{"111"}); err != nil {
		t.Fatalf("CheckForUpdates: %v", err)
	}

	state, err := LoadUpdateState(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := state.Mods["222"]; ok {
		t.Fatalf("removed mod 222 still in state: %+v", state)
	}
}

func TestCheckForUpdatesGameCheckFailureIsNonFatal(t *testing.T) {
	cfg := checkConfig(t)
	statePath := UpdateStatePath(cfg)
	if err := SaveUpdateState(statePath, UpdateState{
		Mods:        map[string]int64{"111": 100},
		GameBuildID: "9",
	}); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"response":{"result":1,"publishedfiledetails":[
			{"publishedfileid":"111","time_updated":100}]}}`)
	}))
	defer server.Close()
	origAPI := publishedFileDetailsAPI
	publishedFileDetailsAPI = server.URL
	defer func() { publishedFileDetailsAPI = origAPI }()

	orig := runSteamCmdCapture
	runSteamCmdCapture = func(args ...string) (string, error) { return "", fmt.Errorf("steamcmd exploded") }
	defer func() { runSteamCmdCapture = orig }()

	updated, game, err := CheckForUpdates(cfg, []string{"111"})
	if err != nil {
		t.Fatalf("CheckForUpdates must tolerate game check failure: %v", err)
	}
	if len(updated) != 0 || game {
		t.Fatalf("unexpected updates: mods=%v game=%v", updated, game)
	}

	// The persisted state must still be written when only the game check fails.
	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("state not saved: %v", err)
	}
}
