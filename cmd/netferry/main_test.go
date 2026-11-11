package main

import (
	"flag"
	"path/filepath"
	"strings"
	"testing"

	"github.com/coni233/netferry/internal/config"
)

func TestProxyEnvironmentReplacesExistingProxy(t *testing.T) {
	input := []string{
		"PATH=C:\\Windows",
		"HTTPS_PROXY=http://old.example",
		"NO_PROXY=internal.example,localhost",
	}

	output := proxyEnvironment(input, "http://127.0.0.1:17890")
	joined := strings.Join(output, "\n")
	if strings.Contains(joined, "old.example") {
		t.Fatalf("old proxy value remains:\n%s", joined)
	}
	if !strings.Contains(joined, "HTTPS_PROXY=http://127.0.0.1:17890") {
		t.Fatalf("new HTTPS_PROXY missing:\n%s", joined)
	}
	if !strings.Contains(joined, "NO_PROXY=internal.example,localhost,127.0.0.1,::1") {
		t.Fatalf("NO_PROXY was not merged:\n%s", joined)
	}
}

func TestNormalizeAddress(t *testing.T) {
	tests := map[string]string{
		"github.com":         "github.com:443",
		"github.com:8443":    "github.com:8443",
		"https://github.com": "github.com:443",
		"::1":                "[::1]:443",
		"[::1]":              "[::1]:443",
	}
	for input, want := range tests {
		got, err := normalizeAddress(input)
		if err != nil {
			t.Fatalf("normalizeAddress(%q) error = %v", input, err)
		}
		if got != want {
			t.Fatalf("normalizeAddress(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestResolveSettingsPrecedence(t *testing.T) {
	t.Setenv("NETFERRY_CONFIG", "")
	t.Setenv("NETFERRY_LISTEN", "127.0.0.1:18002")
	t.Setenv("NETFERRY_UPSTREAM", "")
	t.Setenv("NETFERRY_PROBE_TIMEOUT", "")
	t.Setenv("NETFERRY_CACHE_TTL", "")

	path := filepath.Join(t.TempDir(), "netferry.toml")
	if err := config.WriteNew(path, config.Settings{
		Listen:       "127.0.0.1:18001",
		Upstream:     "http://127.0.0.1:10809",
		ProbeTimeout: config.DefaultProbeTimeout,
		CacheTTL:     config.DefaultCacheTTL,
	}); err != nil {
		t.Fatal(err)
	}

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	settingsFlags := addSettingsFlags(fs)
	if err := fs.Parse([]string{
		"--config", path,
		"--listen", "127.0.0.1:18003",
	}); err != nil {
		t.Fatal(err)
	}
	settings, resolvedPath, err := resolveSettings(fs, settingsFlags)
	if err != nil {
		t.Fatal(err)
	}

	if resolvedPath != path {
		t.Fatalf("config path = %q, want %q", resolvedPath, path)
	}
	if settings.Listen != "127.0.0.1:18003" {
		t.Fatalf("listen = %q, want command-line override", settings.Listen)
	}
	if settings.Upstream != "http://127.0.0.1:10809" {
		t.Fatalf("upstream = %q, want config value", settings.Upstream)
	}
}
