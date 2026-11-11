//go:build windows

package service

import (
	"strings"
	"testing"
)

func TestRenderPowerShellEscapesSingleQuotes(t *testing.T) {
	content := renderPowerShell(Spec{
		Executable: `C:\Program Files\O'Brien\netferry.exe`,
		ConfigPath: `C:\Users\O'Brien\netferry.toml`,
	})
	if strings.Contains(content, `'C:\Program Files\O'Brien`) {
		t.Fatalf("single quote was not escaped:\n%s", content)
	}
	if !strings.Contains(content, `O''Brien`) {
		t.Fatalf("escaped path missing:\n%s", content)
	}
}

func TestTaskNameIsSafe(t *testing.T) {
	task, err := taskName()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(task, "NetFerry-") {
		t.Fatalf("task name = %q", task)
	}
	if strings.ContainsAny(task, `\/:*?"<>|`) {
		t.Fatalf("task name contains unsafe character: %q", task)
	}
}
