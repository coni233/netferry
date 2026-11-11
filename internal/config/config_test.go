package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadOptionalAndApply(t *testing.T) {
	path := filepath.Join(t.TempDir(), "netferry.toml")
	content := `
# Local proxy
listen = "127.0.0.1:19000"
upstream = "http://user:pass@127.0.0.1:10809"
probe_timeout = "2s"
cache_ttl = "30s" # short cache
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	overrides, err := LoadOptional(path)
	if err != nil {
		t.Fatal(err)
	}
	settings := Default()
	settings.Apply(overrides)

	if settings.Listen != "127.0.0.1:19000" {
		t.Fatalf("listen = %q", settings.Listen)
	}
	if settings.Upstream != "http://user:pass@127.0.0.1:10809" {
		t.Fatalf("upstream = %q", settings.Upstream)
	}
	if settings.ProbeTimeout != 2*time.Second {
		t.Fatalf("probe timeout = %s", settings.ProbeTimeout)
	}
	if settings.CacheTTL != 30*time.Second {
		t.Fatalf("cache TTL = %s", settings.CacheTTL)
	}
}

func TestLoadOptionalRejectsUnknownKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "netferry.toml")
	if err := os.WriteFile(path, []byte(`unknown = "value"`), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := LoadOptional(path); err == nil {
		t.Fatal("expected unknown key error")
	}
}

func TestWriteNewPreservesExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "netferry.toml")
	if err := WriteNew(path, Default()); err != nil {
		t.Fatal(err)
	}
	if err := WriteNew(path, Default()); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("second write error = %v, want ErrAlreadyExists", err)
	}
}

func TestMissingConfigUsesDefaults(t *testing.T) {
	overrides, err := LoadOptional(filepath.Join(t.TempDir(), "missing.toml"))
	if err != nil {
		t.Fatal(err)
	}
	settings := Default()
	settings.Apply(overrides)
	if settings != Default() {
		t.Fatalf("settings = %#v, want defaults", settings)
	}
}

func TestValidateRejectsUnsupportedUpstream(t *testing.T) {
	settings := Default()
	settings.Upstream = "ftp://127.0.0.1:10808"
	if err := settings.Validate(); err == nil {
		t.Fatal("expected unsupported upstream error")
	}
}
