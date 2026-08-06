package steam

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/faudil/project-zomboid-server-docker/internal/config"
)

// publishedFileDetailsAPI is the Steam Web API endpoint that reports the
// last-modified time of workshop items. Unlike GetCollectionDetails it works
// without an API key, so auto-update detection needs no credentials.
// Overridable in tests.
var publishedFileDetailsAPI = "https://api.steampowered.com/ISteamRemoteStorage/GetPublishedFileDetails/v1/"

// UpdateState is the persisted record of the last update baseline seen for
// workshop mods and the game build. It lives in the data volume so it
// survives container restarts.
type UpdateState struct {
	// Mods maps workshop item ID to its last seen time_updated (unix).
	Mods map[string]int64 `json:"mods"`
	// GameBuildID is the last seen public branch buildid of the game app.
	GameBuildID string `json:"gameBuildId"`
}

// UpdateStatePath returns the state file location inside the data volume.
func UpdateStatePath(cfg *config.ServerConfig) string {
	return filepath.Join(cfg.DataDir, "update-state.json")
}

// LoadUpdateState reads the persisted update baseline, returning an empty
// state when the file does not exist yet (first run).
func LoadUpdateState(path string) (UpdateState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return UpdateState{Mods: map[string]int64{}}, nil
		}
		return UpdateState{}, fmt.Errorf("reading update state %s: %w", path, err)
	}

	var state UpdateState
	if err := json.Unmarshal(data, &state); err != nil {
		return UpdateState{}, fmt.Errorf("decoding update state %s: %w", path, err)
	}
	if state.Mods == nil {
		state.Mods = map[string]int64{}
	}
	return state, nil
}

// SaveUpdateState persists the update baseline.
func SaveUpdateState(path string, state UpdateState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding update state: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating update state directory: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

// FetchModUpdateTimes queries the Steam Workshop for the last-modified time
// of every item ID. Items Steam cannot resolve (private, invalid) are skipped
// silently; the endpoint needs no API key.
func FetchModUpdateTimes(ids []string) (map[string]int64, error) {
	if len(ids) == 0 {
		return map[string]int64{}, nil
	}

	form := url.Values{}
	form.Set("itemcount", fmt.Sprintf("%d", len(ids)))
	for i, id := range ids {
		form.Set(fmt.Sprintf("publishedfileids[%d]", i), id)
	}

	req, err := http.NewRequest(http.MethodPost, publishedFileDetailsAPI, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Steam API returned status %d", resp.StatusCode)
	}

	var out struct {
		Response struct {
			Details []struct {
				PublishedFileID string `json:"publishedfileid"`
				TimeUpdated     int64  `json:"time_updated"`
			} `json:"publishedfiledetails"`
		} `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decoding Steam API response: %w", err)
	}

	times := make(map[string]int64, len(out.Response.Details))
	for _, d := range out.Response.Details {
		if d.PublishedFileID != "" {
			times[d.PublishedFileID] = d.TimeUpdated
		}
	}
	return times, nil
}

// appInfoBuildIDRE extracts the public branch buildid from steamcmd's
// app_info_print output: branches appear as nested "name" { ... } blocks and
// the block's "buildid" is the branch's current build.
var appInfoBuildIDRE = regexp.MustCompile(`"public"\s*\{[^}]*"buildid"\s*"(\d+)"`)

// FetchGameBuildID returns the current public branch buildid of the game app
// by asking steamcmd for its app info. This is the same source SteamCMD's own
// update checks use, needs no credentials, and takes only a few seconds.
func FetchGameBuildID() (string, error) {
	args := []string{
		"+login", "anonymous",
		"+app_info_update", "1",
		"+app_info_print", "380870",
		"+quit",
	}
	output, err := runSteamCmdCapture(args...)
	if err != nil {
		return "", fmt.Errorf("running steamcmd app_info: %w", err)
	}

	m := appInfoBuildIDRE.FindStringSubmatch(output)
	if m == nil {
		return "", fmt.Errorf("no public buildid found in steamcmd output")
	}
	return m[1], nil
}

// CheckForUpdates compares the current workshop/game state against the
// persisted baseline and returns what changed. The baseline is updated and
// saved in the process, so a first run never triggers a restart: it only
// records where the mods/game stand. Mod IDs that are no longer configured
// are pruned from the state.
func CheckForUpdates(cfg *config.ServerConfig, ids []string) (updatedMods []string, gameUpdated bool, err error) {
	statePath := UpdateStatePath(cfg)
	state, err := LoadUpdateState(statePath)
	if err != nil {
		return nil, false, err
	}

	times, err := FetchModUpdateTimes(ids)
	if err != nil {
		return nil, false, fmt.Errorf("checking workshop mods: %w", err)
	}

	firstRun := len(state.Mods) == 0 && state.GameBuildID == ""
	for _, id := range ids {
		t, ok := times[id]
		if !ok {
			continue // item unresolvable; leave the baseline untouched
		}
		prev, seen := state.Mods[id]
		if seen && prev != t {
			updatedMods = append(updatedMods, id)
		}
		state.Mods[id] = t
	}

	// Prune IDs that are no longer configured.
	for id := range state.Mods {
		if _, ok := times[id]; !ok {
			delete(state.Mods, id)
		}
	}

	buildID, err := FetchGameBuildID()
	if err != nil {
		// Game checks are best-effort: a steamcmd hiccup must not block mod
		// updates. Log and continue with the mod result only.
		fmt.Printf("WARNING: game build check failed: %v\n", err)
	} else if state.GameBuildID != "" && state.GameBuildID != buildID {
		gameUpdated = true
		state.GameBuildID = buildID
	} else {
		state.GameBuildID = buildID
	}

	if err := SaveUpdateState(statePath, state); err != nil {
		return nil, false, fmt.Errorf("saving update state: %w", err)
	}

	if firstRun {
		fmt.Println("Recorded initial update baseline (no restart on first check)")
	}
	if len(updatedMods) > 0 {
		sort.Strings(updatedMods)
	}
	return updatedMods, gameUpdated, nil
}
