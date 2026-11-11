package service

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Spec struct {
	Executable string
	ConfigPath string
}

type Status struct {
	Manager   string
	Installed bool
	Detail    string
}

func (spec Spec) Normalize() (Spec, error) {
	executable, err := filepath.Abs(strings.TrimSpace(spec.Executable))
	if err != nil {
		return Spec{}, fmt.Errorf("resolve executable path: %w", err)
	}
	configPath, err := filepath.Abs(strings.TrimSpace(spec.ConfigPath))
	if err != nil {
		return Spec{}, fmt.Errorf("resolve config path: %w", err)
	}
	if executable == "" {
		return Spec{}, errors.New("executable path cannot be empty")
	}
	if configPath == "" {
		return Spec{}, errors.New("config path cannot be empty")
	}
	if _, err := os.Stat(executable); err != nil {
		return Spec{}, fmt.Errorf("inspect executable %s: %w", executable, err)
	}
	return Spec{Executable: executable, ConfigPath: configPath}, nil
}

func appConfigDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("find user config directory: %w", err)
	}
	return filepath.Join(dir, "netferry"), nil
}
