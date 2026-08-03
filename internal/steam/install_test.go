package steam

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/faudil/project-zomboid-server-docker/internal/config"
)

func writeModDir(t *testing.T, dir string, withModInfo bool) {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if withModInfo {
		if err := os.WriteFile(filepath.Join(dir, "mod.info"), []byte("name=test\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

func testConfig(t *testing.T) *config.ServerConfig {
	t.Helper()
	cfg := config.DefaultConfig()
	cfg.ServerDir = t.TempDir()
	cfg.DataDir = t.TempDir()
	return cfg
}

func TestDiscoverModNames(t *testing.T) {
	cfg := testConfig(t)

	// Standard layout: <item>/mods/<ModName>/
	writeModDir(t, filepath.Join(cfg.ServerDir, "steamapps/workshop/content/108600/2503743612/mods/SkillRecoveryJournal"), true)
	// Second mod inside the same item.
	writeModDir(t, filepath.Join(cfg.ServerDir, "steamapps/workshop/content/108600/2503743612/mods/SecondMod"), true)
	// Legacy layout: <item>/<ModName>/
	writeModDir(t, filepath.Join(cfg.ServerDir, "steamapps/workshop/content/108600/2160432461/OldStyleMod"), true)
	// Item with no mods/ (e.g. a texture pack) - must be ignored.
	writeModDir(t, filepath.Join(cfg.ServerDir, "steamapps/workshop/content/108600/1111111111"), false)
	// Folder without mod.info in a mods/ dir - ignored.
	writeModDir(t, filepath.Join(cfg.ServerDir, "steamapps/workshop/content/108600/2222222222/mods/NotAMod"), false)
	// Manual mod in <DATA_DIR>/Workshop/
	writeModDir(t, filepath.Join(cfg.DataDir, "Workshop/ManualMod"), true)

	names := DiscoverModNames(cfg)
	want := []string{"ManualMod", "OldStyleMod", "SecondMod", "SkillRecoveryJournal"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("DiscoverModNames = %v, want %v", names, want)
	}
}

func TestDiscoverModNamesEmpty(t *testing.T) {
	cfg := testConfig(t)
	if names := DiscoverModNames(cfg); len(names) != 0 {
		t.Errorf("DiscoverModNames = %v, want none", names)
	}
}

func TestWarnMissingMods(t *testing.T) {
	cfg := testConfig(t)
	writeModDir(t, filepath.Join(cfg.ServerDir, "steamapps/workshop/content/108600/2503743612/mods/GoodMod"), true)
	cfg.ModNames = "GoodMod;TypoMod"

	// Must not panic; TypoMod gets a warning, GoodMod does not.
	WarnMissingMods(cfg)
}

func TestResolveModWorkshopIDsMergesCollections(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{
			"response": {
				"result": [{
					"publishedfileid": "9999",
					"children": [
						{"publishedfileid": "2685168362"},
						{"publishedfileid": "2503743612"},
						{"publishedfileid": "2685168362"}
					]
				}]
			}
		}`)
	}))
	defer server.Close()
	oldAPI := collectionAPI
	collectionAPI = server.URL
	defer func() { collectionAPI = oldAPI }()

	cfg := testConfig(t)
	cfg.ModWorkshopIDs = "2160432461;2503743612"
	cfg.ModWorkshopCollection = "9999"

	ids := ResolveModWorkshopIDs(cfg)
	want := []string{"2160432461", "2503743612", "2685168362"}
	if !reflect.DeepEqual(ids, want) {
		t.Errorf("ResolveModWorkshopIDs = %v, want %v", ids, want)
	}
}

func TestResolveModWorkshopIDsCollectionFailureIsWarning(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "missing key", http.StatusUnauthorized)
	}))
	defer server.Close()
	oldAPI := collectionAPI
	collectionAPI = server.URL
	defer func() { collectionAPI = oldAPI }()

	cfg := testConfig(t)
	cfg.ModWorkshopIDs = "2160432461"
	cfg.ModWorkshopCollection = "9999"

	// Explicit IDs survive a failed collection lookup.
	ids := ResolveModWorkshopIDs(cfg)
	if !reflect.DeepEqual(ids, []string{"2160432461"}) {
		t.Errorf("ResolveModWorkshopIDs = %v, want explicit IDs only", ids)
	}
}

func TestResolveModWorkshopIDsEmpty(t *testing.T) {
	cfg := testConfig(t)
	if ids := ResolveModWorkshopIDs(cfg); len(ids) != 0 {
		t.Errorf("ResolveModWorkshopIDs = %v, want none", ids)
	}
}

func TestResolveCollectionNotfound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{"response": {"result": []}}`)
	}))
	defer server.Close()
	oldAPI := collectionAPI
	collectionAPI = server.URL
	defer func() { collectionAPI = oldAPI }()

	cfg := testConfig(t)
	if _, err := resolveCollection(cfg, "9999"); err == nil {
		t.Fatal("resolveCollection should fail for an unknown collection")
	}
}

func TestResolveCollectionSendsAPIKey(t *testing.T) {
	var gotKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.URL.Query().Get("key")
		fmt.Fprintln(w, `{"response": {"result": [{"publishedfileid": "9999", "children": []}]}}`)
	}))
	defer server.Close()
	oldAPI := collectionAPI
	collectionAPI = server.URL
	defer func() { collectionAPI = oldAPI }()

	cfg := testConfig(t)
	cfg.SteamAPIKey = "secret-key"
	if _, err := resolveCollection(cfg, "9999"); err != nil {
		t.Fatalf("resolveCollection: %v", err)
	}
	if gotKey != "secret-key" {
		t.Errorf("key = %q, want secret-key", gotKey)
	}
}

func TestDownloadWorkshopItemsSkipsExisting(t *testing.T) {
	cfg := testConfig(t)
	itemDir := filepath.Join(cfg.ServerDir, "steamapps/workshop/content/108600/2503743612")
	writeModDir(t, itemDir, false)

	// All items present -> nothing to download, returns without running steamcmd.
	if err := DownloadWorkshopItems(cfg, []string{"2503743612"}); err != nil {
		t.Errorf("DownloadWorkshopItems: %v", err)
	}
}

func TestDownloadWorkshopItemsForcesUpdate(t *testing.T) {
	cfg := testConfig(t)
	itemDir := filepath.Join(cfg.ServerDir, "steamapps/workshop/content/108600/2503743612")
	writeModDir(t, itemDir, false)
	cfg.ModUpdateOnStart = true

	// ModUpdateOnStart bypasses the existence check; steamcmd is missing in
	// tests so this must fail with an error (proving it attempted to run).
	if err := DownloadWorkshopItems(cfg, []string{"2503743612"}); err == nil {
		t.Error("DownloadWorkshopItems should attempt the download when ModUpdateOnStart is set")
	}
}

func TestWorkshopBatchArgs(t *testing.T) {
	cfg := testConfig(t)
	args := workshopBatchArgs(cfg, []string{"2160432461", "2503743612"})

	want := []string{
		"workshop_download_item 108600 2160432461",
		"workshop_download_item 108600 2503743612",
		"+quit",
	}
	if !reflect.DeepEqual(args, want) {
		t.Errorf("workshopBatchArgs = %v, want %v", args, want)
	}
}

func TestSteamLoginArgsAnonymous(t *testing.T) {
	cfg := testConfig(t)
	cfg.SteamUser = ""
	if got := steamLoginArgs(cfg); !reflect.DeepEqual(got, []string{"+login", "anonymous"}) {
		t.Errorf("steamLoginArgs = %v, want anonymous", got)
	}
}

func TestSteamLoginArgsCredentials(t *testing.T) {
	cfg := testConfig(t)
	cfg.SteamUser = "myuser"
	cfg.SteamPass = "mypass"
	want := []string{"+login", "myuser", "mypass"}
	if got := steamLoginArgs(cfg); !reflect.DeepEqual(got, want) {
		t.Errorf("steamLoginArgs = %v, want %v", got, want)
	}
}

func TestSteamLoginArgsGuardCode(t *testing.T) {
	cfg := testConfig(t)
	cfg.SteamUser = "myuser"
	cfg.SteamPass = "mypass"
	cfg.SteamGuardCode = "ABC12"
	want := []string{"+set_steam_guard_code", "ABC12", "+login", "myuser", "mypass"}
	if got := steamLoginArgs(cfg); !reflect.DeepEqual(got, want) {
		t.Errorf("steamLoginArgs = %v, want %v", got, want)
	}
}

func TestSteamFailureDetection(t *testing.T) {
	cases := []struct {
		output string
		want   bool
	}{
		{"ERROR! Failed to install app '380870' (Missing file permissions)", true},
		{"ERROR! Failed to install app '380870' (No subscription)", true},
		{"...\nERROR! Failed to install app '380870' (Missing configuration)\n", true},
		{"Success! App '380870' fully installed", false},
		{"Connecting anonymously to Steam Public...OK\nWaiting for user info...OK", false},
	}
	for _, tc := range cases {
		if got := steamFailure(tc.output) != ""; got != tc.want {
			t.Errorf("steamFailure(%q) = %v, want %v", tc.output, got, tc.want)
		}
	}
}

func TestInstallOrUpdateRequiresPassWithUser(t *testing.T) {
	cfg := testConfig(t)
	cfg.SteamUser = "myuser"
	cfg.SteamPass = ""
	if err := InstallOrUpdate(cfg); err == nil {
		t.Fatal("InstallOrUpdate should fail when STEAM_USER is set without STEAM_PASS")
	}
}

func TestInstallOrUpdateSkipsWhenPresent(t *testing.T) {
	cfg := testConfig(t)
	cfg.UpdateOnStart = false
	cfg.SteamUser = ""
	if err := os.WriteFile(startScriptPath(cfg), []byte("#!/bin/bash\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := InstallOrUpdate(cfg); err != nil {
		t.Errorf("InstallOrUpdate should skip when files exist and UPDATE_ON_START=false, got %v", err)
	}
}

func TestInstallOrUpdateRetriesTransientFailure(t *testing.T) {
	cfg := testConfig(t)
	cfg.SteamUser = ""

	oldRun := runSteamCmdCapture
	oldDelay := updateRetryDelay
	runSteamCmdCapture = func(args ...string) (string, error) {
		// Transient Valve-side regression, then success.
		if _, err := os.Stat(startScriptPath(cfg)); err != nil {
			if err := os.WriteFile(startScriptPath(cfg), []byte("#!/bin/bash\n"), 0755); err != nil {
				t.Fatal(err)
			}
			return "ERROR! Failed to install app '380870' (Missing file permissions)", nil
		}
		return "Success! App '380870' fully installed", nil
	}
	updateRetryDelay = time.Millisecond
	defer func() {
		runSteamCmdCapture = oldRun
		updateRetryDelay = oldDelay
	}()

	if err := InstallOrUpdate(cfg); err != nil {
		t.Errorf("InstallOrUpdate should succeed after transient failure, got %v", err)
	}
}

func TestInstallOrUpdateExhaustsRetries(t *testing.T) {
	cfg := testConfig(t)
	cfg.SteamUser = ""

	oldRun := runSteamCmdCapture
	oldDelay := updateRetryDelay
	calls := 0
	runSteamCmdCapture = func(args ...string) (string, error) {
		calls++
		return "ERROR! Failed to install app '380870' (Missing file permissions)", nil
	}
	updateRetryDelay = time.Millisecond
	defer func() {
		runSteamCmdCapture = oldRun
		updateRetryDelay = oldDelay
	}()

	err := InstallOrUpdate(cfg)
	if err == nil {
		t.Fatal("InstallOrUpdate should fail after exhausting retries")
	}
	if calls != maxUpdateAttempts {
		t.Errorf("steamcmd ran %d times, want %d", calls, maxUpdateAttempts)
	}
}

func TestInstallOrUpdatePermanentFailureNoRetry(t *testing.T) {
	cfg := testConfig(t)
	cfg.SteamUser = "myuser"
	cfg.SteamPass = "mypass"

	oldRun := runSteamCmdCapture
	oldDelay := updateRetryDelay
	calls := 0
	runSteamCmdCapture = func(args ...string) (string, error) {
		calls++
		return "ERROR! Failed to login: Invalid Password", nil
	}
	updateRetryDelay = time.Millisecond
	defer func() {
		runSteamCmdCapture = oldRun
		updateRetryDelay = oldDelay
	}()

	if err := InstallOrUpdate(cfg); err == nil {
		t.Fatal("InstallOrUpdate should fail on bad credentials")
	}
	if calls != 1 {
		t.Errorf("steamcmd ran %d times, want 1 (no retry on permanent failure)", calls)
	}
}
