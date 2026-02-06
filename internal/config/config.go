// Package config provides application configuration loaded from environment variables.
package config

import "os"

// Default values for configuration.
const (
	DefaultPiholeTarget = "http://127.0.0.1:8080"
	DefaultListenAddr   = ":80"
	DefaultDBFile       = "go_links.json"
)

// Config holds the application configuration.
type Config struct {
	PiholeTarget string
	ListenAddr   string
	DBFile       string
}

// Load reads configuration from environment variables, falling back to defaults.
func Load() Config {
	return Config{
		PiholeTarget: getEnv("PIHOLE_TARGET", DefaultPiholeTarget),
		ListenAddr:   getEnv("LISTEN_ADDR", DefaultListenAddr),
		DBFile:       getEnv("DB_FILE", DefaultDBFile),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
