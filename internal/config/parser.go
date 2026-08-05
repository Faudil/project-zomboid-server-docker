package config

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// validServerNameRE restricts SERVER_NAME to a safe subset used to build
// file paths (ini, SandboxVars.lua, save dir, backup names). Anything else
// could traverse outside the data dir.
var validServerNameRE = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func envInt(key string, fallback int) int {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: %s=%q is not a valid integer, using default %d\n", key, v, fallback)
		return fallback
	}
	return i
}

func envBool(key string, fallback bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback
	}
	v = strings.ToLower(v)
	switch v {
	case "true", "1", "yes", "y", "on":
		return true
	case "false", "0", "no", "n", "off":
		return false
	default:
		fmt.Fprintf(os.Stderr, "WARNING: %s=%q is not a valid boolean, using default %v\n", key, v, fallback)
		return fallback
	}
}

func generatePassword() (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	length := 16
	result := make([]byte, length)
	for i := range result {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", fmt.Errorf("generating random password: %w", err)
		}
		result[i] = charset[n.Int64()]
	}
	return string(result), nil
}

// CredentialsPath returns the file where auto-generated passwords are persisted.
// The file lives in the persistent data volume so passwords stay stable across
// container restarts and are shared with the healthcheck subprocess.
func (c *ServerConfig) CredentialsPath() string {
	return filepath.Join(c.DataDir, "credentials.env")
}

// EnsurePasswords resolves RCON and admin passwords. Explicitly set environment
// variables always win; otherwise previously generated values are reused from
// the credentials file, and new ones are generated and persisted on first run.
func (c *ServerConfig) EnsurePasswords() error {
	path := c.CredentialsPath()

	var err error
	if vals, err := readEnvFile(path); err == nil {
		// Defensive: the file may pre-exist with loose permissions (created by a
		// host tool or copied in). Re-tighten on every load.
		_ = os.Chmod(path, 0600)
		if c.RCONPassword == "" {
			c.RCONPassword = vals["RCON_PASSWORD"]
		}
		if c.AdminPassword == "" {
			c.AdminPassword = vals["ADMIN_PASSWORD"]
		}
	} else if !os.IsNotExist(err) {
		// The file exists but cannot be read (bad permissions, truncation). Fail
		// loudly instead of silently generating fresh passwords that would not
		// be persisted (O_EXCL) and flap on every process start.
		return fmt.Errorf("reading credentials file %s: %w", path, err)
	}

	if c.RCONPassword != "" && c.AdminPassword != "" {
		return nil
	}

	if c.RCONPassword == "" {
		if c.RCONPassword, err = generatePassword(); err != nil {
			return err
		}
	}
	if c.AdminPassword == "" {
		if c.AdminPassword, err = generatePassword(); err != nil {
			return err
		}
	}

	if err := c.writeCredentials(path); err != nil {
		return fmt.Errorf("persisting credentials: %w", err)
	}

	// Another process (e.g. a concurrent healthcheck) may have created the file
	// between our check and write. Adopt whatever is on disk so all processes
	// agree on the same passwords.
	if vals, err := readEnvFile(path); err == nil {
		if v := vals["RCON_PASSWORD"]; v != "" {
			c.RCONPassword = v
		}
		if v := vals["ADMIN_PASSWORD"]; v != "" {
			c.AdminPassword = v
		}
	}

	return nil
}

func (c *ServerConfig) writeCredentials(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	var sb strings.Builder
	sb.WriteString("# Auto-generated credentials for project-zomboid-server-docker\n")
	sb.WriteString("# Set ADMIN_PASSWORD and RCON_PASSWORD in your .env to override.\n")
	sb.WriteString(fmt.Sprintf("ADMIN_PASSWORD=%s\n", c.AdminPassword))
	sb.WriteString(fmt.Sprintf("RCON_PASSWORD=%s\n", c.RCONPassword))

	// O_EXCL: only the first process wins, avoiding races between the entrypoint
	// and concurrent healthcheck invocations.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		if os.IsExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	_, err = f.WriteString(sb.String())
	return err
}

func readEnvFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	vals := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if ok {
			vals[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return vals, nil
}

// ParseList splits a semicolon/comma/whitespace separated list, trimming and
// dropping empty and duplicate entries while preserving order.
func ParseList(raw string) []string {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ';' || r == ',' || r == ' ' || r == '\t' || r == '\n'
	})
	out := make([]string, 0, len(fields))
	seen := make(map[string]struct{}, len(fields))
	for _, f := range fields {
		if _, ok := seen[f]; ok {
			continue
		}
		seen[f] = struct{}{}
		out = append(out, f)
	}
	return out
}

// ParseModWorkshopIDs normalizes MOD_* list values and drops non-numeric
// workshop/collection IDs with a warning.
func (c *ServerConfig) ParseModWorkshopIDs() {
	c.ModWorkshopIDs = joinNumericList(c.ModWorkshopIDs)
	c.ModNames = strings.Join(ParseList(c.ModNames), ";")
	c.ModWorkshopCollection = joinNumericList(c.ModWorkshopCollection)
}

func joinNumericList(raw string) string {
	parts := ParseList(raw)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if _, err := strconv.Atoi(p); err != nil {
			fmt.Fprintf(os.Stderr, "WARNING: %q is not a valid numeric workshop ID, ignoring\n", p)
			continue
		}
		out = append(out, p)
	}
	return strings.Join(out, ";")
}

// loadSandboxEnv maps every SANDBOX_* environment variable onto SandboxVars.
// For example SANDBOX_Zombies=2 becomes Zombies=2 in SandboxVars.lua.
func (c *ServerConfig) loadSandboxEnv() {
	for _, kv := range os.Environ() {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		if strings.HasPrefix(k, "SANDBOX_") {
			key := strings.TrimPrefix(k, "SANDBOX_")
			if key == "MODE" {
				// SANDBOX_MODE selects a preset; it is not a sandbox key itself.
				continue
			}
			c.SandboxVars[key] = v
		}
	}
}

// loadIniEnv maps every INI_* environment variable onto IniOptions.
// For example INI_SleepAllowed=true becomes SleepAllowed=true in server.ini.
func (c *ServerConfig) loadIniEnv() {
	for _, kv := range os.Environ() {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		if strings.HasPrefix(k, "INI_") {
			key := strings.TrimPrefix(k, "INI_")
			if key == "" {
				continue
			}
			c.IniOptions[key] = v
		}
	}
}

func (c *ServerConfig) Validate() []string {
	var errors []string

	if c.ServerName == "" {
		errors = append(errors, "SERVER_NAME must not be empty")
	} else if !validServerNameRE.MatchString(c.ServerName) {
		errors = append(errors, fmt.Sprintf("SERVER_NAME %q contains invalid characters: only letters, digits, '_' and '-' are allowed (it is used in file paths)", c.ServerName))
	}
	if c.DefaultPort < 1 || c.DefaultPort > 65535 {
		errors = append(errors, "DEFAULT_PORT must be between 1 and 65535")
	}
	if c.UDPPort < 1 || c.UDPPort > 65534 {
		errors = append(errors, "UDP_PORT must be between 1 and 65534 (SteamPort2 is UDP_PORT+1)")
	}
	if c.RCONPort < 1 || c.RCONPort > 65535 {
		errors = append(errors, "RCON_PORT must be between 1 and 65535")
	}
	if c.DefaultPort == c.UDPPort || c.DefaultPort == c.RCONPort || c.UDPPort == c.RCONPort {
		errors = append(errors, "DEFAULT_PORT, UDP_PORT and RCON_PORT must all be distinct")
	}
	if c.MaxPlayers < 1 {
		errors = append(errors, "MAX_PLAYERS must be at least 1")
	}
	if c.MaxPlayers > 100 {
		errors = append(errors, "MAX_PLAYERS must not exceed 100")
	}
	if c.AutosaveInterval < 0 {
		errors = append(errors, "AUTOSAVE_INTERVAL must be non-negative")
	}
	if c.BackupInterval < 1 {
		errors = append(errors, "BACKUP_INTERVAL must be at least 1 minute")
	}
	if c.BackupMaxCount < 1 {
		errors = append(errors, "BACKUP_MAX_COUNT must be at least 1")
	}
	if c.MaxRam == "" || c.MinRam == "" {
		errors = append(errors, "MAX_RAM and MIN_RAM must not be empty")
	}
	if !sandboxModes[c.SandboxMode] {
		errors = append(errors, fmt.Sprintf("SANDBOX_MODE must be one of apocalypse, performance, max (got %q)", c.SandboxMode))
	}
	// BACKUP_PATH is used with os.Create + rotation (deletions); require it to
	// stay inside the data dir so a misconfigured value cannot write to or
	// rotate files in the game install or other mounts.
	if !pathWithin(c.DataDir, c.BackupPath) {
		errors = append(errors, fmt.Sprintf("BACKUP_PATH %q must resolve inside DATA_DIR %q", c.BackupPath, c.DataDir))
	}

	return errors
}

// pathWithin reports whether path, after cleaning, is inside or equal to base.
func pathWithin(base, path string) bool {
	cleanBase := filepath.Clean(base)
	cleanPath := filepath.Clean(path)
	return cleanPath == cleanBase || strings.HasPrefix(cleanPath, cleanBase+string(filepath.Separator))
}
