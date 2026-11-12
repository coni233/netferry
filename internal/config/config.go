package config

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultListen       = "127.0.0.1:17890"
	DefaultUpstream     = "socks5h://127.0.0.1:10808"
	DefaultProbeTimeout = 1500 * time.Millisecond
	DefaultCacheTTL     = 5 * time.Minute
)

var ErrAlreadyExists = errors.New("configuration file already exists")

type Settings struct {
	Listen       string
	Upstream     string
	ProbeTimeout time.Duration
	CacheTTL     time.Duration
}

type Overrides struct {
	Listen       *string
	Upstream     *string
	ProbeTimeout *time.Duration
	CacheTTL     *time.Duration
}

func Default() Settings {
	return Settings{
		Listen:       DefaultListen,
		Upstream:     DefaultUpstream,
		ProbeTimeout: DefaultProbeTimeout,
		CacheTTL:     DefaultCacheTTL,
	}
}

func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("find user config directory: %w", err)
	}
	return filepath.Join(dir, "netferry", "netferry.toml"), nil
}

// LoadOptional loads a flat TOML configuration. A missing file is treated as
// an empty configuration so commands can still run with defaults.
func LoadOptional(path string) (Overrides, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return Overrides{}, nil
	}
	if err != nil {
		return Overrides{}, fmt.Errorf("open config %s: %w", path, err)
	}
	defer file.Close()

	var result Overrides
	seen := make(map[string]bool)
	scanner := bufio.NewScanner(file)
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := strings.TrimSpace(stripComment(scanner.Text()))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") {
			return Overrides{}, fmt.Errorf("%s:%d: TOML tables are not supported in this config", path, lineNumber)
		}

		key, rawValue, ok := strings.Cut(line, "=")
		if !ok {
			return Overrides{}, fmt.Errorf("%s:%d: expected key = value", path, lineNumber)
		}
		key = strings.ToLower(strings.TrimSpace(key))
		if seen[key] {
			return Overrides{}, fmt.Errorf("%s:%d: duplicate key %q", path, lineNumber, key)
		}
		seen[key] = true

		value, err := parseString(strings.TrimSpace(rawValue))
		if err != nil {
			return Overrides{}, fmt.Errorf("%s:%d: %w", path, lineNumber, err)
		}
		if err := applyValue(&result, key, value); err != nil {
			return Overrides{}, fmt.Errorf("%s:%d: %w", path, lineNumber, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return Overrides{}, fmt.Errorf("read config %s: %w", path, err)
	}
	return result, nil
}

func FromEnvironment() (Overrides, error) {
	var result Overrides
	if value := strings.TrimSpace(os.Getenv("NETFERRY_LISTEN")); value != "" {
		result.Listen = &value
	}
	if value := strings.TrimSpace(os.Getenv("NETFERRY_UPSTREAM")); value != "" {
		result.Upstream = &value
	}
	if value := strings.TrimSpace(os.Getenv("NETFERRY_PROBE_TIMEOUT")); value != "" {
		duration, err := time.ParseDuration(value)
		if err != nil {
			return Overrides{}, fmt.Errorf("parse NETFERRY_PROBE_TIMEOUT: %w", err)
		}
		result.ProbeTimeout = &duration
	}
	if value := strings.TrimSpace(os.Getenv("NETFERRY_CACHE_TTL")); value != "" {
		duration, err := time.ParseDuration(value)
		if err != nil {
			return Overrides{}, fmt.Errorf("parse NETFERRY_CACHE_TTL: %w", err)
		}
		result.CacheTTL = &duration
	}
	return result, nil
}

func (settings *Settings) Apply(overrides Overrides) {
	if overrides.Listen != nil {
		settings.Listen = *overrides.Listen
	}
	if overrides.Upstream != nil {
		settings.Upstream = *overrides.Upstream
	}
	if overrides.ProbeTimeout != nil {
		settings.ProbeTimeout = *overrides.ProbeTimeout
	}
	if overrides.CacheTTL != nil {
		settings.CacheTTL = *overrides.CacheTTL
	}
}

func (settings Settings) Validate() error {
	if strings.TrimSpace(settings.Listen) == "" {
		return errors.New("listen address cannot be empty")
	}
	if _, _, err := net.SplitHostPort(settings.Listen); err != nil {
		return fmt.Errorf("invalid listen address %q: %w", settings.Listen, err)
	}
	if strings.TrimSpace(settings.Upstream) == "" {
		return errors.New("upstream proxy cannot be empty")
	}
	parsed, err := url.Parse(settings.Upstream)
	if err != nil {
		return fmt.Errorf("invalid upstream proxy: %w", err)
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "socks5", "socks5h":
	default:
		return fmt.Errorf("unsupported upstream proxy scheme %q", parsed.Scheme)
	}
	if parsed.Host == "" {
		return errors.New("upstream proxy must include a host")
	}
	if settings.ProbeTimeout <= 0 {
		return errors.New("probe timeout must be greater than zero")
	}
	if settings.CacheTTL <= 0 {
		return errors.New("cache TTL must be greater than zero")
	}
	return nil
}

func WriteNew(path string, settings Settings) error {
	if err := settings.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return ErrAlreadyExists
	}
	if err != nil {
		return fmt.Errorf("create config %s: %w", path, err)
	}
	defer file.Close()

	content := fmt.Sprintf(
		"# NetFerry configuration\n"+
			"listen = %s\n"+
			"upstream = %s\n"+
			"probe_timeout = %s\n"+
			"cache_ttl = %s\n",
		strconv.Quote(settings.Listen),
		strconv.Quote(settings.Upstream),
		strconv.Quote(settings.ProbeTimeout.String()),
		strconv.Quote(settings.CacheTTL.String()),
	)
	if _, err := file.WriteString(content); err != nil {
		return fmt.Errorf("write config %s: %w", path, err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync config %s: %w", path, err)
	}
	return nil
}

// UpdateUpstream rewrites the upstream value in an existing configuration file
// while preserving comments and the other keys.
func UpdateUpstream(path, upstream string) error {
	upstream = strings.TrimSpace(upstream)
	if upstream == "" {
		return errors.New("upstream proxy cannot be empty")
	}

	overrides, err := LoadOptional(path)
	if err != nil {
		return err
	}
	settings := Default()
	settings.Apply(overrides)
	settings.Upstream = upstream
	if err := settings.Validate(); err != nil {
		return err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config %s: %w", path, err)
	}
	lines := strings.Split(string(data), "\n")
	replaced := false
	for index, line := range lines {
		key, _, ok := strings.Cut(strings.TrimSpace(stripComment(line)), "=")
		if !ok || !strings.EqualFold(strings.TrimSpace(key), "upstream") {
			continue
		}
		suffix := ""
		if strings.HasSuffix(line, "\r") {
			suffix = "\r"
		}
		lines[index] = "upstream = " + strconv.Quote(upstream) + suffix
		replaced = true
		break
	}
	if !replaced {
		suffix := ""
		if strings.Contains(string(data), "\r\n") {
			suffix = "\r"
		}
		line := "upstream = " + strconv.Quote(upstream) + suffix
		if last := len(lines) - 1; last >= 0 && strings.TrimSpace(lines[last]) == "" {
			lines = append(lines[:last], line, "")
		} else {
			lines = append(lines, line)
		}
	}
	return writeFileAtomic(path, []byte(strings.Join(lines, "\n")))
}

func writeFileAtomic(path string, content []byte) error {
	temp, err := os.CreateTemp(filepath.Dir(path), ".netferry-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary config: %w", err)
	}
	tempName := temp.Name()
	defer os.Remove(tempName)

	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return fmt.Errorf("set config permissions: %w", err)
	}
	if _, err := temp.Write(content); err != nil {
		temp.Close()
		return fmt.Errorf("write config %s: %w", path, err)
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return fmt.Errorf("sync config %s: %w", path, err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close config %s: %w", path, err)
	}
	if err := os.Rename(tempName, path); err != nil {
		return fmt.Errorf("replace config %s: %w", path, err)
	}
	return nil
}

func Format(settings Settings) string {
	return fmt.Sprintf(
		"listen = %s\nupstream = %s\nprobe_timeout = %s\ncache_ttl = %s",
		strconv.Quote(settings.Listen),
		strconv.Quote(settings.Upstream),
		strconv.Quote(settings.ProbeTimeout.String()),
		strconv.Quote(settings.CacheTTL.String()),
	)
}

func applyValue(result *Overrides, key, value string) error {
	switch key {
	case "listen":
		result.Listen = &value
	case "upstream":
		result.Upstream = &value
	case "probe_timeout":
		duration, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("parse probe_timeout: %w", err)
		}
		result.ProbeTimeout = &duration
	case "cache_ttl":
		duration, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("parse cache_ttl: %w", err)
		}
		result.CacheTTL = &duration
	default:
		return fmt.Errorf("unknown key %q", key)
	}
	return nil
}

func parseString(raw string) (string, error) {
	if strings.HasPrefix(raw, "\"") {
		value, err := strconv.Unquote(raw)
		if err != nil {
			return "", fmt.Errorf("invalid quoted string: %w", err)
		}
		return value, nil
	}
	if raw == "" {
		return "", errors.New("missing value")
	}
	return raw, nil
}

func stripComment(line string) string {
	inString := false
	escaped := false
	for index, char := range line {
		switch {
		case escaped:
			escaped = false
		case char == '\\' && inString:
			escaped = true
		case char == '"':
			inString = !inString
		case char == '#' && !inString:
			return line[:index]
		}
	}
	return line
}
