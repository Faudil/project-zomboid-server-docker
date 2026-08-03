package steam

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/faudil/project-zomboid-server-docker/internal/config"
)

const steamcmdPath = "/home/steam/steamcmd/steamcmd.sh"

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

func DownloadWorkshopItems(cfg *config.ServerConfig) error {
	if cfg.ModWorkshopIDs == "" {
		return nil
	}

	ids := strings.Split(cfg.ModWorkshopIDs, ";")
	workshopDir := cfg.ServerDir + "/steamapps/workshop/content/108600"

	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}

		modPath := workshopDir + "/" + id
		if _, err := os.Stat(modPath); err == nil {
			fmt.Printf("Workshop mod %s already downloaded\n", id)
			continue
		}

		args := []string{
			"+force_install_dir", cfg.ServerDir,
			"+login", "anonymous",
			fmt.Sprintf("workshop_download_item 108600 %s", id),
			"+quit",
		}

		if err := runSteamCmd(args...); err != nil {
			return fmt.Errorf("downloading workshop mod %s: %w", id, err)
		}
		fmt.Printf("Downloaded workshop mod %s\n", id)
	}

	return nil
}
