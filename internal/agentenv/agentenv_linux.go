//go:build linux

package agentenv

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const managerName = "systemd user environment"

func ManagerName() string {
	return managerName
}

func Install(listen string, dryRun bool) ([]string, error) {
	path, err := environmentPath()
	if err != nil {
		return nil, err
	}
	messages := []string{
		fmt.Sprintf("write %s", path),
		"sign out and back in so graphical applications inherit the environment",
	}
	if dryRun {
		return messages, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create environment.d directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(renderEnvironment(Values(listen))), 0o600); err != nil {
		return nil, fmt.Errorf("write environment.d file: %w", err)
	}
	return messages, nil
}

func Uninstall(dryRun bool) ([]string, error) {
	path, err := environmentPath()
	if err != nil {
		return nil, err
	}
	message := fmt.Sprintf("remove %s", path)
	if dryRun {
		return []string{message, "sign out and back in to clear the environment"}, nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("remove environment.d file: %w", err)
	}
	return []string{message, "sign out and back in to clear the environment"}, nil
}

func CurrentStatus() (Status, error) {
	path, err := environmentPath()
	if err != nil {
		return Status{}, err
	}
	if _, err := os.Stat(path); err == nil {
		return Status{Manager: managerName, Installed: true, Detail: path}, nil
	}
	return Status{Manager: managerName, Detail: path}, nil
}

func environmentPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("find user config directory: %w", err)
	}
	return filepath.Join(dir, "environment.d", "50-netferry.conf"), nil
}

func renderEnvironment(values map[string]string) string {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	var builder strings.Builder
	builder.WriteString("# Managed by NetFerry\n")
	for _, name := range names {
		fmt.Fprintf(&builder, "%s=%s\n", name, quoteEnvironmentValue(values[name]))
	}
	return builder.String()
}

func quoteEnvironmentValue(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return `"` + value + `"`
}
