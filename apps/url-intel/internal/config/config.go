// Package config loads url-intel settings from environment variables.
package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds runtime settings for url-intel.
type Config struct {
	ListenAddr      string
	ShutdownTimeout time.Duration
	RequestTimeout  time.Duration

	MaxRedirects  int
	ExpandTimeout time.Duration
	LookupTimeout time.Duration

	SafeBrowsingAPIKey string
	Version            string
}

// FromEnv builds a Config from the environment, applying defaults.
func FromEnv() Config {
	return Config{
		ListenAddr:      getEnv("URL_INTEL_LISTEN_ADDR", ":8081"),
		ShutdownTimeout: getDuration("URL_INTEL_SHUTDOWN_TIMEOUT", 10*time.Second),
		RequestTimeout:  getDuration("URL_INTEL_REQUEST_TIMEOUT", 15*time.Second),
		MaxRedirects:    getInt("URL_INTEL_MAX_REDIRECTS", 5),
		ExpandTimeout:   getDuration("URL_INTEL_EXPAND_TIMEOUT", 4*time.Second),
		LookupTimeout:   getDuration("URL_INTEL_LOOKUP_TIMEOUT", 4*time.Second),

		SafeBrowsingAPIKey: os.Getenv("URL_INTEL_SAFE_BROWSING_API_KEY"),
		Version:            getEnv("URL_INTEL_VERSION", "dev"),
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

func getInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	if n, err := strconv.Atoi(v); err == nil {
		return n
	}
	return def
}
