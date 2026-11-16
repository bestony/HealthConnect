// Package config reads process settings from the environment.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
)

const (
	defaultHTTPAddr             = "127.0.0.1:8080"
	defaultDataDir              = "./data"
	defaultMaxUploadBytes       = 8 << 30
	defaultMaxUncompressedBytes = 16 << 30
	defaultLogLevel             = "info"
)

// Config is the process configuration.
type Config struct {
	HTTPAddr             string
	DataDir              string
	MaxUploadBytes       int64
	MaxUncompressedBytes int64
	LogLevel             slog.Level
	MCPEnabled           bool
}

// FromEnv reads configuration from the process environment.
func FromEnv() (Config, error) {
	return Parse(osEnv())
}

func osEnv() map[string]string {
	out := make(map[string]string)
	for _, pair := range os.Environ() {
		key, value, ok := strings.Cut(pair, "=")
		if !ok {
			continue
		}
		out[key] = value
	}
	return out
}

// Parse builds a Config from a key-value map. Missing keys use defaults.
func Parse(env map[string]string) (Config, error) {
	cfg := Config{
		HTTPAddr:             valueOr(env, "HTTP_ADDR", defaultHTTPAddr),
		DataDir:              valueOr(env, "DATA_DIR", defaultDataDir),
		MaxUploadBytes:       defaultMaxUploadBytes,
		MaxUncompressedBytes: defaultMaxUncompressedBytes,
		LogLevel:             slog.LevelInfo,
		MCPEnabled:           true,
	}
	var err error
	if raw := strings.TrimSpace(env["MAX_UPLOAD_BYTES"]); raw != "" {
		cfg.MaxUploadBytes, err = strconv.ParseInt(raw, 10, 64)
		if err != nil || cfg.MaxUploadBytes <= 0 {
			return Config{}, fmt.Errorf("MAX_UPLOAD_BYTES: want a positive integer")
		}
	}
	if raw := strings.TrimSpace(env["MAX_UNCOMPRESSED_BYTES"]); raw != "" {
		cfg.MaxUncompressedBytes, err = strconv.ParseInt(raw, 10, 64)
		if err != nil || cfg.MaxUncompressedBytes <= 0 {
			return Config{}, fmt.Errorf("MAX_UNCOMPRESSED_BYTES: want a positive integer")
		}
	}
	level := valueOr(env, "LOG_LEVEL", defaultLogLevel)
	cfg.LogLevel, err = parseLevel(level)
	if err != nil {
		return Config{}, err
	}
	cfg.MCPEnabled, err = parseBool(env["MCP_ENABLED"], true)
	if err != nil {
		return Config{}, err
	}
	if strings.TrimSpace(cfg.HTTPAddr) == "" {
		return Config{}, fmt.Errorf("HTTP_ADDR: empty")
	}
	if strings.TrimSpace(cfg.DataDir) == "" {
		return Config{}, fmt.Errorf("DATA_DIR: empty")
	}
	return cfg, nil
}

func valueOr(env map[string]string, key, fallback string) string {
	raw := strings.TrimSpace(env[key])
	if raw == "" {
		return fallback
	}
	return raw
}

func parseBool(raw string, fallback bool) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "":
		return fallback, nil
	case "1", "true":
		return true, nil
	case "0", "false":
		return false, nil
	default:
		return false, fmt.Errorf("MCP_ENABLED: want true or false")
	}
}

func parseLevel(raw string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("LOG_LEVEL: want debug, info, warn, or error")
	}
}
