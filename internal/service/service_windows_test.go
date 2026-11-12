//go:build windows

package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderPowerShellEscapesSingleQuotes(t *testing.T) {
	content := renderPowerShell(
		Spec{
			Executable: `C:\Program Files\O'Brien\netferry.exe`,
			ConfigPath: `C:\Users\O'Brien\netferry.toml`,
		},
		`C:\Users\O'Brien\netferry.pid`,
		`C:\Users\O'Brien\netferry.log`,
	)
	if strings.Contains(content, `'C:\Program Files\O'Brien`) {
		t.Fatalf("single quote was not escaped:\n%s", content)
	}
	if !strings.Contains(content, `O''Brien`) {
		t.Fatalf("escaped path missing:\n%s", content)
	}
	if !strings.Contains(content, "Start-Process") {
		t.Fatalf("launcher missing:\n%s", content)
	}
}

func TestReadPID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "netferry.pid")
	if _, ok := readPID(path); ok {
		t.Fatal("missing PID file reported as valid")
	}

	if err := os.WriteFile(path, []byte(" 12345\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	pid, ok := readPID(path)
	if !ok || pid != 12345 {
		t.Fatalf("readPID = %d, %v; want 12345, true", pid, ok)
	}

	if err := os.WriteFile(path, []byte("not-a-pid"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := readPID(path); ok {
		t.Fatal("invalid PID file reported as valid")
	}
}

func TestRenderStartupLauncher(t *testing.T) {
	content := renderStartupLauncher(`C:\Users\O'Brien\100%\netferry-service.ps1`)
	if !strings.Contains(content, "powershell.exe") {
		t.Fatalf("PowerShell launcher missing:\n%s", content)
	}
	if !strings.Contains(content, `O'Brien`) {
		t.Fatalf("script path missing:\n%s", content)
	}
	if !strings.Contains(content, `100%%`) {
		t.Fatalf("percent sign was not escaped for cmd.exe:\n%s", content)
	}
}

func TestInstallStartupLauncher(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AppData", dir)

	path, err := installStartupLauncher(`C:\Tools\netferry-service.ps1`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(path, dir) {
		t.Fatalf("launcher path = %q, want under %q", path, dir)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "netferry-service.ps1") {
		t.Fatalf("launcher content missing script path:\n%s", data)
	}
}
