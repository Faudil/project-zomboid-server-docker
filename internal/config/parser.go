package config

import (
	"crypto/rand"
	"math/big"
	"os"
	"strconv"
	"strings"
)

func envInt(key string, fallback int) int {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback
	}
	i, err := strconv.Atoi(v)
	if err != nil {
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
	return v == "true" || v == "1" || v == "yes"
}

func generatePassword() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	length := 16
	result := make([]byte, length)
	for i := range result {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			panic(err)
		}
		result[i] = charset[n.Int64()]
	}
	return string(result)
}

func (c *ServerConfig) Validate() []string {
	var errors []string

	if c.ServerName == "" {
		errors = append(errors, "SERVER_NAME must not be empty")
	}
	if c.DefaultPort < 1 || c.DefaultPort > 65535 {
		errors = append(errors, "DEFAULT_PORT must be between 1 and 65535")
	}
	if c.UDPPort < 1 || c.UDPPort > 65535 {
		errors = append(errors, "UDP_PORT must be between 1 and 65535")
	}
	if c.RCONPort < 1 || c.RCONPort > 65535 {
		errors = append(errors, "RCON_PORT must be between 1 and 65535")
	}
	if c.MaxPlayers < 1 {
		errors = append(errors, "MAX_PLAYERS must be at least 1")
	}
	if c.AutosaveInterval < 0 {
		errors = append(errors, "AUTOSAVE_INTERVAL must be non-negative")
	}
	if c.PUID < 0 || c.PGID < 0 {
		errors = append(errors, "PUID and PGID must be non-negative")
	}

	return errors
}
