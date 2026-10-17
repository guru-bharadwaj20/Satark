// Package config loads gateway-api settings from environment variables.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds runtime settings.
type Config struct {
	Addr            string
	LogLevel        slog.Level
	ShutdownTimeout time.Duration
	TextAnalyzerURL string
	QRDecoderURL    string
	AnalyzerTimeout time.Duration
	ScanTimeout     time.Duration
	MaxTextChars    int
	MaxImageBytes   int64

	// Optional backends: empty DatabaseURL disables scan persistence, empty
	// ValkeyAddr disables rate limiting (local docker compose runs neither).
	DatabaseURL    string
	ValkeyAddr     string
	ValkeyPassword string
	RateLimitRPS   float64
	RateLimitBurst int
}

// Load reads configuration from the environment, applying defaults.
func Load() (Config, error) {
	c := Config{
		Addr:            env("ADDR", ":"+env("PORT", "8080")),
		TextAnalyzerURL: strings.TrimRight(env("TEXT_ANALYZER_URL", "http://text-analyzer:8000"), "/"),
		QRDecoderURL:    strings.TrimRight(env("QR_DECODER_URL", "http://qr-decoder:8001"), "/"),
		DatabaseURL:     env("DATABASE_URL", ""),
		ValkeyAddr:      env("VALKEY_ADDR", ""),
		ValkeyPassword:  env("VALKEY_PASSWORD", ""),
	}
	var err error
	if c.LogLevel, err = parseLevel(env("LOG_LEVEL", "info")); err != nil {
		return c, err
	}
	if c.ShutdownTimeout, err = time.ParseDuration(env("SHUTDOWN_TIMEOUT", "15s")); err != nil {
		return c, fmt.Errorf("SHUTDOWN_TIMEOUT: %w", err)
	}
	if c.AnalyzerTimeout, err = time.ParseDuration(env("ANALYZER_TIMEOUT", "2s")); err != nil {
		return c, fmt.Errorf("ANALYZER_TIMEOUT: %w", err)
	}
	if c.ScanTimeout, err = time.ParseDuration(env("SCAN_TIMEOUT", "4s")); err != nil {
		return c, fmt.Errorf("SCAN_TIMEOUT: %w", err)
	}
	if c.MaxTextChars, err = strconv.Atoi(env("MAX_TEXT_CHARS", "5000")); err != nil {
		return c, fmt.Errorf("MAX_TEXT_CHARS: %w", err)
	}
	maxImg, err := strconv.ParseInt(env("MAX_IMAGE_BYTES", "5242880"), 10, 64)
	if err != nil {
		return c, fmt.Errorf("MAX_IMAGE_BYTES: %w", err)
	}
	c.MaxImageBytes = maxImg
	if c.RateLimitRPS, err = strconv.ParseFloat(env("RATE_LIMIT_RPS", "1"), 64); err != nil || c.RateLimitRPS <= 0 {
		return c, fmt.Errorf("RATE_LIMIT_RPS must be a positive number")
	}
	if c.RateLimitBurst, err = strconv.Atoi(env("RATE_LIMIT_BURST", "30")); err != nil || c.RateLimitBurst < 1 {
		return c, fmt.Errorf("RATE_LIMIT_BURST must be a positive integer")
	}
	return c, nil
}

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func parseLevel(s string) (slog.Level, error) {
	var l slog.Level
	if err := l.UnmarshalText([]byte(s)); err != nil {
		return l, fmt.Errorf("LOG_LEVEL: %w", err)
	}
	return l, nil
}
