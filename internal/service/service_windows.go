//go:build windows

package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	managerName  = "Windows logon command"
	runKeyPath   = `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`
	runValueName = "NetFerry"
)

func ManagerName() string {
	return managerName
}

func Preflight() error {
	return nil
}

func Install(spec Spec, dryRun bool) ([]string, error) {
	spec, err := spec.Normalize()
	if err != nil {
		return nil, err
	}
	script, pidPath, logPath, err := servicePaths()
	if err != nil {
		return nil, err
	}
	command := fmt.Sprintf(
		`powershell.exe -NoProfile -WindowStyle Hidden -ExecutionPolicy Bypass -File "%s"`,
		script,
	)
	messages := []string{
		fmt.Sprintf("write %s", script),
		"register current-user logon command NetFerry",
		"start NetFerry",
	}
	if dryRun {
		return messages, nil
	}

	if err := os.MkdirAll(filepath.Dir(script), 0o700); err != nil {
		return nil, fmt.Errorf("create service directory: %w", err)
	}
	if err := os.WriteFile(script, []byte(renderPowerShell(spec, pidPath, logPath)), 0o600); err != nil {
		return nil, fmt.Errorf("write service script: %w", err)
	}
	removeLegacyTasks()
	if err := setRunValue(command); err != nil {
		return nil, err
	}
	if pid, ok := readPID(pidPath); ok && processRunning(pid) {
		return append(messages, fmt.Sprintf("NetFerry already running (pid %d)", pid)), nil
	}
	if err := startScript(script, pidPath, logPath); err != nil {
		return nil, err
	}
	return messages, nil
}

func Uninstall(dryRun bool) ([]string, error) {
	script, pidPath, _, err := servicePaths()
	if err != nil {
		return nil, err
	}
	messages := []string{
		"remove current-user logon command NetFerry",
		"stop NetFerry",
		fmt.Sprintf("remove %s", script),
	}
	if dryRun {
		return messages, nil
	}

	if runValueInstalled() {
		if err := deleteRunValue(); err != nil {
			return nil, err
		}
	}
	if pid, ok := readPID(pidPath); ok {
		if processRunning(pid) {
			if process, err := os.FindProcess(pid); err == nil {
				_ = process.Kill()
			}
		}
		_ = os.Remove(pidPath)
	}
	removeLegacyTasks()
	if err := os.Remove(script); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("remove service script: %w", err)
	}
	return messages, nil
}

func Start(dryRun bool) ([]string, error) {
	script, pidPath, logPath, err := servicePaths()
	if err != nil {
		return nil, err
	}
	message := "start NetFerry"
	if dryRun {
		return []string{message}, nil
	}
	if pid, ok := readPID(pidPath); ok && processRunning(pid) {
		return []string{fmt.Sprintf("NetFerry already running (pid %d)", pid)}, nil
	}
	if err := startScript(script, pidPath, logPath); err != nil {
		return nil, err
	}
	return []string{message}, nil
}

func Stop(dryRun bool) ([]string, error) {
	_, pidPath, _, err := servicePaths()
	if err != nil {
		return nil, err
	}
	message := "stop NetFerry"
	if dryRun {
		return []string{message}, nil
	}
	pid, ok := readPID(pidPath)
	if !ok {
		return []string{"NetFerry is not running"}, nil
	}
	if processRunning(pid) {
		process, err := os.FindProcess(pid)
		if err != nil {
			return nil, fmt.Errorf("find NetFerry process: %w", err)
		}
		if err := process.Kill(); err != nil {
			return nil, fmt.Errorf("stop NetFerry: %w", err)
		}
	}
	_ = os.Remove(pidPath)
	return []string{fmt.Sprintf("stopped NetFerry (pid %d)", pid)}, nil
}

func CurrentStatus() (Status, error) {
	if runValueInstalled() {
		return Status{Manager: managerName, Installed: true, Detail: "HKCU logon command"}, nil
	}
	return Status{Manager: managerName, Detail: "no logon command"}, nil
}

func servicePaths() (scriptPath, pidPath, logPath string, err error) {
	dir, err := appConfigDir()
	if err != nil {
		return "", "", "", err
	}
	return filepath.Join(dir, "netferry-service.ps1"),
		filepath.Join(dir, "netferry.pid"),
		filepath.Join(dir, "netferry.log"),
		nil
}

func setRunValue(command string) error {
	output, err := exec.Command(
		"reg.exe", "add", runKeyPath,
		"/v", runValueName,
		"/t", "REG_SZ",
		"/d", command,
		"/f",
	).CombinedOutput()
	if err != nil {
		return fmt.Errorf("register logon command: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func deleteRunValue() error {
	output, err := exec.Command("reg.exe", "delete", runKeyPath, "/v", runValueName, "/f").CombinedOutput()
	if err != nil {
		return fmt.Errorf("remove logon command: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func runValueInstalled() bool {
	return exec.Command("reg.exe", "query", runKeyPath, "/v", runValueName).Run() == nil
}

func startScript(script, pidPath, logPath string) error {
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open service log: %w", err)
	}
	command := exec.Command(
		"powershell.exe",
		"-NoProfile",
		"-WindowStyle", "Hidden",
		"-ExecutionPolicy", "Bypass",
		"-File", script,
	)
	command.Stdout = logFile
	command.Stderr = logFile
	if err := command.Start(); err != nil {
		_ = logFile.Close()
		return fmt.Errorf("start NetFerry: %w", err)
	}
	_ = logFile.Close()
	_ = command.Process.Release()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if pid, ok := readPID(pidPath); ok && processRunning(pid) {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	if tail := readLogTail(logPath, 2048); tail != "" {
		return fmt.Errorf("NetFerry did not start within 5s: %s", tail)
	}
	return fmt.Errorf("NetFerry did not start within 5s")
}

func readPID(path string) (int, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return 0, false
	}
	return pid, true
}

func processRunning(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	_ = process.Release()
	return true
}

func readLogTail(path string, limit int64) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return ""
	}
	offset := info.Size() - limit
	if offset < 0 {
		offset = 0
	}
	if _, err := file.Seek(offset, 0); err != nil {
		return ""
	}
	buffer := make([]byte, int(limit))
	count, _ := file.Read(buffer)
	return strings.TrimSpace(string(buffer[:count]))
}

func removeLegacyTasks() {
	output, err := exec.Command("schtasks.exe", "/Query", "/FO", "CSV", "/NH").Output()
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(output), "\n") {
		name := csvFirstField(line)
		if !strings.HasPrefix(strings.ToLower(name), "netferry-") {
			continue
		}
		_ = exec.Command("schtasks.exe", "/Delete", "/TN", name, "/F").Run()
	}
}

func csvFirstField(line string) string {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, `"`) {
		return ""
	}
	end := strings.Index(line[1:], `"`)
	if end < 0 {
		return ""
	}
	return line[1 : end+1]
}

func renderPowerShell(spec Spec, pidPath, logPath string) string {
	quote := func(value string) string {
		return strings.ReplaceAll(value, "'", "''")
	}
	return fmt.Sprintf(
		"$ErrorActionPreference = 'Stop'\r\n"+
			"$pidFile = '%s'\r\n"+
			"$logFile = '%s'\r\n"+
			"try {\r\n"+
			"    $process = Start-Process -FilePath '%s' -ArgumentList 'serve --config \"%s\"' -WindowStyle Hidden -PassThru\r\n"+
			"    Set-Content -LiteralPath $pidFile -Value $process.Id\r\n"+
			"    $process.WaitForExit()\r\n"+
			"} catch {\r\n"+
			"    $_ | Out-String | Add-Content -LiteralPath $logFile\r\n"+
			"    exit 1\r\n"+
			"} finally {\r\n"+
			"    Remove-Item -LiteralPath $pidFile -ErrorAction SilentlyContinue\r\n"+
			"}\r\n",
		quote(pidPath),
		quote(logPath),
		quote(spec.Executable),
		quote(spec.ConfigPath),
	)
}
