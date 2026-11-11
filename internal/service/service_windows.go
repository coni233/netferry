//go:build windows

package service

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
)

const managerName = "Windows logon task"

func ManagerName() string {
	return managerName
}

func Install(spec Spec, dryRun bool) ([]string, error) {
	spec, err := spec.Normalize()
	if err != nil {
		return nil, err
	}
	task, err := taskName()
	if err != nil {
		return nil, err
	}
	script, err := serviceScriptPath()
	if err != nil {
		return nil, err
	}

	action := fmt.Sprintf(
		`powershell.exe -NoProfile -WindowStyle Hidden -ExecutionPolicy Bypass -File "%s"`,
		script,
	)
	commands := [][]string{
		{"schtasks.exe", "/Create", "/TN", task, "/SC", "ONLOGON", "/TR", action, "/F"},
		{"schtasks.exe", "/Run", "/TN", task},
	}
	messages := []string{
		fmt.Sprintf("write %s", script),
		fmt.Sprintf("create logon task %s", task),
		fmt.Sprintf("start task %s", task),
	}
	if dryRun {
		return messages, nil
	}

	if err := os.MkdirAll(filepath.Dir(script), 0o700); err != nil {
		return nil, fmt.Errorf("create service directory: %w", err)
	}
	if err := os.WriteFile(script, []byte(renderPowerShell(spec)), 0o600); err != nil {
		return nil, fmt.Errorf("write service script: %w", err)
	}
	for _, command := range commands {
		if output, err := exec.Command(command[0], command[1:]...).CombinedOutput(); err != nil {
			return nil, fmt.Errorf("%s: %w: %s", strings.Join(command, " "), err, strings.TrimSpace(string(output)))
		}
	}
	return messages, nil
}

func Uninstall(dryRun bool) ([]string, error) {
	task, err := taskName()
	if err != nil {
		return nil, err
	}
	script, err := serviceScriptPath()
	if err != nil {
		return nil, err
	}
	messages := []string{
		fmt.Sprintf("stop task %s", task),
		fmt.Sprintf("delete logon task %s", task),
		fmt.Sprintf("remove %s", script),
	}
	if dryRun {
		return messages, nil
	}

	if err := exec.Command("schtasks.exe", "/Query", "/TN", task).Run(); err == nil {
		_, _ = exec.Command("schtasks.exe", "/End", "/TN", task).CombinedOutput()
		if output, err := exec.Command("schtasks.exe", "/Delete", "/TN", task, "/F").CombinedOutput(); err != nil {
			return nil, fmt.Errorf("delete logon task: %w: %s", err, strings.TrimSpace(string(output)))
		}
	}
	if err := os.Remove(script); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("remove service script: %w", err)
	}
	return messages, nil
}

func Start(dryRun bool) ([]string, error) {
	task, err := taskName()
	if err != nil {
		return nil, err
	}
	message := fmt.Sprintf("start task %s", task)
	if dryRun {
		return []string{message}, nil
	}
	if output, err := exec.Command("schtasks.exe", "/Run", "/TN", task).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("start logon task: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return []string{message}, nil
}

func Stop(dryRun bool) ([]string, error) {
	task, err := taskName()
	if err != nil {
		return nil, err
	}
	message := fmt.Sprintf("stop task %s", task)
	if dryRun {
		return []string{message}, nil
	}
	if output, err := exec.Command("schtasks.exe", "/End", "/TN", task).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("stop logon task: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return []string{message}, nil
}

func CurrentStatus() (Status, error) {
	task, err := taskName()
	if err != nil {
		return Status{}, err
	}
	err = exec.Command("schtasks.exe", "/Query", "/TN", task).Run()
	if err == nil {
		return Status{Manager: managerName, Installed: true, Detail: task}, nil
	}
	return Status{Manager: managerName, Installed: false, Detail: task}, nil
}

func renderPowerShell(spec Spec) string {
	quote := func(value string) string {
		return strings.ReplaceAll(value, "'", "''")
	}
	return fmt.Sprintf(
		"$ErrorActionPreference = 'Stop'\r\n"+
			"& '%s' serve --config '%s'\r\n",
		quote(spec.Executable),
		quote(spec.ConfigPath),
	)
}

func serviceScriptPath() (string, error) {
	dir, err := appConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "netferry-service.ps1"), nil
}

func taskName() (string, error) {
	current, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("get current user: %w", err)
	}
	name := strings.ToLower(current.Username)
	if index := strings.LastIndexAny(name, `\/`); index >= 0 {
		name = name[index+1:]
	}
	var safe strings.Builder
	for _, char := range name {
		switch {
		case char >= 'a' && char <= 'z':
			safe.WriteRune(char)
		case char >= '0' && char <= '9':
			safe.WriteRune(char)
		case char == '-', char == '_':
			safe.WriteRune(char)
		}
	}
	suffix := safe.String()
	if suffix == "" {
		suffix = "user"
	}
	if len(suffix) > 48 {
		suffix = suffix[:48]
	}
	return "NetFerry-" + suffix, nil
}
