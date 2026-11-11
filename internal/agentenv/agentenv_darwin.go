//go:build darwin

package agentenv

import (
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

const (
	managerName = "macOS launchd environment"
	envLabel    = "io.github.coni233.netferry.env"
)

func ManagerName() string {
	return managerName
}

func Install(listen string, dryRun bool) ([]string, error) {
	values := Values(listen)
	plistPath, err := plistPath()
	if err != nil {
		return nil, err
	}
	messages := []string{
		fmt.Sprintf("set %d variables in the current launchd session", len(values)),
		fmt.Sprintf("write %s", plistPath),
		"restart Agent applications so they inherit the environment",
	}
	if dryRun {
		return messages, nil
	}

	current, err := readSessionEnvironment(VariableNames())
	if err != nil {
		return nil, err
	}
	state, err := LoadState()
	if err != nil {
		return nil, err
	}
	if state == nil {
		state = &State{Previous: previousSnapshot(current, values)}
	} else {
		if state.Previous == nil {
			state.Previous = make(map[string]*string)
		}
		fresh := previousSnapshot(current, values)
		for name := range values {
			if _, exists := state.Previous[name]; !exists {
				state.Previous[name] = fresh[name]
			}
		}
	}
	state.Platform = runtime.GOOS
	state.Managed = values
	if err := SaveState(*state); err != nil {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(plistPath), 0o700); err != nil {
		return nil, fmt.Errorf("create LaunchAgent directory: %w", err)
	}
	if err := os.WriteFile(plistPath, []byte(renderPlist(values)), 0o600); err != nil {
		return nil, fmt.Errorf("write environment LaunchAgent: %w", err)
	}

	domain := fmt.Sprintf("gui/%d", os.Getuid())
	_, _ = exec.Command("launchctl", "bootout", domain, plistPath).CombinedOutput()
	if output, err := exec.Command("launchctl", "bootstrap", domain, plistPath).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("bootstrap environment LaunchAgent: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if err := applySessionEnvironment(values, nil); err != nil {
		return nil, err
	}
	return messages, nil
}

func Uninstall(dryRun bool) ([]string, error) {
	state, err := LoadState()
	if err != nil {
		return nil, err
	}
	plistPath, pathErr := plistPath()
	if pathErr != nil {
		return nil, pathErr
	}
	if state == nil {
		return []string{"no managed launchd environment to restore"}, nil
	}
	if dryRun {
		return []string{
			fmt.Sprintf("restore %d previously managed launchd variables", len(state.Managed)),
			fmt.Sprintf("remove %s", plistPath),
		}, nil
	}

	current, err := readSessionEnvironment(VariableNames())
	if err != nil {
		return nil, err
	}
	changes := make(map[string]*string)
	var skipped []string
	for name, managed := range state.Managed {
		if current[name] != managed {
			skipped = append(skipped, name)
			continue
		}
		changes[name] = state.Previous[name]
	}
	if err := applySessionEnvironment(nil, changes); err != nil {
		return nil, err
	}

	domain := fmt.Sprintf("gui/%d", os.Getuid())
	_, _ = exec.Command("launchctl", "bootout", domain, plistPath).CombinedOutput()
	if err := os.Remove(plistPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("remove environment LaunchAgent: %w", err)
	}
	if err := RemoveState(); err != nil {
		return nil, err
	}

	messages := []string{
		fmt.Sprintf("restored %d managed launchd variables", len(changes)),
		"restart Agent applications to clear the NetFerry environment",
	}
	if len(skipped) > 0 {
		sort.Strings(skipped)
		messages = append(messages, "left user-modified variables unchanged: "+strings.Join(skipped, ", "))
	}
	return messages, nil
}

func CurrentStatus() (Status, error) {
	state, err := LoadState()
	if err != nil {
		return Status{}, err
	}
	if state == nil {
		return Status{Manager: managerName, Detail: "no managed launchd environment"}, nil
	}
	return Status{
		Manager:   managerName,
		Installed: true,
		Detail:    fmt.Sprintf("%d managed variables", len(state.Managed)),
	}, nil
}

func readSessionEnvironment(names []string) (map[string]string, error) {
	result := make(map[string]string)
	for _, name := range names {
		output, err := exec.Command("launchctl", "getenv", name).Output()
		if err != nil {
			continue
		}
		value := strings.TrimSpace(string(output))
		if value != "" {
			result[name] = value
		}
	}
	return result, nil
}

func applySessionEnvironment(set map[string]string, changes map[string]*string) error {
	for name, value := range set {
		if output, err := exec.Command("launchctl", "setenv", name, value).CombinedOutput(); err != nil {
			return fmt.Errorf("set launchd variable %s: %w: %s", name, err, strings.TrimSpace(string(output)))
		}
	}
	for name, value := range changes {
		var output []byte
		var err error
		if value == nil {
			output, err = exec.Command("launchctl", "unsetenv", name).CombinedOutput()
		} else {
			output, err = exec.Command("launchctl", "setenv", name, *value).CombinedOutput()
		}
		if err != nil {
			return fmt.Errorf("restore launchd variable %s: %w: %s", name, err, strings.TrimSpace(string(output)))
		}
	}
	return nil
}

func plistPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find home directory: %w", err)
	}
	return filepath.Join(home, "Library", "LaunchAgents", envLabel+".plist"), nil
}

func renderPlist(values map[string]string) string {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	var commands strings.Builder
	for _, name := range names {
		fmt.Fprintf(
			&commands,
			"launchctl setenv %s %s; ",
			shellQuote(name),
			shellQuote(values[name]),
		)
	}
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
    <string>/bin/sh</string>
    <string>-c</string>
    <string>%s</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
</dict>
</plist>
`, envLabel, escape(commands.String()))
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
