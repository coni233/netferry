package agentenv

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type State struct {
	Platform string             `json:"platform"`
	Managed  map[string]string  `json:"managed"`
	Previous map[string]*string `json:"previous"`
	Updated  time.Time          `json:"updated"`
}

type Status struct {
	Manager   string
	Installed bool
	Detail    string
}

func Values(listen string) map[string]string {
	proxyValue := proxyURL(listen)
	localBypass := "localhost,127.0.0.1,::1"
	return map[string]string{
		"HTTP_PROXY":  proxyValue,
		"HTTPS_PROXY": proxyValue,
		"ALL_PROXY":   proxyValue,
		"http_proxy":  proxyValue,
		"https_proxy": proxyValue,
		"all_proxy":   proxyValue,
		"NO_PROXY":    localBypass,
		"no_proxy":    localBypass,
	}
}

func proxyURL(listen string) string {
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return "http://" + listen
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port)
}

func VariableNames() []string {
	values := Values("127.0.0.1:0")
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func StatePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("find user config directory: %w", err)
	}
	return filepath.Join(dir, "netferry", "environment-state.json"), nil
}

func LoadState() (*State, error) {
	path, err := StatePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read environment state: %w", err)
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parse environment state: %w", err)
	}
	return &state, nil
}

func SaveState(state State) error {
	path, err := StatePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create environment state directory: %w", err)
	}
	state.Updated = time.Now().UTC()
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode environment state: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write environment state: %w", err)
	}
	return nil
}

func RemoveState() error {
	path, err := StatePath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove environment state: %w", err)
	}
	return nil
}

func previousSnapshot(current map[string]string, managed map[string]string) map[string]*string {
	previous := make(map[string]*string, len(managed))
	for name := range managed {
		if value, exists := current[name]; exists {
			copied := value
			previous[name] = &copied
		} else {
			previous[name] = nil
		}
	}
	return previous
}
