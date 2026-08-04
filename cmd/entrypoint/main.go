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

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "healthcheck":
			runHealthcheck()
			return
		case "mods":
			runMods()
			return
		}
	}

	cfg := config.DefaultConfig()

	if errs := cfg.Validate(); len(errs) > 0 {
		fmt.Println("Configuration errors:")
		for _, e := range errs {
			fmt.Printf("  - %s\n", e)
		}
		os.Exit(1)
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
		os.Exit(1)
	}

	if err := cfg.EnsurePasswords(); err != nil {
		fmt.Printf("ERROR resolving credentials: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Starting Project Zomboid server: %s\n", cfg.PublicName)
	fmt.Printf("Server name: %s\n", cfg.ServerName)
	fmt.Printf("Passwords (auto-generated unless set in .env) are stored in: %s\n", cfg.CredentialsPath())

	// Health server starts early so Docker can observe the install phase.
	srv := server.NewManager(cfg)
	healthSrv := health.NewServer(srv)
	healthSrv.SetStatus("installing")
	go func() {
		if err := healthSrv.ListenAndServe(8080); err != nil {
			fmt.Printf("Health server error: %v\n", err)
		}
	}()

	if err := steam.InstallOrUpdate(cfg); err != nil {
		fmt.Printf("ERROR installing/updating server: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Server files up to date")

	// Resolve collections, download workshop items, and derive mod folder
	// names before writing the ini so Mods= is populated automatically.
	modIDs := steam.ResolveModWorkshopIDs(cfg)
	if len(modIDs) > 0 {
		cfg.ModWorkshopIDs = strings.Join(modIDs, ";")
		if err := steam.DownloadWorkshopItems(cfg, modIDs); err != nil {
			fmt.Printf("ERROR downloading workshop mods: %v\n", err)
		}
	}

	if cfg.ModNames == "" {
		names := steam.DiscoverModNames(cfg)
		if len(names) > 0 {
			cfg.ModNames = strings.Join(names, ";")
			fmt.Printf("Auto-detected mods (MOD_NAMES): %s\n", cfg.ModNames)
		}
	} else {
		steam.WarnMissingMods(cfg)
	}

	if err := cfg.WriteIni(); err != nil {
		fmt.Printf("ERROR writing server.ini: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Server configuration written")

	if err := cfg.WriteSandboxVars(); err != nil {
		fmt.Printf("ERROR writing SandboxVars.lua: %v\n", err)
		os.Exit(1)
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
	if err := srv.Start(); err != nil {
		fmt.Printf("ERROR starting server: %v\n", err)
		discord.NotifyCrash(err)
		os.Exit(1)
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

	bk := backup.NewManager(cfg)
	bk.Scheduler(srv)

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
		os.Exit(0)
	}()

	healthSrv.SetStatus("healthy")

	// Block until the server exits on its own (crash) or shutdown completes.
	if err := srv.Wait(); err != nil {
		fmt.Printf("Server exited: %v\n", err)
		discord.NotifyCrash(err)
		os.Exit(1)
	}

	fmt.Println("Server exited cleanly")
}

func runHealthcheck() {
	cfg := config.DefaultConfig()
	if err := cfg.EnsurePasswords(); err != nil {
		fmt.Println("Healthcheck failed: cannot load credentials")
		os.Exit(1)
	}

	client := server.NewRCONClient(cfg)
	if err := client.Connect(); err != nil {
		fmt.Println("Healthcheck failed: RCON connection error")
		os.Exit(1)
	}
	defer client.Close()

	if err := client.Ping(); err != nil {
		fmt.Println("Healthcheck failed: RCON ping error")
		os.Exit(1)
	}

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
