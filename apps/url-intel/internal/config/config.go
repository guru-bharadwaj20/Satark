// Package config loads url-intel settings from environment variables.
package config

import (
	"os"
	"time"
)

// Config holds runtime settings for url-intel.
type Config struct {
	ListenAddr      string
	ShutdownTimeout time.Duration
	RequestTimeout  time.Duration
}

// FromEnv builds a Config from the environment, applying defaults.
func FromEnv() Config {
	return Config{
		ListenAddr:      getEnv("URL_INTEL_LISTEN_ADDR", ":8081"),
		ShutdownTimeout: getDuration("URL_INTEL_SHUTDOWN_TIMEOUT", 10*time.Second),
		RequestTimeout:  getDuration("URL_INTEL_REQUEST_TIMEOUT", 15*time.Second),
	}
}

func getEnv(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func getDuration(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	if d, err := time.ParseDuration(v); err == nil {
		return d
	}
	return def
}
