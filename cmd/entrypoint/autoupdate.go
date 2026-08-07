package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/faudil/project-zomboid-server-docker/internal/config"
	"github.com/faudil/project-zomboid-server-docker/internal/server"
	"github.com/faudil/project-zomboid-server-docker/internal/steam"
	"github.com/faudil/project-zomboid-server-docker/internal/webhook"
)

// backupRunner is the subset of *backup.Manager the auto-updater needs, so
// tests can substitute a fake.
type backupRunner interface {
	Run()
}

// Seams for tests, following the package-level override pattern used across
// this codebase.
var (
	checkForUpdates = steam.CheckForUpdates

	rconBroadcast = func(cfg *config.ServerConfig, msg string) error {
		client := server.NewRCONClient(cfg)
		if err := client.Connect(); err != nil {
			return err
		}
		defer client.Close()
		return client.Broadcast(msg)
	}

	rconPlayerCount = func(cfg *config.ServerConfig) (int, error) {
		client := server.NewRCONClient(cfg)
		if err := client.Connect(); err != nil {
			return 0, err
		}
		defer client.Close()
		return client.PlayerCount()
	}

	// Time knobs so tests can run without waiting real minutes.
	now              = time.Now
	sleep            = time.Sleep
	emptyPoll        = time.Minute
	emptyNoticeEvery = 5 * time.Minute
	announceStep     = time.Minute
	// exitProcess is the process exit path; the container's restart policy
	// picks the boot flow up again, which applies the updates.
	exitProcess = os.Exit
)

// runAutoUpdater polls for workshop mod and game build updates while the
// server runs. On an update it announces the restart, optionally waits for
// the server to empty, then stops it cleanly and exits 0 so Docker's restart
// policy re-runs the boot flow, which downloads and loads the new versions.
func runAutoUpdater(cfg *config.ServerConfig, modIDs []string, srv serverRunner, bk backupRunner, discord *webhook.DiscordWebhook) {
	interval := time.Duration(cfg.AutoUpdateInterval) * time.Minute
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	fmt.Printf("Auto-update enabled: checking for mod/game updates every %d minute(s)\n", cfg.AutoUpdateInterval)
	for range ticker.C {
		updatedMods, gameUpdated, err := checkForUpdates(cfg, modIDs)
		if err != nil {
			fmt.Printf("WARNING: update check failed: %v\n", err)
			continue
		}
		if len(updatedMods) == 0 && !gameUpdated {
			continue
		}
		restartForUpdates(cfg, updatedMods, gameUpdated, srv, bk, discord)
	}
}

// restartForUpdates performs the graceful restart cycle: notify, optionally
// wait for the server to empty, warn players, save+quit, final backup, exit.
func restartForUpdates(cfg *config.ServerConfig, updatedMods []string, gameUpdated bool, srv serverRunner, bk backupRunner, discord *webhook.DiscordWebhook) {
	reason := updateReason(updatedMods, gameUpdated)
	fmt.Printf("Updates detected (%s), restarting server to apply them\n", reason)

	if discord != nil {
		discord.NotifyUpdate(updatedMods, gameUpdated)
	}

	if cfg.AutoUpdateWaitEmpty {
		waitForEmpty(cfg, reason)
	}
	if cfg.AutoUpdateAnnounce > 0 {
		announceRestart(cfg, reason)
	}

	fmt.Println("Stopping server to apply updates")
	if err := srv.Stop(); err != nil {
		// RCON is likely wedged (PZ stops answering RCON exec commands while
		// the server is paused/empty). Manager.Stop has already force-
		// terminated the process; the world is as of the last autosave
		// (AUTOSAVE_INTERVAL). Restart anyway so the update is applied -
		// better than a stuck container or an update that never lands.
		fmt.Printf("WARNING: graceful shutdown failed during auto-update (%v); world data is as of the last autosave (AUTOSAVE_INTERVAL=%d minutes). Continuing with the update.\n", err, cfg.AutosaveInterval)
	}
	bk.Run() // final backup against the saved state before the update is applied
	fmt.Println("Auto-update complete, exiting for container restart")
	exitProcess(0)
}

// waitForEmpty polls the player count until nobody is online, the maximum
// wait elapses, or RCON becomes unreachable (a broken server should be
// restarted, not waited on). Repeats the in-game notice periodically so
// players know why the restart has not happened yet.
func waitForEmpty(cfg *config.ServerConfig, reason string) {
	deadline := now().Add(time.Duration(cfg.AutoUpdateWaitMax) * time.Hour)
	lastNotice := now().Add(-emptyNoticeEvery)
	errStreak := 0

	for {
		count, err := rconPlayerCount(cfg)
		if err != nil {
			errStreak++
			fmt.Printf("WARNING: player count query failed (%d consecutive): %v\n", errStreak, err)
			if errStreak >= 3 {
				fmt.Println("RCON unreachable; restarting without waiting for an empty server")
				return
			}
		} else {
			errStreak = 0
			if count == 0 {
				fmt.Println("Server empty - proceeding with restart")
				return
			}
			fmt.Printf("%d player(s) still online, waiting for the server to empty\n", count)
		}

		if now().After(deadline) {
			fmt.Printf("Server never emptied within %d hour(s); restarting anyway\n", cfg.AutoUpdateWaitMax)
			return
		}

		if time.Since(lastNotice) >= emptyNoticeEvery {
			lastNotice = time.Now()
			msg := fmt.Sprintf("Server restarting to apply updates (%s). Please log off - the restart happens once the server is empty.", reason)
			_ = rconBroadcast(cfg, msg)
		}
		sleep(emptyPoll)
	}
}

// announceRestart broadcasts a countdown so players can log off in time.
func announceRestart(cfg *config.ServerConfig, reason string) {
	total := cfg.AutoUpdateAnnounce
	fmt.Printf("Announcing restart in %d minute(s)\n", total)
	for remaining := total; remaining > 0; remaining-- {
		msg := fmt.Sprintf("Server restarting in %d minute(s) to apply updates (%s). Please log off.", remaining, reason)
		_ = rconBroadcast(cfg, msg)
		sleep(announceStep)
	}
}

// updateReason builds a human-readable summary of what changed.
func updateReason(updatedMods []string, gameUpdated bool) string {
	var parts []string
	if gameUpdated {
		parts = append(parts, "a new game build")
	}
	if len(updatedMods) > 0 {
		parts = append(parts, fmt.Sprintf("workshop mod update(s): %s", strings.Join(updatedMods, ", ")))
	}
	if len(parts) == 0 {
		return "unknown change"
	}
	return strings.Join(parts, " and ")
}
