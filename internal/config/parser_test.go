package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultConfigDefaults(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.ServerName != "servertest" {
		t.Errorf("ServerName = %q, want servertest", cfg.ServerName)
	}
	if cfg.MaxPlayers != 16 {
		t.Errorf("MaxPlayers = %d, want 16", cfg.MaxPlayers)
	}
	if cfg.MaxRam != "4096m" {
		t.Errorf("MaxRam = %q, want 4096m", cfg.MaxRam)
	}
	if len(cfg.SandboxVars) != 0 {
		t.Errorf("SandboxVars = %v, want empty", cfg.SandboxVars)
	}
}

func TestDefaultConfigFromEnv(t *testing.T) {
	t.Setenv("SERVER_NAME", "myworld")
	t.Setenv("MAX_PLAYERS", "32")
	t.Setenv("PVP", "false")
	t.Setenv("USE_STEAM", "no")
	t.Setenv("BACKUP_INTERVAL", "not-a-number")

	cfg := DefaultConfig()
	if cfg.ServerName != "myworld" {
		t.Errorf("ServerName = %q, want myworld", cfg.ServerName)
	}
	if cfg.MaxPlayers != 32 {
		t.Errorf("MaxPlayers = %d, want 32", cfg.MaxPlayers)
	}
	if cfg.PvP {
		t.Error("PvP = true, want false")
	}
	if cfg.UseSteam {
		t.Error("UseSteam = true, want false")
	}
	// Invalid integer falls back to the default with a warning.
	if cfg.BackupInterval != 360 {
		t.Errorf("BackupInterval = %d, want fallback 360", cfg.BackupInterval)
	}
}

func TestValidate(t *testing.T) {
	valid := DefaultConfig()
	if errs := valid.Validate(); len(errs) != 0 {
		t.Errorf("valid config errors: %v", errs)
	}

	cases := []struct {
		name   string
		mutate func(*ServerConfig)
		want   string
	}{
		{"empty server name", func(c *ServerConfig) { c.ServerName = "" }, "SERVER_NAME"},
		{"port too high", func(c *ServerConfig) { c.RCONPort = 70000 }, "RCON_PORT"},
		{"port collision", func(c *ServerConfig) { c.UDPPort = c.DefaultPort }, "distinct"},
		{"zero players", func(c *ServerConfig) { c.MaxPlayers = 0 }, "MAX_PLAYERS"},
		{"backup interval zero", func(c *ServerConfig) { c.BackupInterval = 0 }, "BACKUP_INTERVAL"},
		{"auto update interval zero", func(c *ServerConfig) { c.AutoUpdateInterval = 0 }, "MOD_AUTO_UPDATE_INTERVAL"},
		{"auto update wait max zero", func(c *ServerConfig) { c.AutoUpdateWaitMax = 0 }, "MOD_AUTO_UPDATE_WAIT_MAX"},
		{"empty ram", func(c *ServerConfig) { c.MaxRam = "" }, "MAX_RAM"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tc.mutate(cfg)
			errs := cfg.Validate()
			if len(errs) == 0 {
				t.Fatal("expected validation error, got none")
			}
			if !strings.Contains(strings.Join(errs, ";"), tc.want) {
				t.Errorf("errors = %v, want containing %q", errs, tc.want)
			}
		})
	}
}

func TestEnsurePasswordsGeneratesAndPersists(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultConfig()
	cfg.DataDir = dir

	if err := cfg.EnsurePasswords(); err != nil {
		t.Fatalf("EnsurePasswords: %v", err)
	}
	if len(cfg.RCONPassword) < 8 || len(cfg.AdminPassword) < 8 {
		t.Fatalf("passwords too short: rcon=%q admin=%q", cfg.RCONPassword, cfg.AdminPassword)
	}

	data, err := os.ReadFile(cfg.CredentialsPath())
	if err != nil {
		t.Fatalf("credentials file not written: %v", err)
	}
	if !strings.Contains(string(data), "RCON_PASSWORD="+cfg.RCONPassword) {
		t.Errorf("credentials file missing RCON password:\n%s", data)
	}

	// A second load reuses the persisted values instead of regenerating.
	cfg2 := DefaultConfig()
	cfg2.DataDir = dir
	if err := cfg2.EnsurePasswords(); err != nil {
		t.Fatalf("second EnsurePasswords: %v", err)
	}
	if cfg2.RCONPassword != cfg.RCONPassword || cfg2.AdminPassword != cfg.AdminPassword {
		t.Errorf("passwords regenerated: got %q/%q, want %q/%q",
			cfg2.RCONPassword, cfg2.AdminPassword, cfg.RCONPassword, cfg.AdminPassword)
	}
}

func TestEnsurePasswordsEnvWins(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ADMIN_PASSWORD", "env-admin-pass")

	cfg := DefaultConfig()
	cfg.DataDir = dir
	cfg.AdminPassword = os.Getenv("ADMIN_PASSWORD")
	if err := cfg.EnsurePasswords(); err != nil {
		t.Fatalf("EnsurePasswords: %v", err)
	}

	if cfg.AdminPassword != "env-admin-pass" {
		t.Errorf("AdminPassword = %q, want env value", cfg.AdminPassword)
	}

	// Only the unset RCON password is generated; the env one is not stored
	// in a way that overrides later loads.
	cfg2 := DefaultConfig()
	cfg2.DataDir = dir
	cfg2.AdminPassword = os.Getenv("ADMIN_PASSWORD")
	if err := cfg2.EnsurePasswords(); err != nil {
		t.Fatalf("second EnsurePasswords: %v", err)
	}
	if cfg2.RCONPassword != cfg.RCONPassword {
		t.Errorf("RCONPassword regenerated: %q vs %q", cfg2.RCONPassword, cfg.RCONPassword)
	}
}

func TestCredentialsFilePermissions(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultConfig()
	cfg.DataDir = dir
	if err := cfg.EnsurePasswords(); err != nil {
		t.Fatalf("EnsurePasswords: %v", err)
	}

	info, err := os.Stat(cfg.CredentialsPath())
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("credentials permissions = %o, want 600", info.Mode().Perm())
	}
}

func TestEnsurePasswordsCreatesServerDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "data")
	cfg := DefaultConfig()
	cfg.DataDir = dir
	if err := cfg.EnsurePasswords(); err != nil {
		t.Fatalf("EnsurePasswords: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "credentials.env")); err != nil {
		t.Errorf("credentials file not created: %v", err)
	}
}

func TestParseModWorkshopIDs(t *testing.T) {
	t.Setenv("MOD_WORKSHOP_IDS", "2160432461, 2685168362 ; 2503743612")
	t.Setenv("MOD_NAMES", "  ClaimNonResidential;MoreDescriptionForTraits , SkillRecoveryJournal ")
	t.Setenv("MOD_WORKSHOP_COLLECTION_IDS", "1111111111; 2222222222")

	cfg := DefaultConfig()
	if cfg.ModWorkshopIDs != "2160432461;2685168362;2503743612" {
		t.Errorf("ModWorkshopIDs = %q", cfg.ModWorkshopIDs)
	}
	if cfg.ModNames != "ClaimNonResidential;MoreDescriptionForTraits;SkillRecoveryJournal" {
		t.Errorf("ModNames = %q", cfg.ModNames)
	}
	if cfg.ModWorkshopCollection != "1111111111;2222222222" {
		t.Errorf("ModWorkshopCollection = %q", cfg.ModWorkshopCollection)
	}
}

func TestParseModWorkshopIDsDedupes(t *testing.T) {
	t.Setenv("MOD_WORKSHOP_IDS", "2160432461;2160432461;2503743612")
	cfg := DefaultConfig()
	if cfg.ModWorkshopIDs != "2160432461;2503743612" {
		t.Errorf("ModWorkshopIDs = %q, want deduped", cfg.ModWorkshopIDs)
	}
}

func TestParseModWorkshopIDsDropsInvalid(t *testing.T) {
	t.Setenv("MOD_WORKSHOP_IDS", "2160432461;not-a-number;2503743612")
	cfg := DefaultConfig()
	if cfg.ModWorkshopIDs != "2160432461;2503743612" {
		t.Errorf("ModWorkshopIDs = %q, want invalid entry dropped", cfg.ModWorkshopIDs)
	}
}

func TestParseList(t *testing.T) {
	got := ParseList("a; b ,c\td\n\ne")
	want := []string{"a", "b", "c", "d", "e"}
	if len(got) != len(want) {
		t.Fatalf("parseList = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("parseList[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	if got := ParseList(" ; , "); len(got) != 0 {
		t.Errorf("parseList of separators = %v, want empty", got)
	}
}

func TestParseListDedupes(t *testing.T) {
	got := ParseList("x;y;x")
	if len(got) != 2 || got[0] != "x" || got[1] != "y" {
		t.Errorf("parseList = %v, want [x y]", got)
	}
}

func TestCheckWritable(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultConfig()
	cfg.DataDir = dir
	cfg.ServerDir = filepath.Join(dir, "server")
	if errs := cfg.CheckWritable(); len(errs) != 0 {
		t.Fatalf("expected all writable, got: %v", errs)
	}
	if _, err := os.Stat(filepath.Join(dir, ".write-test")); !os.IsNotExist(err) {
		t.Error("probe file was not cleaned up")
	}
}

func TestCheckWritableReadOnly(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root, permission checks are bypassed")
	}
	parent := t.TempDir()
	ro := filepath.Join(parent, "ro")
	if err := os.Mkdir(ro, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(ro, 0555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(ro, 0755)

	cfg := DefaultConfig()
	cfg.DataDir = ro
	cfg.ServerDir = ro
	if errs := cfg.CheckWritable(); len(errs) != 2 {
		t.Fatalf("expected 2 permission errors, got: %v", errs)
	}
}

func TestLoadSandboxEnv(t *testing.T) {
	t.Setenv("SANDBOX_Zombies", "2")
	t.Setenv("SANDBOX_DayLength", "3")
	t.Setenv("UNRELATED", "ignored")

	cfg := DefaultConfig()
	if cfg.SandboxVars["Zombies"] != "2" {
		t.Errorf("SandboxVars[Zombies] = %q, want 2", cfg.SandboxVars["Zombies"])
	}
	if cfg.SandboxVars["DayLength"] != "3" {
		t.Errorf("SandboxVars[DayLength] = %q, want 3", cfg.SandboxVars["DayLength"])
	}
	if _, ok := cfg.SandboxVars["UNRELATED"]; ok {
		t.Error("non-SANDBOX_ env leaked into SandboxVars")
	}
}

func TestLoadSandboxEnvSkipsMode(t *testing.T) {
	t.Setenv("SANDBOX_MODE", "performance")
	t.Setenv("SANDBOX_Zombies", "4")

	cfg := DefaultConfig()
	if _, ok := cfg.SandboxVars["MODE"]; ok {
		t.Error("SANDBOX_MODE leaked into SandboxVars as a bogus MODE key")
	}
	if cfg.SandboxVars["Zombies"] != "4" {
		t.Errorf("SandboxVars[Zombies] = %q, want 4", cfg.SandboxVars["Zombies"])
	}
}

func TestValidateServerNameCharset(t *testing.T) {
	base := func(name string) *ServerConfig {
		cfg := DefaultConfig()
		cfg.ServerName = name
		return cfg
	}
	for _, ok := range []string{"servertest", "my-server_1", "A"} {
		if errs := base(ok).Validate(); len(errs) != 0 {
			t.Errorf("ServerName %q should validate, got %v", ok, errs)
		}
	}
	for _, bad := range []string{"../evil", "a/b", "a\\b", "a b", "a;b", "a..b", ""} {
		if errs := base(bad).Validate(); len(errs) == 0 {
			t.Errorf("ServerName %q should fail validation", bad)
		}
	}
}

func TestValidateBackupPathContained(t *testing.T) {
	dir := t.TempDir()

	cfg := DefaultConfig()
	cfg.DataDir = dir
	cfg.BackupPath = filepath.Join(dir, "backups")
	if errs := cfg.Validate(); len(errs) != 0 {
		t.Errorf("BACKUP_PATH inside DATA_DIR should validate, got %v", errs)
	}

	cfg.BackupPath = "/etc"
	if errs := cfg.Validate(); len(errs) == 0 {
		t.Error("BACKUP_PATH outside DATA_DIR should fail validation")
	}
}

func TestValidateUDPPortCap(t *testing.T) {
	cfg := DefaultConfig()
	cfg.UDPPort = 65535
	if errs := cfg.Validate(); len(errs) == 0 {
		t.Error("UDP_PORT=65535 should fail (SteamPort2 overflows)")
	}

	cfg.UDPPort = 65534
	for _, e := range cfg.Validate() {
		if strings.Contains(e, "UDP_PORT") {
			t.Errorf("UDP_PORT=65534 should pass the UDP range check, got: %s", e)
		}
	}
}
