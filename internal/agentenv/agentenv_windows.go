//go:build windows

package agentenv

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"unicode/utf16"
)

const managerName = "Windows user environment"

func ManagerName() string {
	return managerName
}

func Install(listen string, dryRun bool) ([]string, error) {
	values := Values(listen)
	messages := []string{
		fmt.Sprintf("set %d user environment variables to %s", len(values), values["HTTP_PROXY"]),
		"restart Agent applications so they inherit the new environment",
	}
	if dryRun {
		return messages, nil
	}

	current, err := readUserEnvironment(VariableNames())
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
	if err := applyUserEnvironment(values, nil); err != nil {
		return nil, err
	}
	return messages, nil
}

func Uninstall(dryRun bool) ([]string, error) {
	state, err := LoadState()
	if err != nil {
		return nil, err
	}
	if state == nil {
		return []string{"no managed user environment to restore"}, nil
	}
	if dryRun {
		return []string{
			fmt.Sprintf("restore %d previously managed user environment variables", len(state.Managed)),
			"remove environment state",
		}, nil
	}

	current, err := readUserEnvironment(VariableNames())
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
	if err := applyUserEnvironment(nil, changes); err != nil {
		return nil, err
	}
	if err := RemoveState(); err != nil {
		return nil, err
	}

	messages := []string{
		fmt.Sprintf("restored %d managed user environment variables", len(changes)),
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
		return Status{Manager: managerName, Detail: "no managed user environment"}, nil
	}
	return Status{
		Manager:   managerName,
		Installed: true,
		Detail:    fmt.Sprintf("%d managed variables", len(state.Managed)),
	}, nil
}

func readUserEnvironment(names []string) (map[string]string, error) {
	quoted := make([]string, 0, len(names))
	for _, name := range names {
		quoted = append(quoted, powerShellQuote(name))
	}
	script := fmt.Sprintf(`
$ErrorActionPreference = 'Stop'
$names = @(%s)
$result = @{}
foreach ($name in $names) {
    $value = [Environment]::GetEnvironmentVariable($name, 'User')
    if ($null -ne $value) { $result[$name] = $value }
}
$result | ConvertTo-Json -Compress
`, strings.Join(quoted, ","))
	output, err := runPowerShell(script)
	if err != nil {
		return nil, err
	}
	output = bytes.TrimSpace(output)
	if len(output) == 0 {
		return map[string]string{}, nil
	}
	var values map[string]string
	if err := json.Unmarshal(output, &values); err != nil {
		return nil, fmt.Errorf("parse Windows user environment: %w: %s", err, output)
	}
	return values, nil
}

func applyUserEnvironment(set map[string]string, changes map[string]*string) error {
	var script strings.Builder
	script.WriteString("$ErrorActionPreference = 'Stop'\n")
	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Fprintf(
			&script,
			"[Environment]::SetEnvironmentVariable(%s, %s, 'User')\n",
			powerShellQuote(name),
			powerShellQuote(set[name]),
		)
	}
	names = names[:0]
	for name := range changes {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		value := "$null"
		if changes[name] != nil {
			value = powerShellQuote(*changes[name])
		}
		fmt.Fprintf(
			&script,
			"[Environment]::SetEnvironmentVariable(%s, %s, 'User')\n",
			powerShellQuote(name),
			value,
		)
	}
	_, err := runPowerShell(script.String())
	return err
}

func runPowerShell(script string) ([]byte, error) {
	encoded := base64.StdEncoding.EncodeToString(utf16LE(script))
	command := exec.Command(
		"powershell.exe",
		"-NoProfile",
		"-NonInteractive",
		"-ExecutionPolicy", "Bypass",
		"-EncodedCommand", encoded,
	)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		if detail := strings.TrimSpace(stderr.String()); detail != "" {
			return nil, fmt.Errorf("update Windows user environment: %w: %s", err, detail)
		}
		return nil, fmt.Errorf("update Windows user environment: %w", err)
	}
	return output, nil
}

func powerShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func utf16LE(value string) []byte {
	encoded := utf16.Encode([]rune(value))
	output := make([]byte, len(encoded)*2)
	for index, codeUnit := range encoded {
		binary.LittleEndian.PutUint16(output[index*2:], codeUnit)
	}
	return output
}
