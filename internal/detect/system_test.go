package detect

import (
	"reflect"
	"testing"
)

func TestParseProxyServerSingleAddress(t *testing.T) {
	got := ParseProxyServer("127.0.0.1:7897")
	want := []string{"127.0.0.1:7897"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseProxyServer = %v, want %v", got, want)
	}
}

func TestParseProxyServerPerProtocol(t *testing.T) {
	got := ParseProxyServer("http=127.0.0.1:7897;https=127.0.0.1:7897")
	want := []string{"127.0.0.1:7897"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseProxyServer = %v, want %v", got, want)
	}

	got = ParseProxyServer("socks=127.0.0.1:1080;http=127.0.0.1:8080")
	want = []string{"127.0.0.1:1080", "127.0.0.1:8080"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseProxyServer = %v, want %v", got, want)
	}
}

func TestParseProxyServerRejectsInvalidPorts(t *testing.T) {
	for _, raw := range []string{"", "http=127.0.0.1:0", "127.0.0.1", "http=127.0.0.1:70000"} {
		if got := ParseProxyServer(raw); len(got) != 0 {
			t.Fatalf("ParseProxyServer(%q) = %v, want no addresses", raw, got)
		}
	}
}

func TestProxyTargetsFromEnvironment(t *testing.T) {
	values := map[string]string{
		"HTTPS_PROXY": "http://127.0.0.1:7897",
		"HTTP_PROXY":  "socks5://127.0.0.1:1080",
		"ALL_PROXY":   "127.0.0.1:2080",
	}
	targets := proxyTargetsFromEnvironment(func(name string) string {
		return values[name]
	})
	if len(targets) != 3 {
		t.Fatalf("got %d targets, want 3: %v", len(targets), targets)
	}
	want := []string{"127.0.0.1:7897", "127.0.0.1:1080", "127.0.0.1:2080"}
	for index, target := range targets {
		if target.Address != want[index] {
			t.Fatalf("target %d address = %q, want %q", index, target.Address, want[index])
		}
	}
}

func TestNormalizeProxyAddress(t *testing.T) {
	if got := normalizeProxyAddress("LOCALHOST:7897"); got != "localhost:7897" {
		t.Fatalf("normalizeProxyAddress = %q", got)
	}
	if got := normalizeProxyAddress("not-an-address"); got != "not-an-address" {
		t.Fatalf("normalizeProxyAddress = %q", got)
	}
}

func TestParseScutilProxy(t *testing.T) {
	output := `<dictionary> {
  HTTPEnable : 1
  HTTPPort : 7890
  HTTPProxy : 127.0.0.1
  HTTPSEnable : 0
  HTTPSPort : 0
  HTTPSProxy :
  SOCKSEnable : 1
  SOCKSPort : 7891
  SOCKSProxy : 127.0.0.1
}`
	got := parseScutilProxy(output)
	want := []string{"127.0.0.1:7890", "127.0.0.1:7891"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseScutilProxy = %v, want %v", got, want)
	}
}

func TestParseKioslaverc(t *testing.T) {
	content := `[Proxy Settings]
ProxyType=1
httpProxy=http://127.0.0.1:7890
httpsProxy=http://127.0.0.1:7890
socksProxy=socks://127.0.0.1:7891

[Other]
ProxyType=0`
	got := parseKioslaverc(content)
	want := []string{"127.0.0.1:7890", "127.0.0.1:7891"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseKioslaverc = %v, want %v", got, want)
	}

	if got := parseKioslaverc("[Proxy Settings]\nProxyType=0\nhttpProxy=http://127.0.0.1:7890"); len(got) != 0 {
		t.Fatalf("parseKioslaverc with disabled proxy = %v, want no addresses", got)
	}
}
