// Package config loads runtime configuration from FP_* environment variables.
package config

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	// Embed the timezone database so time.LoadLocation works on Windows and on
	// a scratch container that has no system zoneinfo.
	_ "time/tzdata"
)

// Config holds all runtime configuration.
type Config struct {
	Env             string         // "dev" or "prod"
	Addr            string         // HTTP listen address, e.g. ":8080"
	BaseURL         string         // externally-reachable base URL
	DataDir         string         // directory for the sqlite db + cached files
	DBPath          string         // full path to the sqlite file
	EncryptionKey   []byte         // 32-byte AES key derived from FP_ENCRYPTION_KEY
	AdminPassphrase string         // optional bootstrap passphrase (first run only)
	DefaultLocale   string         // e.g. "nl"
	TimeZone        *time.Location // e.g. Europe/Brussels
	SessionTTL      time.Duration  // admin session lifetime

	// App-level OAuth client credentials (one app per provider). Data sources
	// only store the user's token obtained via interactive sign-in.
	MSClientID         string // Microsoft app (ms_graph + onedrive)
	MSClientSecret     string
	GoogleClientID     string // Google app (google_photos)
	GoogleClientSecret string
}

// OAuthApp returns the app-level client credentials for a data-source type.
func (c *Config) OAuthApp(dsType string) (clientID, clientSecret string) {
	switch dsType {
	case "ms_graph", "onedrive", "ms_todo":
		return c.MSClientID, c.MSClientSecret
	case "google_photos":
		return c.GoogleClientID, c.GoogleClientSecret
	}
	return "", ""
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// Load reads configuration from the environment, applying sensible defaults.
func Load() (*Config, error) {
	c := &Config{
		Env:             env("FP_ENV", "dev"),
		Addr:            env("FP_ADDR", ":8080"),
		BaseURL:         env("FP_BASE_URL", "http://localhost:8080"),
		DataDir:         env("FP_DATA_DIR", "./data"),
		AdminPassphrase: os.Getenv("FP_ADMIN_PASSPHRASE"),
		DefaultLocale:   env("FP_LOCALE", "nl"),
	}
	c.DBPath = env("FP_DB_PATH", filepath.Join(c.DataDir, "planner.db"))

	tzName := env("FP_TIMEZONE", "Europe/Brussels")
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		return nil, fmt.Errorf("load timezone %q: %w", tzName, err)
	}
	c.TimeZone = loc

	days, err := strconv.Atoi(env("FP_SESSION_DAYS", "90"))
	if err != nil {
		return nil, fmt.Errorf("invalid FP_SESSION_DAYS: %w", err)
	}
	c.SessionTTL = time.Duration(days) * 24 * time.Hour

	secret := os.Getenv("FP_ENCRYPTION_KEY")
	if secret == "" {
		if c.Env == "prod" {
			return nil, fmt.Errorf("FP_ENCRYPTION_KEY is required when FP_ENV=prod")
		}
		secret = "dev-insecure-key-change-me"
	}
	sum := sha256.Sum256([]byte(secret))
	c.EncryptionKey = sum[:]

	c.MSClientID = os.Getenv("FP_MS_CLIENT_ID")
	c.MSClientSecret = os.Getenv("FP_MS_CLIENT_SECRET")
	c.GoogleClientID = os.Getenv("FP_GOOGLE_CLIENT_ID")
	c.GoogleClientSecret = os.Getenv("FP_GOOGLE_CLIENT_SECRET")

	return c, nil
}
