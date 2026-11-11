//go:build linux

package agentenv

import (
	"strings"
	"testing"
)

func TestRenderEnvironment(t *testing.T) {
	content := renderEnvironment(Values("127.0.0.1:17890"))
	for _, expected := range []string{
		`HTTP_PROXY="http://127.0.0.1:17890"`,
		`HTTPS_PROXY="http://127.0.0.1:17890"`,
		`NO_PROXY="localhost,127.0.0.1,::1"`,
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("environment missing %q:\n%s", expected, content)
		}
	}
}
