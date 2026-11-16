package config

import "testing"

func TestParseDefaults(t *testing.T) {
	cfg, err := Parse(map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddr != "127.0.0.1:8080" {
		t.Fatalf("HTTPAddr = %q", cfg.HTTPAddr)
	}
	if cfg.DataDir != "./data" {
		t.Fatalf("DataDir = %q", cfg.DataDir)
	}
	if cfg.MaxUploadBytes != 8<<30 {
		t.Fatalf("MaxUploadBytes = %d", cfg.MaxUploadBytes)
	}
	if cfg.MaxUncompressedBytes != 16<<30 {
		t.Fatalf("MaxUncompressedBytes = %d", cfg.MaxUncompressedBytes)
	}
	if cfg.LogLevel.String() != "INFO" {
		t.Fatalf("LogLevel = %s", cfg.LogLevel)
	}
	if !cfg.MCPEnabled {
		t.Fatal("MCPEnabled = false")
	}
}

func TestParseOverrides(t *testing.T) {
	cfg, err := Parse(map[string]string{
		"HTTP_ADDR":              "127.0.0.1:9090",
		"DATA_DIR":               "/tmp/health",
		"MAX_UPLOAD_BYTES":       "1024",
		"MAX_UNCOMPRESSED_BYTES": "2048",
		"LOG_LEVEL":              "debug",
		"MCP_ENABLED":            "false",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddr != "127.0.0.1:9090" || cfg.DataDir != "/tmp/health" {
		t.Fatalf("cfg = %+v", cfg)
	}
	if cfg.MaxUploadBytes != 1024 || cfg.MaxUncompressedBytes != 2048 {
		t.Fatalf("limits = %d %d", cfg.MaxUploadBytes, cfg.MaxUncompressedBytes)
	}
	if cfg.LogLevel.String() != "DEBUG" {
		t.Fatalf("level = %s", cfg.LogLevel)
	}
	if cfg.MCPEnabled {
		t.Fatal("MCPEnabled = true")
	}
}

func TestParseMCPEnabled(t *testing.T) {
	on, err := Parse(map[string]string{"MCP_ENABLED": "true"})
	if err != nil || !on.MCPEnabled {
		t.Fatalf("on = %+v %v", on.MCPEnabled, err)
	}
	_, err = Parse(map[string]string{"MCP_ENABLED": "maybe"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParseRejectsBadLevel(t *testing.T) {
	_, err := Parse(map[string]string{"LOG_LEVEL": "loud"})
	if err == nil {
		t.Fatal("expected error")
	}
}
