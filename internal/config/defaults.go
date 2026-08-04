package config

import (
	"fmt"
	"os"
	"path/filepath"
)

type ServerConfig struct {
	ServerName            string
	PublicName            string
	PublicServer          bool
	ServerPassword        string
	MaxPlayers            int
	DefaultPort           int
	UDPPort               int
	RCONPort              int
	RCONPassword          string
	AdminPassword         string
	BindIP                string
	SteamVAC              bool
	UseSteam              bool
	PauseOnEmpty          bool
	AutosaveInterval      int
	MapNames              string
	PvP                   bool
	ModNames              string
	ModWorkshopIDs        string
	ModWorkshopCollection string
	ModUpdateOnStart      bool
	SteamAPIKey           string
	MaxRam                string
	MinRam                string
	GCConfig              string
	JvmExtraArgs          string
	UpdateOnStart         bool
	ServerBranch          string
	SteamAppID            string
	SteamUser             string
	SteamPass             string
	SteamGuardCode        string
	BackupEnabled         bool
	BackupInterval        int
	BackupMaxCount        int
	BackupPath            string
	DiscordURL            string
	DiscordStart          bool
	DiscordStop           bool
	DiscordCrash          bool
	TZ                    string
	ServerDir             string
	DataDir               string
	SandboxMode           string

	SandboxVars map[string]string
}

func DefaultConfig() *ServerConfig {
	c := &ServerConfig{
		ServerName:            envStr("SERVER_NAME", "servertest"),
		PublicName:            envStr("PUBLIC_NAME", "My PZ Server"),
		PublicServer:          envBool("PUBLIC_SERVER", true),
		ServerPassword:        envStr("SERVER_PASSWORD", ""),
		MaxPlayers:            envInt("MAX_PLAYERS", 16),
		DefaultPort:           envInt("DEFAULT_PORT", 16261),
		UDPPort:               envInt("UDP_PORT", 16262),
		RCONPort:              envInt("RCON_PORT", 27015),
		RCONPassword:          envStr("RCON_PASSWORD", ""),
		AdminPassword:         envStr("ADMIN_PASSWORD", ""),
		BindIP:                envStr("BIND_IP", "0.0.0.0"),
		SteamVAC:              envBool("STEAM_VAC", true),
		UseSteam:              envBool("USE_STEAM", true),
		PauseOnEmpty:          envBool("PAUSE_ON_EMPTY", true),
		AutosaveInterval:      envInt("AUTOSAVE_INTERVAL", 15),
		MapNames:              envStr("MAP_NAMES", "Muldraugh, KY"),
		PvP:                   envBool("PVP", true),
		ModNames:              envStr("MOD_NAMES", ""),
		ModWorkshopIDs:        envStr("MOD_WORKSHOP_IDS", ""),
		ModWorkshopCollection: envStr("MOD_WORKSHOP_COLLECTION_IDS", ""),
		ModUpdateOnStart:      envBool("MOD_UPDATE_ON_START", false),
		SteamAPIKey:           envStr("STEAM_API_KEY", ""),
		MaxRam:                envStr("MAX_RAM", "4096m"),
		MinRam:                envStr("MIN_RAM", "4096m"),
		GCConfig:              envStr("GC_CONFIG", "ZGC"),
		JvmExtraArgs:          envStr("JVM_EXTRA_ARGS", ""),
		UpdateOnStart:         envBool("UPDATE_ON_START", true),
		ServerBranch:          envStr("SERVER_BRANCH", ""),
		SteamAppID:            envStr("STEAM_APP_ID", "380870"),
		SteamUser:             envStr("STEAM_USER", ""),
		SteamPass:             envStr("STEAM_PASS", ""),
		SteamGuardCode:        envStr("STEAM_GUARD_CODE", ""),
		BackupEnabled:         envBool("BACKUP_ENABLED", false),
		BackupInterval:        envInt("BACKUP_INTERVAL", 360),
		BackupMaxCount:        envInt("BACKUP_MAX_COUNT", 24),
		BackupPath:            envStr("BACKUP_PATH", "/home/steam/Zomboid/backups"),
		DiscordURL:            envStr("DISCORD_WEBHOOK_URL", ""),
		DiscordStart:          envBool("DISCORD_NOTIFY_START", true),
		DiscordStop:           envBool("DISCORD_NOTIFY_STOP", true),
		DiscordCrash:          envBool("DISCORD_NOTIFY_CRASH", true),
		TZ:                    envStr("TZ", "UTC"),
		ServerDir:             envStr("SERVER_DIR", "/home/steam/pzserver"),
		DataDir:               envStr("DATA_DIR", "/home/steam/Zomboid"),
		SandboxMode:           envStr("SANDBOX_MODE", "apocalypse"),
		SandboxVars:           map[string]string{},
	}

	c.loadSandboxEnv()
	c.ParseModWorkshopIDs()

	return c
}

func (c *ServerConfig) ServerIniPath() string {
	return c.DataDir + "/Server/" + c.ServerName + ".ini"
}

func (c *ServerConfig) SandboxVarsPath() string {
	return c.DataDir + "/Server/" + c.ServerName + "_SandboxVars.lua"
}

func (c *ServerConfig) SavePath() string {
	return c.DataDir + "/Saves/Multiplayer/" + c.ServerName
}

// CheckWritable verifies the entrypoint can write to the data and server
// directories (bind-mounted host folders). The container runs as UID 1000;
// host folders created by root or Docker fail this probe.
func (c *ServerConfig) CheckWritable() []error {
	var errs []error
	for _, dir := range []string{c.DataDir, c.ServerDir} {
		if err := checkDirWritable(dir); err != nil {
			errs = append(errs, fmt.Errorf("%s is not writable by UID %d: %w", dir, os.Geteuid(), err))
		}
	}
	return errs
}

func checkDirWritable(dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	probe := filepath.Join(dir, ".write-test")
	f, err := os.OpenFile(probe, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	f.Close()
	return os.Remove(probe)
}

func envStr(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}
