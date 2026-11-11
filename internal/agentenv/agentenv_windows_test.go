//go:build windows

package agentenv

import (
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
