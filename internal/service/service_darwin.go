//go:build darwin

package service

import (
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	managerName = "macOS LaunchAgent"
	label       = "io.github.coni233.netferry"
)

func ManagerName() string {
	return managerName
}

func Install(spec Spec, dryRun bool) ([]string, error) {
	spec, err := spec.Normalize()
	if err != nil {
		return nil, err
	}
	plistPath, logPath, err := paths()
	if err != nil {
		return nil, err
	}
	domain := fmt.Sprintf("gui/%d", os.Getuid())
	messages := []string{
		fmt.Sprintf("write %s", plistPath),
		fmt.Sprintf("load %s/%s", domain, label),
	}
	if dryRun {
		return messages, nil
	}
	if err := os.MkdirAll(filepath.Dir(plistPath), 0o700); err != nil {
		return nil, fmt.Errorf("create LaunchAgent directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
		return nil, fmt.Errorf("create service log directory: %w", err)
	}
	if err := os.WriteFile(plistPath, []byte(renderPlist(spec, logPath)), 0o600); err != nil {
		return nil, fmt.Errorf("write LaunchAgent: %w", err)
	}
	_, _ = exec.Command("launchctl", "bootout", domain, plistPath).CombinedOutput()
	if output, err := exec.Command("launchctl", "bootstrap", domain, plistPath).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("bootstrap LaunchAgent: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return messages, nil
}

func Uninstall(dryRun bool) ([]string, error) {
	plistPath, _, err := paths()
	if err != nil {
		return nil, err
	}
	domain := fmt.Sprintf("gui/%d", os.Getuid())
	messages := []string{
		fmt.Sprintf("unload %s/%s", domain, label),
		fmt.Sprintf("remove %s", plistPath),
	}
	if dryRun {
		return messages, nil
	}
	_, _ = exec.Command("launchctl", "bootout", domain, plistPath).CombinedOutput()
	if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("remove LaunchAgent: %w", err)
	}
	return messages, nil
}

func Start(dryRun bool) ([]string, error) {
	domain := fmt.Sprintf("gui/%d", os.Getuid())
	message := fmt.Sprintf("start %s/%s", domain, label)
	if dryRun {
		return []string{message}, nil
	}
	if output, err := exec.Command("launchctl", "kickstart", "-k", domain+"/"+label).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("start LaunchAgent: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return []string{message}, nil
}

func Stop(dryRun bool) ([]string, error) {
	domain := fmt.Sprintf("gui/%d", os.Getuid())
	message := fmt.Sprintf("stop %s/%s", domain, label)
	if dryRun {
		return []string{message}, nil
	}
	if output, err := exec.Command("launchctl", "kill", "SIGTERM", domain+"/"+label).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("stop LaunchAgent: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return []string{message}, nil
}

func CurrentStatus() (Status, error) {
	plistPath, _, err := paths()
	if err != nil {
		return Status{}, err
	}
	domain := fmt.Sprintf("gui/%d/%s", os.Getuid(), label)
	err = exec.Command("launchctl", "print", domain).Run()
	if err == nil {
		return Status{Manager: managerName, Installed: true, Detail: domain}, nil
	}
	if _, statErr := os.Stat(plistPath); statErr == nil {
		return Status{Manager: managerName, Installed: false, Detail: plistPath}, nil
	}
	return Status{Manager: managerName, Installed: false, Detail: label}, nil
}

func paths() (string, string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", fmt.Errorf("find home directory: %w", err)
	}
	dir, err := appConfigDir()
	if err != nil {
		return "", "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", label+".plist"),
		filepath.Join(dir, "netferry.log"),
		nil
}

func renderPlist(spec Spec, logPath string) string {
	escape := func(value string) string {
		var builder strings.Builder
		_ = xml.EscapeText(&builder, []byte(value))
		return builder.String()
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>%s</string>
  <key>ProgramArguments</key>
  <array>
    <string>%s</string>
    <string>serve</string>
    <string>--config</string>
    <string>%s</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>ProcessType</key>
  <string>Background</string>
  <key>StandardOutPath</key>
  <string>%s</string>
  <key>StandardErrorPath</key>
  <string>%s</string>
</dict>
</plist>
`, label, escape(spec.Executable), escape(spec.ConfigPath), escape(logPath), escape(logPath))
}
