package detect

import (
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
)

const (
	systemProxyPriority = 0
	envProxyPriority    = 5
)

func systemTargets() []Target {
	targets := platformSystemProxyTargets()
	return append(targets, environmentProxyTargets()...)
}

func targetsForSource(software string, addresses []string) []Target {
	targets := make([]Target, 0, len(addresses))
	for _, address := range addresses {
		targets = append(targets, Target{
			Software: software,
			Name:     "system proxy",
			Address:  address,
			Priority: systemProxyPriority,
		})
	}
	return targets
}

func environmentProxyTargets() []Target {
	return proxyTargetsFromEnvironment(os.Getenv)
}

func proxyTargetsFromEnvironment(getenv func(string) string) []Target {
	names := []string{
		"HTTPS_PROXY", "https_proxy",
		"HTTP_PROXY", "http_proxy",
		"ALL_PROXY", "all_proxy",
	}
	targets := make([]Target, 0, len(names))
	for _, name := range names {
		address, ok := proxyAddressFromURL(getenv(name))
		if !ok {
			continue
		}
		targets = append(targets, Target{
			Software: "environment variable",
			Name:     name,
			Address:  address,
			Priority: envProxyPriority,
		})
	}
	return targets
}

// ParseProxyServer parses the Windows ProxyServer registry value. The value is
// either a single host:port or a semicolon-separated list such as
// "http=127.0.0.1:7897;https=127.0.0.1:7897".
func ParseProxyServer(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	parts := []string{raw}
	if strings.Contains(raw, "=") {
		parts = parts[:0]
		for _, entry := range strings.Split(raw, ";") {
			_, value, ok := strings.Cut(entry, "=")
			if !ok {
				continue
			}
			parts = append(parts, value)
		}
	}

	result := make([]string, 0, len(parts))
	seen := make(map[string]bool, len(parts))
	for _, part := range parts {
		address, ok := normalizeHostPort(part)
		if !ok || seen[address] {
			continue
		}
		seen[address] = true
		result = append(result, address)
	}
	return result
}

// parseScutilProxy parses the dictionary printed by "scutil --proxy" on macOS.
func parseScutilProxy(output string) []string {
	values := make(map[string]string)
	for _, line := range strings.Split(output, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok {
			continue
		}
		values[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}

	var addresses []string
	seen := make(map[string]bool)
	for _, protocol := range []string{"HTTP", "HTTPS", "SOCKS"} {
		if values[protocol+"Enable"] != "1" {
			continue
		}
		host := values[protocol+"Proxy"]
		port := values[protocol+"Port"]
		if host == "" || port == "" {
			continue
		}
		address, ok := normalizeHostPort(net.JoinHostPort(host, port))
		if !ok || seen[address] {
			continue
		}
		seen[address] = true
		addresses = append(addresses, address)
	}
	return addresses
}

// parseKioslaverc extracts manual proxy settings from KDE's kioslaverc file.
func parseKioslaverc(content string) []string {
	inProxySection := false
	values := make(map[string]string)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inProxySection = strings.EqualFold(line[1:len(line)-1], "Proxy Settings")
			continue
		}
		if !inProxySection {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		values[strings.ToLower(strings.TrimSpace(key))] = strings.TrimSpace(value)
	}
	if values["proxytype"] != "1" {
		return nil
	}

	var addresses []string
	seen := make(map[string]bool)
	for _, key := range []string{"httpproxy", "httpsproxy", "socksproxy"} {
		address, ok := proxyAddressFromURL(values[key])
		if !ok || seen[address] {
			continue
		}
		seen[address] = true
		addresses = append(addresses, address)
	}
	return addresses
}

func proxyAddressFromURL(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return "", false
	}
	return normalizeHostPort(parsed.Host)
}

func normalizeHostPort(raw string) (string, bool) {
	host, port, err := net.SplitHostPort(strings.TrimSpace(raw))
	if err != nil {
		return "", false
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 {
		return "", false
	}
	host = strings.Trim(strings.ToLower(host), "[]")
	if host == "" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port), true
}

func normalizeProxyAddress(raw string) string {
	if address, ok := normalizeHostPort(raw); ok {
		return address
	}
	return strings.ToLower(strings.TrimSpace(raw))
}
