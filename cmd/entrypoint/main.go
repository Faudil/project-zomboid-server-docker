package main

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/faudil/project-zomboid-server-docker/internal/backup"
	"github.com/faudil/project-zomboid-server-docker/internal/config"
	"github.com/faudil/project-zomboid-server-docker/internal/health"
	"github.com/faudil/project-zomboid-server-docker/internal/server"
	"github.com/faudil/project-zomboid-server-docker/internal/steam"
	"github.com/faudil/project-zomboid-server-docker/internal/webhook"
)

// version is injected at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "healthcheck":
			runHealthcheck()
			return
		case "mods":
			runMods()
			return
		case "--version", "-version":
			fmt.Printf("project-zomboid-server-docker entrypoint %s\n", version)
			return
		}
	}

	if err := run(); err != nil {
		fmt.Printf("ERROR: %v\n", err)
		os.Exit(1)
	}
}

// serverRunner is the subset of *server.Manager the orchestration needs, so
// tests can substitute a fake.
type serverRunner interface {
	Start() error
	Wait() error
	Stop() error
}

// Seams for tests, following the package-level override pattern used across
// this codebase.
var (
	installOrUpdate    = steam.InstallOrUpdate
	resolveModWorkshop = steam.ResolveModWorkshopIDs
	downloadWorkshop   = steam.DownloadWorkshopItems
	discoverModNames   = steam.DiscoverModNames
	warnMissingMods    = steam.WarnMissingMods
	newServerManager   = server.NewManager
	newBackupManager   = backup.NewManager
)

func run() error {
	cfg := config.DefaultConfig()

	if errs := cfg.Validate(); len(errs) > 0 {
		fmt.Println("Configuration errors:")
		for _, e := range errs {
			fmt.Printf("  - %s\n", e)
		}
		return fmt.Errorf("configuration validation failed")
	}

	if errs := cfg.CheckWritable(); len(errs) > 0 {
		fmt.Println("Permission errors - the container cannot write to its volumes:")
		for _, e := range errs {
			fmt.Printf("  - %v\n", e)
		}
		fmt.Println()
		fmt.Println("The container runs as UID 1000 (steam user). The host directories")
		fmt.Println("mounted into the container must be writable by UID 1000. From the")
		fmt.Println("directory containing your docker-compose.yml, run:")
		fmt.Println()
		fmt.Println("  sudo chown -R 1000:1000 data server-files backups")
		fmt.Println()
		fmt.Println("then restart the container.")
		return fmt.Errorf("volumes are not writable")
	}

	if err := cfg.EnsurePasswords(); err != nil {
		return fmt.Errorf("resolving credentials: %w", err)
	}

	fmt.Printf("Starting Project Zomboid server: %s\n", cfg.PublicName)
	fmt.Printf("Server name: %s\n", cfg.ServerName)
	fmt.Printf("Passwords (auto-generated unless set in .env) are stored in: %s\n", cfg.CredentialsPath())

	// Health server starts early so Docker can observe the install phase.
	healthSrv := health.NewServer()
	healthSrv.SetStatus("installing")
	go func() {
		if err := healthSrv.ListenAndServe(8080); err != nil {
			fmt.Printf("Health server error: %v\n", err)
		}
	}()

	if err := installOrUpdate(cfg); err != nil {
		return fmt.Errorf("installing/updating server: %w", err)
	}
	fmt.Println("Server files up to date")

	// Resolve collections, download workshop items, and derive mod folder
	// names before writing the ini so Mods= is populated automatically.
	modIDs := resolveModWorkshop(cfg)
	if len(modIDs) > 0 {
		cfg.ModWorkshopIDs = strings.Join(modIDs, ";")
		// With auto-update enabled every start re-checks the workshop items
		// so a restart triggered by an update actually applies it. Re-download
		// is incremental: unchanged items are skipped. Anonymous setups skip
		// the steamcmd pre-download anyway and the PZ server refreshes the
		// items itself at startup.
		if cfg.AutoUpdate && cfg.SteamUser != "" {
			cfg.ModUpdateOnStart = true
		}
		if err := downloadWorkshop(cfg, modIDs); err != nil {
			fmt.Printf("ERROR downloading workshop mods: %v\n", err)
		}
	}

	if cfg.ModNames == "" {
		names := discoverModNames(cfg)
		if len(names) > 0 {
			cfg.ModNames = strings.Join(names, ";")
			fmt.Printf("Auto-detected mods (MOD_NAMES): %s\n", cfg.ModNames)
		}
	} else {
		warnMissingMods(cfg)
	}

	if err := cfg.WriteIni(); err != nil {
		return fmt.Errorf("writing server.ini: %w", err)
	}
	fmt.Println("Server configuration written")

	if err := cfg.WriteSandboxVars(); err != nil {
		return fmt.Errorf("writing SandboxVars.lua: %w", err)
	}

	// The launcher passes vmArgs from ProjectZomboid64.json on the java
	// command line, which overrides _JAVA_OPTIONS. Patch it so MAX_RAM,
	// MIN_RAM, GC_CONFIG and JVM_EXTRA_ARGS actually take effect.
	if err := cfg.PatchLauncherJson(); err != nil {
		fmt.Printf("WARNING: could not patch ProjectZomboid64.json: %v\n", err)
	} else {
		fmt.Printf("JVM settings patched into ProjectZomboid64.json (heap %s, GC %s)\n", cfg.MaxRam, cfg.GCConfig)
	}

	discord := webhook.NewDiscord(cfg)
	discord.NotifyStart()

	healthSrv.SetStatus("starting")
	srv := newServerManager(cfg)
	if err := srv.Start(); err != nil {
		discord.NotifyCrash(err)
		return fmt.Errorf("starting server: %w", err)
	}

	// First boot with anonymous Steam: steamcmd cannot download workshop items
	// (Steam rejects anonymous downloads), but the running server downloads
	// them itself from WorkshopItems=. Wait for the downloads, and restart
	// whenever the on-disk mod set grew since this boot started, so Mods= is
	// regenerated and the new mods load. Converges within a couple of restarts.
	bootModCount := steam.ModCountOnDisk(cfg)
	if len(modIDs) > 0 && cfg.SteamUser == "" && cfg.UseSteam {
		go func() {
			if steam.WaitForModDownloads(cfg, modIDs) && steam.ModCountOnDisk(cfg) > bootModCount {
				fmt.Println("Workshop mods downloaded by the server; restarting once to load them")
				_ = syscall.Kill(os.Getpid(), syscall.SIGTERM)
			}
		}()
	}

	bk := newBackupManager(cfg)
	bk.Scheduler()

	// Single owner of signal handling: the shutdown path below. Manager.Wait()
	// only waits for the server process to exit.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		sig := <-sigCh
		fmt.Printf("Received %v, shutting down...\n", sig)
		healthSrv.SetStatus("stopping")
		discord.NotifyStop()

		// A second signal forces immediate exit.
		go func() {
			sig := <-sigCh
			fmt.Printf("Received second %v, forcing exit\n", sig)
			os.Exit(1)
		}()

		// Stop the server first (RCON save + quit) so the world is flushed,
		// then run the final backup against the saved state.
		if err := srv.Stop(); err != nil {
			fmt.Printf("Server shutdown failed: %v\n", err)
			os.Exit(1)
		}
		bk.Run() // final backup
		healthSrv.Shutdown()
		os.Exit(0)
	}()

	healthSrv.SetStatus("healthy")

	// Watch for workshop mod and game build updates. On an update the server
	// is stopped cleanly and the container exits for its restart policy to
	// re-run this boot flow, which downloads and loads the new versions.
	if cfg.AutoUpdate {
		go runAutoUpdater(cfg, modIDs, srv, bk, discord)
	}

	// Block until the server exits on its own (crash) or shutdown completes.
	if err := srv.Wait(); err != nil {
		discord.NotifyCrash(err)
		healthSrv.Shutdown()
		return fmt.Errorf("server exited: %w", err)
	}

	healthSrv.Shutdown()
	fmt.Println("Server exited cleanly")
	return nil
}

func runHealthcheck() {
	cfg := config.DefaultConfig()
	if err := cfg.EnsurePasswords(); err != nil {
		fmt.Printf("Healthcheck failed: %v\n", err)
		os.Exit(1)
	}

	client := server.NewRCONClient(cfg)
	// Auth-only, on purpose: PZ only answers RCON exec commands (like "hello")
	// from its main loop, which stalls while the server is paused or still
	// loading. Repeated exec commands then leave stuck RCON client threads
	// that fill PZ's 5-slot connection limit, after which every new
	// connection is silently dropped (unhealthy container with EOF errors).
	// A connect+auth round-trip proves the RCON responder is alive and the
	// password is correct without ever entering the exec queue.
	if err := client.Connect(); err != nil {
		fmt.Printf("Healthcheck failed: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	fmt.Println("Healthcheck OK")
}

// runMods lists the mods discovered on disk and flags MOD_NAMES entries that
// have no matching folder - useful for debugging load order and typos.
func runMods() {
	cfg := config.DefaultConfig()
	names := steam.DiscoverModNames(cfg)
	if len(names) == 0 {
		fmt.Println("No mods found on disk")
	}
	if cfg.ModNames != "" {
		fmt.Printf("Configured MOD_NAMES: %s\n", cfg.ModNames)
		steam.WarnMissingMods(cfg)
	}
}
