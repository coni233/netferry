//go:build windows

package agentenv

import (
	"encoding/json"
	"testing"
)

func TestPowerShellQuote(t *testing.T) {
	got := powerShellQuote("O'Brien")
	if got != "'O''Brien'" {
		t.Fatalf("quote = %q", got)
	}
}

func TestUTF16LE(t *testing.T) {
	got := utf16LE("A")
	if len(got) != 2 || got[0] != 'A' || got[1] != 0 {
		t.Fatalf("UTF-16LE = %v", got)
	}
}

func TestRunPowerShellKeepsStderrOutOfOutput(t *testing.T) {
	output, err := runPowerShell("Write-Warning 'noise'\n@{ ok = $true } | ConvertTo-Json -Compress\n")
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]bool
	if err := json.Unmarshal(output, &value); err != nil {
		t.Fatalf("output = %q: %v", output, err)
	}
	if !value["ok"] {
		t.Fatalf("output = %q, want ok=true", output)
	}
}
