package main

import (
	"strings"
	"testing"
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
