//go:build linux

package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	managerName = "systemd user service"
	unitName    = "netferry.service"
)

func ManagerName() string {
	return managerName
}

func Preflight() error {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return fmt.Errorf("systemctl not found; automatic startup requires systemd with a user session")
	}
	output, err := exec.Command("systemctl", "--user", "show-environment").CombinedOutput()
	if err != nil {
		return fmt.Errorf(
			"systemd user manager is not available: %w: %s\n"+
				"run setup from a graphical login session on a systemd-based distribution, or use --no-service",
			err,
			strings.TrimSpace(string(output)),
		)
	}
	return nil
}

func Install(spec Spec, dryRun bool) ([]string, error) {
	spec, err := spec.Normalize()
	if err != nil {
		return nil, err
	}
	unitPath, err := unitPath()
	if err != nil {
		return nil, err
	}
	messages := []string{
		fmt.Sprintf("write %s", unitPath),
		"run systemctl --user daemon-reload",
		"run systemctl --user enable --now " + unitName,
	}
	if dryRun {
		return messages, nil
	}
	if err := os.MkdirAll(filepath.Dir(unitPath), 0o700); err != nil {
		return nil, fmt.Errorf("create systemd user directory: %w", err)
	}
	if err := os.WriteFile(unitPath, []byte(renderUnit(spec)), 0o600); err != nil {
		return nil, fmt.Errorf("write systemd unit: %w", err)
	}
	if err := systemctl("daemon-reload"); err != nil {
		return nil, err
	}
	if err := systemctl("enable", "--now", unitName); err != nil {
		return nil, err
	}
	return messages, nil
}

func Uninstall(dryRun bool) ([]string, error) {
	unitPath, err := unitPath()
	if err != nil {
		return nil, err
	}
	messages := []string{
		"run systemctl --user disable --now " + unitName,
		fmt.Sprintf("remove %s", unitPath),
		"run systemctl --user daemon-reload",
	}
	if dryRun {
		return messages, nil
	}
	_, _ = exec.Command("systemctl", "--user", "disable", "--now", unitName).CombinedOutput()
	if err := os.Remove(unitPath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("remove systemd unit: %w", err)
	}
	if err := systemctl("daemon-reload"); err != nil {
		return nil, err
	}
	return messages, nil
}

func Start(dryRun bool) ([]string, error) {
	message := "run systemctl --user start " + unitName
	if dryRun {
		return []string{message}, nil
	}
	if err := systemctl("start", unitName); err != nil {
		return nil, err
	}
	return []string{message}, nil
}

func Stop(dryRun bool) ([]string, error) {
	message := "run systemctl --user stop " + unitName
	if dryRun {
		return []string{message}, nil
	}
	if err := systemctl("stop", unitName); err != nil {
		return nil, err
	}
	return []string{message}, nil
}

func CurrentStatus() (Status, error) {
	unitPath, err := unitPath()
	if err != nil {
		return Status{}, err
	}
	if _, err := os.Stat(unitPath); err != nil {
		return Status{Manager: managerName, Installed: false, Detail: unitName}, nil
	}
	if err := exec.Command("systemctl", "--user", "is-enabled", "--quiet", unitName).Run(); err == nil {
		return Status{Manager: managerName, Installed: true, Detail: unitName}, nil
	}
	return Status{Manager: managerName, Installed: false, Detail: unitPath}, nil
}

func unitPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("find user config directory: %w", err)
	}
	return filepath.Join(dir, "systemd", "user", unitName), nil
}

func renderUnit(spec Spec) string {
	return fmt.Sprintf(`[Unit]
Description=NetFerry adaptive egress gateway
After=network-online.target

[Service]
Type=simple
ExecStart=%s serve --config %s
Restart=on-failure
RestartSec=3

[Install]
WantedBy=default.target
`,
		systemdQuote(spec.Executable),
		systemdQuote(spec.ConfigPath),
	)
}

func systemdQuote(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	value = strings.ReplaceAll(value, `%`, `%%`)
	return `"` + value + `"`
}

func systemctl(arguments ...string) error {
	commandArgs := append([]string{"--user"}, arguments...)
	output, err := exec.Command("systemctl", commandArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl %s: %w: %s", strings.Join(arguments, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}
