//go:build darwin

package agentenv

import (
	"strings"
	"testing"
)

func TestRenderPlist(t *testing.T) {
	content := renderPlist(Values("127.0.0.1:17890"))
	if !strings.Contains(content, envLabel) {
		t.Fatalf("label missing:\n%s", content)
	}
	if !strings.Contains(content, "launchctl setenv") {
		t.Fatalf("launchctl command missing:\n%s", content)
	}
}
