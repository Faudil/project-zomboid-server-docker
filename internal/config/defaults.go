package config

import "os"

type ServerConfig struct {
	ServerName       string
	PublicName       string
	PublicServer     bool
	ServerPassword   string
	MaxPlayers       int
	DefaultPort      int
	UDPPort          int
	RCONPort         int
	RCONPassword     string
	AdminPassword    string
	BindIP           string
	SteamVAC         bool
	UseSteam         bool
	PauseOnEmpty     bool
	AutosaveInterval int
	MapNames         string
	PvP              bool
	ModNames         string
	ModWorkshopIDs   string
	MaxRam           string
	MinRam           string
	GCConfig         string
	JvmExtraArgs     string
	UpdateOnStart    bool
	ServerBranch     string
	SteamAppID       string
	BackupEnabled    bool
	BackupInterval   int
	BackupMaxCount   int
	BackupPath       string
	DiscordURL       string
	DiscordStart     bool
	DiscordStop      bool
	DiscordCrash     bool
	DiscordPlayers   bool
	TZ               string
	PUID             int
	PGID             int
	ServerDir        string
	DataDir          string
	GameVersion      string

	SandboxVars map[string]string
}

func DefaultConfig() *ServerConfig {
	return &ServerConfig{
		ServerName:       envStr("SERVER_NAME", "servertest"),
		PublicName:       envStr("PUBLIC_NAME", "My PZ Server"),
		PublicServer:     envBool("PUBLIC_SERVER", true),
		ServerPassword:   envStr("SERVER_PASSWORD", ""),
		MaxPlayers:       envInt("MAX_PLAYERS", 16),
		DefaultPort:      envInt("DEFAULT_PORT", 16261),
		UDPPort:          envInt("UDP_PORT", 16262),
		RCONPort:         envInt("RCON_PORT", 27015),
		RCONPassword:     envStr("RCON_PASSWORD", generatePassword()),
		AdminPassword:    envStr("ADMIN_PASSWORD", generatePassword()),
		BindIP:           envStr("BIND_IP", "0.0.0.0"),
		SteamVAC:         envBool("STEAM_VAC", true),
		UseSteam:         envBool("USE_STEAM", true),
		PauseOnEmpty:     envBool("PAUSE_ON_EMPTY", true),
		AutosaveInterval: envInt("AUTOSAVE_INTERVAL", 15),
		MapNames:         envStr("MAP_NAMES", "Muldraugh, KY"),
		PvP:              envBool("PVP", true),
		ModNames:         envStr("MOD_NAMES", ""),
		ModWorkshopIDs:   envStr("MOD_WORKSHOP_IDS", ""),
		MaxRam:           envStr("MAX_RAM", "4096m"),
		MinRam:           envStr("MIN_RAM", "4096m"),
		GCConfig:         envStr("GC_CONFIG", "ZGC"),
		JvmExtraArgs:     envStr("JVM_EXTRA_ARGS", ""),
		UpdateOnStart:    envBool("UPDATE_ON_START", true),
		ServerBranch:     envStr("SERVER_BRANCH", ""),
		SteamAppID:       envStr("STEAM_APP_ID", "380870"),
		BackupEnabled:    envBool("BACKUP_ENABLED", false),
		BackupInterval:   envInt("BACKUP_INTERVAL", 360),
		BackupMaxCount:   envInt("BACKUP_MAX_COUNT", 24),
		BackupPath:       envStr("BACKUP_PATH", "/home/steam/Zomboid/backups"),
		DiscordURL:       envStr("DISCORD_WEBHOOK_URL", ""),
		DiscordStart:     envBool("DISCORD_NOTIFY_START", true),
		DiscordStop:      envBool("DISCORD_NOTIFY_STOP", true),
		DiscordCrash:     envBool("DISCORD_NOTIFY_CRASH", true),
		DiscordPlayers:   envBool("DISCORD_NOTIFY_PLAYERS", false),
		TZ:               envStr("TZ", "UTC"),
		PUID:             envInt("PUID", 1000),
		PGID:             envInt("PGID", 1000),
		ServerDir:        envStr("SERVER_DIR", "/home/steam/pzserver"),
		DataDir:          envStr("DATA_DIR", "/home/steam/Zomboid"),
		GameVersion:      envStr("GAME_VERSION", ""),
		SandboxVars:      map[string]string{},
	}

}

func (c *ServerConfig) ServerIniPath() string {
	return c.DataDir + "/Server/" + c.ServerName + ".ini"
}

func (c *ServerConfig) SandboxVarsPath() string {
	return c.DataDir + "/Server/" + c.ServerName + "_SandboxVars.lua"
}

func (c *ServerConfig) SpawnRegionsPath() string {
	return c.DataDir + "/Server/" + c.ServerName + "_spawnregions.lua"
}

func (c *ServerConfig) SpawnPointsPath() string {
	return c.DataDir + "/Server/" + c.ServerName + "_spawnpoints.lua"
}

func (c *ServerConfig) SavePath() string {
	return c.DataDir + "/Saves/Multiplayer/" + c.ServerName
}

func envStr(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}
