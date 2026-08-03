package steam

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/faudil/project-zomboid-server-docker/internal/config"
)

const steamcmdPath = "/home/steam/steamcmd/steamcmd.sh"

// depotDownloaderPath is the DepotDownloader binary installed in the image.
// It replaces SteamCMD's app_update for downloading the server files.
const depotDownloaderPath = "/usr/local/bin/depotdownloader"

// workshopAppID is the Steam Workshop app id for Project Zomboid.
const workshopAppID = "108600"

// collectionAPI is the Steam Web API endpoint used to resolve workshop
// collections. Overridable in tests.
var collectionAPI = "https://api.steampowered.com/ISteamRemoteStorage/GetCollectionDetails/v1/"

// runSteamCmdCapture runs steamcmd, streaming output to stdout while keeping
// a copy for failure detection. steamcmd exits 0 even when commands fail, so
// the captured output is the only reliable signal. Overridable in tests.
var runSteamCmdCapture = runSteamCmdCaptureImpl

func runSteamCmdCaptureImpl(args ...string) (string, error) {
	cmd := exec.Command(steamcmdPath, args...)
	cmd.Dir = filepath.Dir(steamcmdPath)
	cmd.Env = append(os.Environ(), "HOME=/home/steam")

	var buf bytes.Buffer
	out := io.MultiWriter(os.Stdout, &buf)
	cmd.Stdout = out
	cmd.Stderr = out

	err := cmd.Run()
	return buf.String(), err
}

// runDepotDownloader runs DepotDownloader, streaming output to stdout while
// keeping a copy for failure detection. Unlike steamcmd it exits non-zero on
// failure, but the output still carries the useful error context.
// Overridable in tests.
var runDepotDownloader = runDepotDownloaderImpl

func runDepotDownloaderImpl(args ...string) (string, error) {
	cmd := exec.Command(depotDownloaderPath, args...)
	// The self-contained .NET build runs without ICU in globalization-invariant
	// mode, avoiding an extra ~200MB of locale packages in the image.
	cmd.Env = append(os.Environ(), "HOME=/home/steam", "DOTNET_SYSTEM_GLOBALIZATION_INVARIANT=1")

	var buf bytes.Buffer
	out := io.MultiWriter(os.Stdout, &buf)
	cmd.Stdout = out
	cmd.Stderr = out

	err := cmd.Run()
	return buf.String(), err
}

// steamLoginArgs builds the steamcmd login arguments. Real credentials are
// used when STEAM_USER is set; anonymous otherwise.
func steamLoginArgs(cfg *config.ServerConfig) []string {
	if cfg.SteamUser == "" {
		return []string{"+login", "anonymous"}
	}
	args := []string{}
	if cfg.SteamGuardCode != "" {
		args = append(args, "+set_steam_guard_code", cfg.SteamGuardCode)
	}
	return append(args, "+login", cfg.SteamUser, cfg.SteamPass)
}

func startScriptPath(cfg *config.ServerConfig) string {
	return cfg.ServerDir + "/start-server.sh"
}

// fixExecutableBits restores the execute permission on the server binaries.
// DepotDownloader extracts files without preserving the Steam depot's
// executable bits, so jre64/bin/java and ProjectZomboid64 land as mode 0644.
// Project Zomboid's start-server.sh only prints "Only 64bit is supported"
// when jre64/bin/java cannot be executed, so without this fix every start
// fails with that misleading message and exits 0.
func fixExecutableBits(cfg *config.ServerConfig) {
	files := []string{
		startScriptPath(cfg),
		filepath.Join(cfg.ServerDir, "ProjectZomboid64"),
	}

	// jre64/bin and linux64 contents are all executables or shared libraries;
	// shared libraries do not need +x, but applying it matches the depot.
	for _, dir := range []string{
		filepath.Join(cfg.ServerDir, "jre64", "bin"),
		filepath.Join(cfg.ServerDir, "linux64"),
	} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				files = append(files, filepath.Join(dir, e.Name()))
			}
		}
	}

	for _, f := range files {
		if err := os.Chmod(f, 0755); err != nil && !os.IsNotExist(err) {
			fmt.Printf("WARNING: could not set executable bit on %s: %v\n", f, err)
		}
	}
}

const maxUpdateAttempts = 6

var updateRetryDelay = 60 * time.Second

// installArgs builds the DepotDownloader invocation for the server files.
// Anonymous by default; STEAM_USER/STEAM_PASS are passed through when set.
func installArgs(cfg *config.ServerConfig) []string {
	args := []string{
		"-app", cfg.SteamAppID,
		"-dir", cfg.ServerDir,
		"-validate",
	}
	if cfg.ServerBranch != "" {
		args = append(args, "-branch", cfg.ServerBranch)
	}
	if cfg.SteamUser != "" {
		args = append(args, "-username", cfg.SteamUser, "-password", cfg.SteamPass)
	}
	return args
}

// depotPermanentFailure reports whether DepotDownloader output describes a
// problem retrying cannot fix (e.g. bad credentials).
func depotPermanentFailure(output string) bool {
	lower := strings.ToLower(output)
	for _, marker := range []string{
		"password was incorrect",
		"invalid password",
		"steam guard",
		"two-factor",
		"not valid for this account",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// InstallOrUpdate downloads the dedicated server files with DepotDownloader,
// which uses the same Steam3 protocol as steamcmd but downloads anonymously
// reliably (steamcmd's app_update is intermittently rejected by Steam's
// backend). Retries with a backoff as a safety net; permanent failures such
// as bad credentials fail immediately.
func InstallOrUpdate(cfg *config.ServerConfig) error {
	if !cfg.UpdateOnStart {
		if _, err := os.Stat(startScriptPath(cfg)); err == nil {
			fixExecutableBits(cfg)
			return nil
		}
	}

	if cfg.SteamUser != "" && cfg.SteamPass == "" {
		return fmt.Errorf("STEAM_PASS is required when STEAM_USER is set (DepotDownloader would otherwise prompt for a password and hang)")
	}

	args := installArgs(cfg)

	for attempt := 1; attempt <= maxUpdateAttempts; attempt++ {
		if attempt > 1 {
			fmt.Printf("Server download failed, retrying in 60s (attempt %d/%d)...\n", attempt, maxUpdateAttempts)
			time.Sleep(updateRetryDelay)
		}

		output, err := runInstallAttempt(cfg, args)
		if err == nil {
			fixExecutableBits(cfg)
			return nil
		}
		fmt.Printf("Server download attempt %d/%d failed: %v\n", attempt, maxUpdateAttempts, err)

		// Don't burn the remaining attempts on a problem retrying cannot fix.
		if depotPermanentFailure(output) {
			return err
		}
	}

	return fmt.Errorf("could not download app %s after %d attempts", cfg.SteamAppID, maxUpdateAttempts)
}

func runInstallAttempt(cfg *config.ServerConfig, args []string) (string, error) {
	output, err := runDepotDownloader(args...)
	if err != nil {
		return output, fmt.Errorf("DepotDownloader failed: %w", err)
	}

	if _, err := os.Stat(startScriptPath(cfg)); err != nil {
		return output, fmt.Errorf("server files were not installed (start-server.sh missing)")
	}

	return output, nil
}

// steamFailure returns the first steamcmd error line found in the output,
// or an empty string when the run looks successful.
func steamFailure(output string) string {
	for _, line := range strings.Split(output, "\n") {
		lower := strings.ToLower(line)
		for _, marker := range []string{
			"failed to install app",
			"download item",
			"no subscription",
			"missing file permissions",
			"missing configuration",
			"access denied",
			"invalid password",
			"two-factor code required",
			"steam guard code is incorrect",
			"password incorrect",
		} {
			if strings.Contains(lower, marker) {
				return strings.TrimSpace(line)
			}
		}
	}
	return ""
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

// collectionPageURL is the public Steam community page of a workshop
// collection. It is scraped when no STEAM_API_KEY is configured; the Web API
// (collectionAPI) needs a key but the page does not. Overridable in tests.
var collectionPageURL = "https://steamcommunity.com/sharedfiles/filedetails/?id="

// resolveCollection fetches the item IDs of a Steam workshop collection.
// With a STEAM_API_KEY the Web API is used; without one the public collection
// page is scraped instead, so collections work with no key at all.
func resolveCollection(cfg *config.ServerConfig, collectionID string) ([]string, error) {
	if cfg.SteamAPIKey != "" {
		return resolveCollectionAPI(cfg, collectionID)
	}
	return resolveCollectionKeyless(cfg, collectionID)
}

// resolveCollectionAPI resolves a collection through the Steam Web API.
// GetCollectionDetails requires an API key: without one Steam returns 400.
func resolveCollectionAPI(cfg *config.ServerConfig, collectionID string) ([]string, error) {
	form := url.Values{}
	form.Set("collectioncount", "1")
	form.Set("publishedfileids", collectionID)

	req, err := http.NewRequest(http.MethodPost, collectionAPI, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	q := req.URL.Query()
	q.Set("key", cfg.SteamAPIKey)
	req.URL.RawQuery = q.Encode()

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

// resolveCollectionKeyless scrapes the public Steam community page of a
// collection. The item list is server-rendered into a collectionChildren
// block, so no API key is needed. Returns an error for private, empty or
// restructured pages.
func resolveCollectionKeyless(cfg *config.ServerConfig, collectionID string) ([]string, error) {
	req, err := http.NewRequest(http.MethodGet, collectionPageURL+collectionID, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64)")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("collection page returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading collection page: %w", err)
	}

	ids, err := parseCollectionPage(string(body))
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("collection %s is empty or private", collectionID)
	}
	return ids, nil
}

// parseCollectionPage extracts the item IDs of a workshop collection from its
// community page HTML: the links inside the collectionChildren block, deduped
// while preserving order.
func parseCollectionPage(page string) ([]string, error) {
	start := strings.Index(page, `<div class="collectionChildren">`)
	if start == -1 {
		return nil, fmt.Errorf("collection page has no collectionChildren section")
	}
	end := strings.Index(page[start:], `<div style="clear: left">`)
	if end == -1 {
		return nil, fmt.Errorf("collection page children section is truncated")
	}
	section := page[start : start+end]

	re := regexp.MustCompile(`sharedfiles/filedetails/\?id=(\d+)`)
	matches := re.FindAllStringSubmatch(section, -1)
	if len(matches) == 0 {
		return nil, fmt.Errorf("collection page children section has no items")
	}

	seen := make(map[string]struct{}, len(matches))
	ids := make([]string, 0, len(matches))
	for _, m := range matches {
		id := m[1]
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
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

	args := []string{
		"+@NoPromptForPassword", "1",
		"+force_install_dir", cfg.ServerDir,
	}
	args = append(args, steamLoginArgs(cfg)...)
	args = append(args, workshopBatchArgs(cfg, toDownload)...)

	// Anonymous downloads are intermittently rate-limited by Steam; retry the
	// batch once before giving up on this start. Output is captured because
	// steamcmd exits 0 even when a workshop item fails to download.
	for attempt := 1; attempt <= 2; attempt++ {
		output, err := runSteamCmdCapture(args...)
		if err != nil {
			if attempt == 2 {
				fmt.Printf("WARNING: workshop mod download failed: %v\n", err)
				return nil
			}
			fmt.Println("Workshop mod download failed (Steam intermittently rate-limits anonymous downloads), retrying in 60s...")
			time.Sleep(updateRetryDelay)
			continue
		}
		if msg := steamFailure(output); msg != "" {
			if attempt == 2 {
				fmt.Printf("WARNING: workshop mod download reported an error: %s\n", msg)
				return nil
			}
			fmt.Println("Workshop mod download reported an error, retrying in 60s...")
			time.Sleep(updateRetryDelay)
			continue
		}
		break
	}

	for _, id := range toDownload {
		if _, err := os.Stat(filepath.Join(dir, id)); err != nil {
			fmt.Printf("WARNING: workshop item %s did not download (private, region-locked, invalid ID, or Steam-side anonymous download failure). If it is a public mod, try STEAM_USER/STEAM_PASS\n", id)
		} else {
			fmt.Printf("Downloaded workshop mod %s\n", id)
		}
	}

	return nil
}

// workshopBatchArgs builds the workshop_download_item commands for a single
// steamcmd session.
func workshopBatchArgs(cfg *config.ServerConfig, ids []string) []string {
	args := []string{}
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
