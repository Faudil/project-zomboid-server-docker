package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/faudil/project-zomboid-server-docker/internal/backup"
	"github.com/faudil/project-zomboid-server-docker/internal/config"
	"github.com/faudil/project-zomboid-server-docker/internal/health"
	"github.com/faudil/project-zomboid-server-docker/internal/server"
	"github.com/faudil/project-zomboid-server-docker/internal/steam"
	"github.com/faudil/project-zomboid-server-docker/internal/webhook"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		runHealthcheck()
		return
	}

	cfg := config.DefaultConfig()

	if errs := cfg.Validate(); len(errs) > 0 {
		fmt.Println("Configuration errors:")
		for _, e := range errs {
			fmt.Printf("  - %s\n", e)
		}
		os.Exit(1)
	}

	fmt.Printf("Starting Project Zomboid server: %s\n", cfg.PublicName)
	fmt.Printf("Server name: %s\n", cfg.ServerName)
	fmt.Printf("RCON Password: %s\n", cfg.RCONPassword)
	fmt.Printf("Admin Password: %s\n", cfg.AdminPassword)

	if err := cfg.WriteIni(); err != nil {
		fmt.Printf("ERROR writing server.ini: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Server configuration written")

	if err := cfg.WriteSandboxVars(); err != nil {
		fmt.Printf("ERROR writing SandboxVars.lua: %v\n", err)
		os.Exit(1)
	}

	if err := steam.InstallOrUpdate(cfg); err != nil {
		fmt.Printf("ERROR installing/updating server: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Server files up to date")

	if err := steam.DownloadWorkshopItems(cfg); err != nil {
		fmt.Printf("ERROR downloading workshop mods: %v\n", err)
	}

	discord := webhook.NewDiscord(cfg)
	discord.NotifyStart()

	srv := server.NewManager(cfg)
	if err := srv.Start(); err != nil {
		fmt.Printf("ERROR starting server: %v\n", err)
		discord.NotifyCrash(err)
		os.Exit(1)
	}

	healthSrv := health.NewServer(srv)
	go func() {
		if err := healthSrv.ListenAndServe(8080); err != nil {
			fmt.Printf("Health server error: %v\n", err)
		}
	}()

	bk := backup.NewManager(cfg)
	bk.Scheduler(srv)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		sig := <-sigCh
		fmt.Printf("Received %v, shutting down...\n", sig)

		healthSrv.SetStatus("stopping")
		discord.NotifyStop()

		go func() {
			sig := <-sigCh
			fmt.Printf("Received second %v, forcing exit\n", sig)
			os.Exit(1)
		}()

		bk.Run() // final backup
		srv.Stop()
		os.Exit(0)
	}()

	healthSrv.SetStatus("healthy")

	if err := srv.Wait(); err != nil {
		fmt.Printf("Server exited: %v\n", err)
		discord.NotifyCrash(err)
		os.Exit(1)
	}
}

func runHealthcheck() {
	cfg := config.DefaultConfig()
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
