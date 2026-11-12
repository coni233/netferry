//go:build linux

package detect

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func platformSystemProxyTargets() []Target {
	targets := targetsForSource("GNOME system proxy", gnomeProxyAddresses())
	return append(targets, targetsForSource("KDE system proxy", kdeProxyAddresses())...)
}

func gnomeProxyAddresses() []string {
	mode, err := gsettingsValue("org.gnome.system.proxy", "mode")
	if err != nil || strings.Trim(mode, "'") != "manual" {
		return nil
	}

	var addresses []string
	seen := make(map[string]bool)
	for _, protocol := range []string{"http", "https", "socks"} {
		host, err := gsettingsValue("org.gnome.system.proxy."+protocol, "host")
		if err != nil {
			continue
		}
		port, err := gsettingsValue("org.gnome.system.proxy."+protocol, "port")
		if err != nil {
			continue
		}
		host = strings.Trim(host, "'")
		if host == "" {
			continue
		}
		address, ok := normalizeHostPort(net.JoinHostPort(host, strings.TrimSpace(port)))
		if !ok || seen[address] {
			continue
		}
		seen[address] = true
		addresses = append(addresses, address)
	}
	return addresses
}

func gsettingsValue(schema, key string) (string, error) {
	output, err := exec.Command("gsettings", "get", schema, key).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func kdeProxyAddresses() []string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil
	}
	content, err := os.ReadFile(filepath.Join(dir, "kioslaverc"))
	if err != nil {
		return nil
	}
	return parseKioslaverc(string(content))
}
