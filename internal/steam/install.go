package steam

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/faudil/project-zomboid-server-docker/internal/config"
)

const steamcmdPath = "/home/steam/steamcmd/steamcmd.sh"

// workshopAppID is the Steam Workshop app id for Project Zomboid.
const workshopAppID = "108600"

// collectionAPI is the Steam Web API endpoint used to resolve workshop
// collections. Overridable in tests.
var collectionAPI = "https://api.steampowered.com/ISteamRemoteStorage/GetCollectionDetails/v1/"

func runSteamCmd(args ...string) error {
	cmd := exec.Command(steamcmdPath, args...)
	cmd.Dir = filepath.Dir(steamcmdPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "HOME=/home/steam")
	return cmd.Run()
}

func InstallOrUpdate(cfg *config.ServerConfig) error {
	if !cfg.UpdateOnStart {
		_, err := os.Stat(cfg.ServerDir + "/start-server.sh")
		if err == nil {
			return nil
		}
	}

	updateCmd := fmt.Sprintf("app_update %s validate", cfg.SteamAppID)
	if cfg.ServerBranch != "" {
		updateCmd = fmt.Sprintf("app_update %s -beta %s validate", cfg.SteamAppID, cfg.ServerBranch)
	}

	args := []string{
		"+force_install_dir", cfg.ServerDir,
		"+login", "anonymous",
		updateCmd,
		"+quit",
	}

	if err := runSteamCmd(args...); err != nil {
		return fmt.Errorf("steamcmd install/update failed: %w", err)
	}

	return nil
}

func workshopDir(cfg *config.ServerConfig) string {
	return filepath.Join(cfg.ServerDir, "steamapps", "workshop", "content", workshopAppID)
}

// ResolveModWorkshopIDs returns the full list of workshop item IDs to
// download: explicit MOD_WORKSHOP_IDS plus items resolved from
// MOD_WORKSHOP_COLLECTION_IDS. Collections that cannot be resolved only log a
// warning so an explicit ID list keeps working.
func ResolveModWorkshopIDs(cfg *config.ServerConfig) []string {
	ids := splitIDs(cfg.ModWorkshopIDs)

	if cfg.ModWorkshopCollection != "" {
		for _, collID := range splitIDs(cfg.ModWorkshopCollection) {
			items, err := resolveCollection(cfg, collID)
			if err != nil {
				fmt.Printf("WARNING: could not resolve workshop collection %s: %v\n", collID, err)
				continue
			}
			fmt.Printf("Resolved workshop collection %s: %d item(s)\n", collID, len(items))
			ids = append(ids, items...)
		}
	}

	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func splitIDs(raw string) []string {
	if raw == "" {
		return nil
	}
	var out []string
	for _, id := range strings.Split(raw, ";") {
		id = strings.TrimSpace(id)
		if id != "" {
			out = append(out, id)
		}
	}
	return out
}

// resolveCollection fetches the item IDs of a Steam workshop collection.
// GetCollectionDetails works without an API key; when a key is configured it
// is appended for reliability.
func resolveCollection(cfg *config.ServerConfig, collectionID string) ([]string, error) {
	payload := map[string]interface{}{
		"collectioncount":  1,
		"publishedfileids": []string{collectionID},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, collectionAPI, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.SteamAPIKey != "" {
		q := req.URL.Query()
		q.Set("key", cfg.SteamAPIKey)
		req.URL.RawQuery = q.Encode()
	}

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
			Result []struct {
				PublishedFileID string `json:"publishedfileid"`
				Children        []struct {
					PublishedFileID string `json:"publishedfileid"`
				} `json:"children"`
			} `json:"result"`
		} `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decoding Steam API response: %w", err)
	}

	if len(out.Response.Result) == 0 || out.Response.Result[0].PublishedFileID == "" {
		return nil, fmt.Errorf("collection %s not found", collectionID)
	}

	var ids []string
	for _, child := range out.Response.Result[0].Children {
		if child.PublishedFileID != "" {
			ids = append(ids, child.PublishedFileID)
		}
	}
	return ids, nil
}

// DownloadWorkshopItems downloads all workshop items in a single steamcmd
// session. Items already on disk are skipped unless ModUpdateOnStart is set.
// steamcmd's exit code does not reflect per-item failures, so each item's
// presence is verified afterwards.
func DownloadWorkshopItems(cfg *config.ServerConfig, ids []string) error {
	dir := workshopDir(cfg)
	var toDownload []string

	for _, id := range ids {
		if _, err := os.Stat(filepath.Join(dir, id)); err == nil && !cfg.ModUpdateOnStart {
			fmt.Printf("Workshop mod %s already downloaded\n", id)
			continue
		}
		toDownload = append(toDownload, id)
	}

	if len(toDownload) == 0 {
		return nil
	}

	if err := runSteamCmd(workshopBatchArgs(cfg, toDownload)...); err != nil {
		return fmt.Errorf("downloading workshop mods: %w", err)
	}

	for _, id := range toDownload {
		if _, err := os.Stat(filepath.Join(dir, id)); err != nil {
			fmt.Printf("WARNING: workshop item %s did not download (private, region-locked, or invalid ID)\n", id)
		} else {
			fmt.Printf("Downloaded workshop mod %s\n", id)
		}
	}

	return nil
}

// workshopBatchArgs builds a single steamcmd session downloading multiple
// workshop items.
func workshopBatchArgs(cfg *config.ServerConfig, ids []string) []string {
	args := []string{
		"+force_install_dir", cfg.ServerDir,
		"+login", "anonymous",
	}
	for _, id := range ids {
		args = append(args, fmt.Sprintf("workshop_download_item %s %s", workshopAppID, id))
	}
	return append(args, "+quit")
}

// scanModFolders finds every mod folder on disk and maps its name to its
// source (workshop item ID or "manual").
func scanModFolders(cfg *config.ServerConfig) map[string]string {
	mods := make(map[string]string)
	add := func(name, source string) {
		if _, ok := mods[name]; !ok {
			mods[name] = source
		}
	}

	itemDirs, err := os.ReadDir(workshopDir(cfg))
	if err == nil {
		for _, item := range itemDirs {
			if !item.IsDir() {
				continue
			}
			itemPath := filepath.Join(workshopDir(cfg), item.Name())
			// Standard layout: <item>/mods/<ModName>/
			if sub, err := os.ReadDir(filepath.Join(itemPath, "mods")); err == nil {
				for _, d := range sub {
					if d.IsDir() && hasModInfo(filepath.Join(itemPath, "mods", d.Name())) {
						add(d.Name(), "workshop "+item.Name())
					}
				}
			}
			// Legacy layout: <item>/<ModName>/
			if sub, err := os.ReadDir(itemPath); err == nil {
				for _, d := range sub {
					if d.IsDir() && d.Name() != "mods" && hasModInfo(filepath.Join(itemPath, d.Name())) {
						add(d.Name(), "workshop "+item.Name())
					}
				}
			}
		}
	}

	// Manual mods dropped into <DATA_DIR>/Workshop/
	manualDir := filepath.Join(cfg.DataDir, "Workshop")
	if sub, err := os.ReadDir(manualDir); err == nil {
		for _, d := range sub {
			if d.IsDir() && hasModInfo(filepath.Join(manualDir, d.Name())) {
				add(d.Name(), "manual")
			}
		}
	}

	return mods
}

func hasModInfo(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "mod.info"))
	return err == nil
}

// DiscoverModNames returns the sorted names of all mods found on disk,
// logging where each one was found.
func DiscoverModNames(cfg *config.ServerConfig) []string {
	mods := scanModFolders(cfg)

	names := make([]string, 0, len(mods))
	for name, source := range mods {
		fmt.Printf("Discovered mod %q (%s)\n", name, source)
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// WarnMissingMods logs a warning for every configured MOD_NAMES entry that
// has no matching mod folder on disk.
func WarnMissingMods(cfg *config.ServerConfig) {
	if cfg.ModNames == "" {
		return
	}
	onDisk := scanModFolders(cfg)
	for _, name := range strings.Split(cfg.ModNames, ";") {
		if _, ok := onDisk[name]; !ok {
			fmt.Printf("WARNING: MOD_NAMES entry %q has no mod folder on disk - check the spelling (mod folder names differ from workshop titles)\n", name)
		}
	}
}
