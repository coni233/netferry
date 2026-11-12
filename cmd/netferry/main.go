package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/coni233/netferry/internal/agentenv"
	"github.com/coni233/netferry/internal/config"
	"github.com/coni233/netferry/internal/detect"
	"github.com/coni233/netferry/internal/proxy"
	"github.com/coni233/netferry/internal/router"
	"github.com/coni233/netferry/internal/service"
	"github.com/coni233/netferry/internal/upstream"
)

const (
	defaultListen       = config.DefaultListen
	defaultUpstream     = config.DefaultUpstream
	defaultProbeTimeout = config.DefaultProbeTimeout
	defaultCacheTTL     = config.DefaultCacheTTL
)

var version = "dev"

func main() {
	os.Exit(runCLI(os.Args[1:]))
}

func runCLI(args []string) int {
	if len(args) == 0 {
		printUsage()
		return 2
	}

	switch args[0] {
	case "serve":
		return runServe(args[1:])
	case "run":
		return runCommand(args[1:])
	case "doctor":
		return runDoctor(args[1:])
	case "env":
		return runEnv(args[1:])
	case "status":
		return runStatus(args[1:])
	case "config":
		return runConfig(args[1:])
	case "detect":
		return runDetect(args[1:])
	case "setup":
		return runSetup(args[1:])
	case "install":
		return runInstall(args[1:])
	case "uninstall":
		return runUninstall(args[1:])
	case "service":
		return runService(args[1:])
	case "version", "--version", "-version":
		fmt.Println(version)
		return 0
	case "help", "--help", "-h":
		printUsage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", args[0])
		printUsage()
		return 2
	}
}

func runServe(args []string) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	settingsFlags := addSettingsFlags(fs)

	if err := fs.Parse(args); err != nil {
		return 2
	}
	settings, configPath, err := resolveSettings(fs, settingsFlags)
	if err != nil {
		fmt.Fprintf(os.Stderr, "netferry: %v\n", err)
		return 1
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	handler, err := newProxyHandler(settings.Listen, settings.Upstream, settings.ProbeTimeout, settings.CacheTTL, logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "netferry: %v\n", err)
		return 1
	}

	server := &http.Server{
		Addr:              settings.Listen,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	logger.Info(
		"netferry listening",
		"address", settings.Listen,
		"upstream", redactProxyURL(settings.Upstream),
		"config", configPath,
	)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintf(os.Stderr, "netferry: %v\n", err)
		return 1
	}
	return 0
}

func runCommand(args []string) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	settingsFlags := addSettingsFlags(fs)

	if err := fs.Parse(args); err != nil {
		return 2
	}
	settings, _, err := resolveSettings(fs, settingsFlags)
	if err != nil {
		fmt.Fprintf(os.Stderr, "netferry: %v\n", err)
		return 1
	}

	command := fs.Args()
	if len(command) > 0 && command[0] == "--" {
		command = command[1:]
	}
	if len(command) == 0 {
		fmt.Fprintln(os.Stderr, "usage: netferry run [flags] -- <command> [args...]")
		return 2
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	stop, err := ensureServer(settings, logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "netferry: %v\n", err)
		return 1
	}
	defer stop()

	cmd := exec.Command(command[0], command[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = proxyEnvironment(os.Environ(), proxyURL(settings.Listen))

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode()
		}
		fmt.Fprintf(os.Stderr, "netferry: %v\n", err)
		return 1
	}
	return 0
}

func runDoctor(args []string) int {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	settingsFlags := addSettingsFlags(fs)
	timeout := fs.Duration("timeout", 0, "override probe timeout")

	if err := fs.Parse(args); err != nil {
		return 2
	}
	settings, _, err := resolveSettings(fs, settingsFlags)
	if err != nil {
		fmt.Fprintf(os.Stderr, "netferry: %v\n", err)
		return 1
	}
	if *timeout > 0 {
		settings.ProbeTimeout = *timeout
	}

	hosts := fs.Args()
	if len(hosts) == 0 {
		hosts = []string{
			"github.com",
			"api.github.com",
			"codeload.github.com",
			"raw.githubusercontent.com",
		}
	}

	upstreamDialer, err := upstream.New(settings.Upstream)
	if err != nil {
		fmt.Fprintf(os.Stderr, "netferry: invalid upstream: %v\n", err)
		return 1
	}

	directProbe := router.NewDirectProbe(settings.ProbeTimeout)
	failed := false

	fmt.Printf("upstream: %s\n\n", redactProxyURL(settings.Upstream))
	for _, input := range hosts {
		address, err := normalizeAddress(input)
		if err != nil {
			fmt.Printf("%-32s invalid: %v\n", input, err)
			failed = true
			continue
		}

		directCtx, cancelDirect := context.WithTimeout(context.Background(), settings.ProbeTimeout)
		directDuration, directErr := directProbe(directCtx, address)
		cancelDirect()

		proxyCtx, cancelProxy := context.WithTimeout(context.Background(), settings.ProbeTimeout)
		proxyStart := time.Now()
		proxyConn, proxyErr := upstreamDialer.DialContext(proxyCtx, "tcp", address)
		proxyDuration := time.Since(proxyStart)
		cancelProxy()
		if proxyConn != nil {
			_ = proxyConn.Close()
		}

		fmt.Printf("%s\n", address)
		fmt.Printf("  direct: %-6s %s", status(directErr), formatDuration(directDuration))
		if directErr != nil {
			fmt.Printf(" (%v)", directErr)
		}
		fmt.Println()
		fmt.Printf("  proxy:  %-6s %s", status(proxyErr), formatDuration(proxyDuration))
		if proxyErr != nil {
			fmt.Printf(" (%v)", proxyErr)
		}
		fmt.Println()

		switch {
		case directErr == nil:
			fmt.Println("  route:  direct")
		case proxyErr == nil:
			fmt.Println("  route:  proxy")
		default:
			fmt.Println("  route:  unavailable")
			failed = true
		}
		fmt.Println()
	}

	if failed {
		return 1
	}
	return 0
}

func runEnv(args []string) int {
	fs := flag.NewFlagSet("env", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	settingsFlags := addSettingsFlags(fs)
	shell := fs.String("shell", defaultShell(), "output shell: powershell or bash")

	if err := fs.Parse(args); err != nil {
		return 2
	}
	settings, _, err := resolveSettings(fs, settingsFlags)
	if err != nil {
		fmt.Fprintf(os.Stderr, "netferry: %v\n", err)
		return 1
	}

	value := proxyURL(settings.Listen)
	switch strings.ToLower(*shell) {
	case "powershell", "pwsh":
		fmt.Printf("$env:HTTP_PROXY = '%s'\n", value)
		fmt.Printf("$env:HTTPS_PROXY = '%s'\n", value)
		fmt.Printf("$env:ALL_PROXY = '%s'\n", value)
		fmt.Printf("$env:http_proxy = '%s'\n", value)
		fmt.Printf("$env:https_proxy = '%s'\n", value)
		fmt.Printf("$env:all_proxy = '%s'\n", value)
		fmt.Println("$env:NO_PROXY = 'localhost,127.0.0.1,::1'")
		fmt.Println("$env:no_proxy = 'localhost,127.0.0.1,::1'")
	case "bash", "sh", "zsh":
		fmt.Printf("export HTTP_PROXY='%s'\n", value)
		fmt.Printf("export HTTPS_PROXY='%s'\n", value)
		fmt.Printf("export ALL_PROXY='%s'\n", value)
		fmt.Printf("export http_proxy='%s'\n", value)
		fmt.Printf("export https_proxy='%s'\n", value)
		fmt.Printf("export all_proxy='%s'\n", value)
		fmt.Println("export NO_PROXY='localhost,127.0.0.1,::1'")
		fmt.Println("export no_proxy='localhost,127.0.0.1,::1'")
	default:
		fmt.Fprintf(os.Stderr, "unsupported shell %q; use powershell or bash\n", *shell)
		return 2
	}
	return 0
}

type settingsFlags struct {
	config       *string
	listen       *string
	upstream     *string
	probeTimeout *time.Duration
	cacheTTL     *time.Duration
}

func addSettingsFlags(fs *flag.FlagSet) *settingsFlags {
	return &settingsFlags{
		config:       fs.String("config", "", "configuration file path"),
		listen:       fs.String("listen", "", "HTTP proxy listen address"),
		upstream:     fs.String("upstream", "", "upstream proxy URL"),
		probeTimeout: fs.Duration("probe-timeout", 0, "direct connectivity probe timeout"),
		cacheTTL:     fs.Duration("cache-ttl", 0, "route cache lifetime"),
	}
}

func resolveSettings(fs *flag.FlagSet, flags *settingsFlags) (config.Settings, string, error) {
	path, err := resolveConfigPath(*flags.config)
	if err != nil {
		return config.Settings{}, "", err
	}

	settings := config.Default()
	fileSettings, err := config.LoadOptional(path)
	if err != nil {
		return config.Settings{}, "", err
	}
	settings.Apply(fileSettings)

	environment, err := config.FromEnvironment()
	if err != nil {
		return config.Settings{}, "", err
	}
	settings.Apply(environment)

	explicit := make(map[string]bool)
	fs.Visit(func(item *flag.Flag) {
		explicit[item.Name] = true
	})
	var commandLine config.Overrides
	if explicit["listen"] {
		commandLine.Listen = flags.listen
	}
	if explicit["upstream"] {
		commandLine.Upstream = flags.upstream
	}
	if explicit["probe-timeout"] {
		commandLine.ProbeTimeout = flags.probeTimeout
	}
	if explicit["cache-ttl"] {
		commandLine.CacheTTL = flags.cacheTTL
	}
	settings.Apply(commandLine)

	if err := settings.Validate(); err != nil {
		return config.Settings{}, "", err
	}
	return settings, path, nil
}

func resolveConfigPath(explicit string) (string, error) {
	path := strings.TrimSpace(explicit)
	if path == "" {
		path = strings.TrimSpace(os.Getenv("NETFERRY_CONFIG"))
	}
	if path == "" {
		defaultPath, err := config.DefaultPath()
		if err != nil {
			return "", err
		}
		return defaultPath, nil
	}
	return path, nil
}

func runStatus(args []string) int {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	settingsFlags := addSettingsFlags(fs)
	jsonOutput := fs.Bool("json", false, "print machine-readable JSON")

	if err := fs.Parse(args); err != nil {
		return 2
	}
	settings, configPath, err := resolveSettings(fs, settingsFlags)
	if err != nil {
		fmt.Fprintf(os.Stderr, "netferry: %v\n", err)
		return 1
	}

	running, detail, err := netFerryListeningDetail(settings.Listen)
	if err != nil {
		detail = err.Error()
	}
	serviceStatus, serviceErr := service.CurrentStatus()
	if serviceErr != nil {
		serviceStatus = service.Status{
			Manager: service.ManagerName(),
			Detail:  serviceErr.Error(),
		}
	}
	environmentStatus, environmentErr := agentenv.CurrentStatus()
	if environmentErr != nil {
		environmentStatus = agentenv.Status{
			Manager: agentenv.ManagerName(),
			Detail:  environmentErr.Error(),
		}
	}

	status := struct {
		Running              bool   `json:"running"`
		Detail               string `json:"detail,omitempty"`
		Listen               string `json:"listen"`
		ProxyURL             string `json:"proxy_url"`
		Upstream             string `json:"upstream"`
		ConfigPath           string `json:"config_path"`
		ServiceManager       string `json:"service_manager"`
		ServiceInstalled     bool   `json:"service_installed"`
		ServiceDetail        string `json:"service_detail,omitempty"`
		EnvironmentManager   string `json:"environment_manager"`
		EnvironmentInstalled bool   `json:"environment_installed"`
		EnvironmentDetail    string `json:"environment_detail,omitempty"`
	}{
		Running:              running,
		Detail:               detail,
		Listen:               settings.Listen,
		ProxyURL:             proxyURL(settings.Listen),
		Upstream:             redactProxyURL(settings.Upstream),
		ConfigPath:           configPath,
		ServiceManager:       serviceStatus.Manager,
		ServiceInstalled:     serviceStatus.Installed,
		ServiceDetail:        serviceStatus.Detail,
		EnvironmentManager:   environmentStatus.Manager,
		EnvironmentInstalled: environmentStatus.Installed,
		EnvironmentDetail:    environmentStatus.Detail,
	}

	if *jsonOutput {
		encoded, err := json.MarshalIndent(status, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "netferry: %v\n", err)
			return 1
		}
		fmt.Println(string(encoded))
	} else {
		if running {
			fmt.Println("status:    running")
		} else {
			fmt.Println("status:    stopped")
		}
		fmt.Printf("listen:    %s\n", status.Listen)
		fmt.Printf("proxy:     %s\n", status.ProxyURL)
		fmt.Printf("upstream:  %s\n", status.Upstream)
		fmt.Printf("config:    %s\n", status.ConfigPath)
		fmt.Printf("service:   %s (%s)\n", status.ServiceManager, installedText(status.ServiceInstalled))
		fmt.Printf("environment: %s (%s)\n", status.EnvironmentManager, installedText(status.EnvironmentInstalled))
		if status.Detail != "" && !running {
			fmt.Printf("detail:    %s\n", status.Detail)
		}
	}

	if !running {
		return 1
	}
	return 0
}

func runConfig(args []string) int {
	action := "show"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		action = args[0]
		args = args[1:]
	}

	fs := flag.NewFlagSet("config", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	settingsFlags := addSettingsFlags(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	switch action {
	case "path":
		path, err := resolveConfigPath(*settingsFlags.config)
		if err != nil {
			fmt.Fprintf(os.Stderr, "netferry: %v\n", err)
			return 1
		}
		fmt.Println(path)
		return 0
	case "show":
		settings, path, err := resolveSettings(fs, settingsFlags)
		if err != nil {
			fmt.Fprintf(os.Stderr, "netferry: %v\n", err)
			return 1
		}
		fmt.Printf("# %s\n%s\n", path, config.Format(settings))
		return 0
	case "init":
		settings, path, err := resolveSettings(fs, settingsFlags)
		if err != nil {
			fmt.Fprintf(os.Stderr, "netferry: %v\n", err)
			return 1
		}
		if err := config.WriteNew(path, settings); err != nil {
			if errors.Is(err, config.ErrAlreadyExists) {
				fmt.Printf("config already exists: %s\n", path)
				return 1
			}
			fmt.Fprintf(os.Stderr, "netferry: %v\n", err)
			return 1
		}
		fmt.Printf("created config: %s\n", path)
		return 0
	case "set":
		if len(fs.Args()) != 2 {
			fmt.Fprintln(os.Stderr, "usage: netferry config set upstream <url>")
			return 2
		}
		key := strings.ToLower(fs.Args()[0])
		if key != "upstream" {
			fmt.Fprintf(os.Stderr, "unsupported config key %q; only upstream can be set\n", key)
			return 2
		}
		path, err := resolveConfigPath(*settingsFlags.config)
		if err != nil {
			fmt.Fprintf(os.Stderr, "netferry: %v\n", err)
			return 1
		}
		if err := config.UpdateUpstream(path, fs.Args()[1]); err != nil {
			fmt.Fprintf(os.Stderr, "netferry: %v\n", err)
			return 1
		}
		fmt.Printf("updated %s: upstream = %s\n", path, fs.Args()[1])
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown config command %q; use path, show, init, or set\n", action)
		return 2
	}
}

func runDetect(args []string) int {
	fs := flag.NewFlagSet("detect", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	timeout := fs.Duration("timeout", 500*time.Millisecond, "per-protocol probe timeout")
	jsonOutput := fs.Bool("json", false, "print machine-readable JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	candidates := detect.Detect(context.Background(), *timeout)
	if len(candidates) == 0 {
		if *jsonOutput {
			fmt.Println("[]")
		} else {
			fmt.Println("no known local proxy endpoints detected")
		}
		return 1
	}

	if *jsonOutput {
		encoded, err := json.MarshalIndent(candidates, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "netferry: %v\n", err)
			return 1
		}
		fmt.Println(string(encoded))
		return 0
	}

	fmt.Println("detected local proxies:")
	for _, candidate := range candidates {
		fmt.Printf("  - %s\n", candidate.String())
		if candidate.Upstream != "" {
			fmt.Printf("    upstream: %s\n", candidate.Upstream)
		} else {
			fmt.Println("    upstream: unavailable (authentication is not supported)")
		}
	}
	return 0
}

func runInstall(args []string) int {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	configFlag := fs.String("config", "", "configuration file path")
	dryRun := fs.Bool("dry-run", false, "show actions without changing the system")
	noDetect := fs.Bool("no-detect", false, "do not auto-detect a local proxy when creating config")
	upstreamFlag := fs.String("upstream", "", "upstream proxy URL for a new configuration")

	if err := fs.Parse(args); err != nil {
		return 2
	}
	configPath, err := resolveConfigPath(*configFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "netferry: %v\n", err)
		return 1
	}
	if err := service.Preflight(); err != nil {
		fmt.Fprintf(os.Stderr, "netferry: %v\n", err)
		return 1
	}
	_, statErr := os.Stat(configPath)
	configExisted := statErr == nil

	settings, detected, err := initialSettingsForInstall(configPath, !*noDetect, *upstreamFlag, *dryRun)
	if err != nil {
		fmt.Fprintf(os.Stderr, "netferry: %v\n", err)
		return 1
	}
	printDetectedProxy(detected)
	if detected == nil && strings.TrimSpace(*upstreamFlag) != "" {
		fmt.Printf("upstream: %s (manual)\n", settings.Upstream)
	}
	warnUnreachableUpstream(settings, detected, *upstreamFlag)

	if !*dryRun {
		if err := config.WriteNew(configPath, settings); err != nil && !errors.Is(err, config.ErrAlreadyExists) {
			fmt.Fprintf(os.Stderr, "netferry: create config: %v\n", err)
			return 1
		}
	}

	executable, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "netferry: find executable: %v\n", err)
		return 1
	}
	messages, err := service.Install(service.Spec{
		Executable: executable,
		ConfigPath: configPath,
	}, *dryRun)
	if err != nil {
		fmt.Fprintf(os.Stderr, "netferry: %v\n", err)
		printInstallIncomplete(configPath, configExisted, false)
		return 1
	}
	printServiceMessages(service.ManagerName(), messages, *dryRun)
	return 0
}

func runSetup(args []string) int {
	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	configFlag := fs.String("config", "", "configuration file path")
	dryRun := fs.Bool("dry-run", false, "show actions without changing the system")
	noService := fs.Bool("no-service", false, "do not install automatic startup")
	noEnvironment := fs.Bool("no-env", false, "do not install proxy environment variables")
	noDetect := fs.Bool("no-detect", false, "do not auto-detect a local proxy when creating config")
	upstreamFlag := fs.String("upstream", "", "upstream proxy URL for a new configuration")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	configPath, err := resolveConfigPath(*configFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "netferry: %v\n", err)
		return 1
	}
	if !*noService {
		if err := service.Preflight(); err != nil {
			fmt.Fprintf(os.Stderr, "netferry: %v\n", err)
			return 1
		}
	}
	_, statErr := os.Stat(configPath)
	configExisted := statErr == nil
	settings, detected, err := initialSettingsForInstall(configPath, !*noDetect, *upstreamFlag, *dryRun)
	if err != nil {
		fmt.Fprintf(os.Stderr, "netferry: %v\n", err)
		return 1
	}
	printDetectedProxy(detected)
	if detected == nil && strings.TrimSpace(*upstreamFlag) != "" {
		fmt.Printf("upstream: %s (manual)\n", settings.Upstream)
	}
	warnUnreachableUpstream(settings, detected, *upstreamFlag)

	if !*dryRun {
		if err := config.WriteNew(configPath, settings); err != nil && !errors.Is(err, config.ErrAlreadyExists) {
			fmt.Fprintf(os.Stderr, "netferry: create config: %v\n", err)
			return 1
		}
	}

	fmt.Printf("configuration: %s\n", configPath)
	if !*noService {
		executable, err := os.Executable()
		if err != nil {
			fmt.Fprintf(os.Stderr, "netferry: find executable: %v\n", err)
			return 1
		}
		messages, err := service.Install(service.Spec{
			Executable: executable,
			ConfigPath: configPath,
		}, *dryRun)
		if err != nil {
			fmt.Fprintf(os.Stderr, "netferry: %v\n", err)
			printInstallIncomplete(configPath, configExisted, !*noEnvironment)
			return 1
		}
		printServiceMessages(service.ManagerName(), messages, *dryRun)
	}
	if !*noEnvironment {
		messages, err := agentenv.Install(settings.Listen, *dryRun)
		if err != nil {
			fmt.Fprintf(os.Stderr, "netferry: %v\n", err)
			if !*noService {
				fmt.Fprintln(os.Stderr, "netferry: automatic startup was installed, but proxy environment variables were not")
			}
			return 1
		}
		printServiceMessages(agentenv.ManagerName(), messages, *dryRun)
	}
	return 0
}

func loadEffectiveSettings(path string) (config.Settings, error) {
	settings := config.Default()
	fileSettings, err := config.LoadOptional(path)
	if err != nil {
		return config.Settings{}, err
	}
	settings.Apply(fileSettings)
	environment, err := config.FromEnvironment()
	if err != nil {
		return config.Settings{}, err
	}
	settings.Apply(environment)
	if err := settings.Validate(); err != nil {
		return config.Settings{}, err
	}
	return settings, nil
}

func initialSettingsForInstall(path string, autoDetect bool, upstreamOverride string, dryRun bool) (config.Settings, *detect.Candidate, error) {
	upstreamOverride = strings.TrimSpace(upstreamOverride)
	exists := false
	if _, err := os.Stat(path); err == nil {
		exists = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return config.Settings{}, nil, fmt.Errorf("inspect config %s: %w", path, err)
	}

	if exists {
		settings, err := loadEffectiveSettings(path)
		if err != nil {
			return config.Settings{}, nil, err
		}
		if upstreamOverride != "" {
			settings.Upstream = upstreamOverride
			if err := settings.Validate(); err != nil {
				return config.Settings{}, nil, err
			}
			if !dryRun {
				if err := config.UpdateUpstream(path, upstreamOverride); err != nil {
					return config.Settings{}, nil, fmt.Errorf("update upstream: %w", err)
				}
			}
		}
		return settings, nil, nil
	}

	settings := config.Default()
	environment, err := config.FromEnvironment()
	if err != nil {
		return config.Settings{}, nil, err
	}
	settings.Apply(environment)
	if upstreamOverride != "" {
		settings.Upstream = upstreamOverride
	}

	var detected *detect.Candidate
	if upstreamOverride == "" && autoDetect && strings.TrimSpace(os.Getenv("NETFERRY_UPSTREAM")) == "" {
		candidates := detect.DetectExcept(context.Background(), 500*time.Millisecond, settings.Listen)
		found := false
		for index := range candidates {
			if candidates[index].Upstream != "" {
				settings.Upstream = candidates[index].Upstream
				detected = &candidates[index]
				found = true
				break
			}
		}
		if !found {
			return config.Settings{}, nil, fmt.Errorf(
				"no local proxy detected\n"+
					"Start your proxy app (the system proxy setting can stay off), then retry.\n"+
					"Or pass --upstream <url>. Use --no-detect to keep the default upstream %s.",
				config.DefaultUpstream,
			)
		}
	}
	if err := settings.Validate(); err != nil {
		return config.Settings{}, nil, err
	}
	return settings, detected, nil
}

func printDetectedProxy(candidate *detect.Candidate) {
	if candidate == nil {
		return
	}
	fmt.Printf("detected upstream: %s (%s)\n", candidate.Upstream, candidate.Software)
}

func printInstallIncomplete(configPath string, configExisted, includeEnvironment bool) {
	if !configExisted {
		fmt.Fprintf(os.Stderr, "netferry: configuration was created at %s\n", configPath)
	}
	if includeEnvironment {
		fmt.Fprintln(os.Stderr, "netferry: automatic startup and proxy environment variables were not installed; fix the error above and rerun")
		return
	}
	fmt.Fprintln(os.Stderr, "netferry: automatic startup was not installed; fix the error above and rerun")
}

func warnUnreachableUpstream(settings config.Settings, detected *detect.Candidate, upstreamOverride string) {
	if detected != nil || strings.TrimSpace(upstreamOverride) != "" {
		return
	}
	address, ok := upstreamAddress(settings.Upstream)
	if !ok {
		return
	}
	if _, _, reachable := detect.Probe(context.Background(), address, 500*time.Millisecond); reachable {
		return
	}
	fmt.Fprintf(os.Stderr, "warning: upstream %s is not reachable\n", redactProxyURL(settings.Upstream))
	fmt.Fprintln(os.Stderr, "  Start your proxy app, or update it with: netferry config set upstream <url>")
}

func upstreamAddress(raw string) (string, bool) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return "", false
	}
	if _, _, err := net.SplitHostPort(parsed.Host); err != nil {
		return "", false
	}
	return parsed.Host, true
}

func runUninstall(args []string) int {
	fs := flag.NewFlagSet("uninstall", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	dryRun := fs.Bool("dry-run", false, "show actions without changing the system")
	keepEnvironment := fs.Bool("keep-env", false, "keep installed proxy environment variables")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if !*keepEnvironment {
		messages, err := agentenv.Uninstall(*dryRun)
		if err != nil {
			fmt.Fprintf(os.Stderr, "netferry: %v\n", err)
			return 1
		}
		printServiceMessages(agentenv.ManagerName(), messages, *dryRun)
	}

	messages, err := service.Uninstall(*dryRun)
	if err != nil {
		fmt.Fprintf(os.Stderr, "netferry: %v\n", err)
		return 1
	}
	printServiceMessages(service.ManagerName(), messages, *dryRun)
	return 0
}

func runService(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: netferry service <start|stop|status> [--dry-run]")
		return 2
	}
	action := args[0]
	fs := flag.NewFlagSet("service "+action, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	dryRun := fs.Bool("dry-run", false, "show actions without changing the system")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}

	switch action {
	case "start":
		messages, err := service.Start(*dryRun)
		if err != nil {
			fmt.Fprintf(os.Stderr, "netferry: %v\n", err)
			return 1
		}
		printServiceMessages(service.ManagerName(), messages, *dryRun)
		return 0
	case "stop":
		messages, err := service.Stop(*dryRun)
		if err != nil {
			fmt.Fprintf(os.Stderr, "netferry: %v\n", err)
			return 1
		}
		printServiceMessages(service.ManagerName(), messages, *dryRun)
		return 0
	case "status":
		status, err := service.CurrentStatus()
		if err != nil {
			fmt.Fprintf(os.Stderr, "netferry: %v\n", err)
			return 1
		}
		fmt.Printf("manager:   %s\n", status.Manager)
		fmt.Printf("installed: %s\n", installedText(status.Installed))
		if status.Detail != "" {
			fmt.Printf("detail:    %s\n", status.Detail)
		}
		if !status.Installed {
			return 1
		}
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown service command %q; use start, stop, or status\n", action)
		return 2
	}
}

func printServiceMessages(manager string, messages []string, dryRun bool) {
	if dryRun {
		fmt.Printf("%s dry run:\n", manager)
	} else {
		fmt.Printf("%s:\n", manager)
	}
	for _, message := range messages {
		fmt.Printf("  - %s\n", message)
	}
}

func installedText(installed bool) string {
	if installed {
		return "installed"
	}
	return "not installed"
}

func newProxyHandler(listen, upstreamURL string, probeTimeout, cacheTTL time.Duration, logger *slog.Logger) (http.Handler, error) {
	proxyDialer, err := upstream.New(upstreamURL)
	if err != nil {
		return nil, err
	}

	directDialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	routeRouter := router.New(router.Options{
		ProbeTimeout: probeTimeout,
		CacheTTL:     cacheTTL,
		Probe:        router.NewDirectProbe(probeTimeout),
		DirectDial:   directDialer.DialContext,
		ProxyDial:    proxyDialer.DialContext,
		Logger:       logger,
	})

	return &proxy.Server{
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			conn, _, err := routeRouter.DialContext(ctx, network, address)
			return conn, err
		},
		Logger: logger,
	}, nil
}

func ensureServer(settings config.Settings, logger *slog.Logger) (func(), error) {
	running, _, err := netFerryListeningDetail(settings.Listen)
	if err == nil && running {
		return func() {}, nil
	}
	if err != nil {
		return nil, err
	}

	handler, err := newProxyHandler(settings.Listen, settings.Upstream, settings.ProbeTimeout, settings.CacheTTL, logger)
	if err != nil {
		return nil, err
	}

	listener, err := net.Listen("tcp", settings.Listen)
	if err != nil {
		return nil, fmt.Errorf("start local gateway: %w", err)
	}

	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("local gateway stopped", "error", err)
		}
	}()

	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}, nil
}

func netFerryListeningDetail(address string) (bool, string, error) {
	conn, err := net.DialTimeout("tcp", address, 200*time.Millisecond)
	if err == nil {
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(time.Second))
		if _, err := io.WriteString(conn, "GET /__netferry/health HTTP/1.1\r\nHost: netferry.local\r\nConnection: close\r\n\r\n"); err != nil {
			return false, "", err
		}
		body, err := io.ReadAll(io.LimitReader(conn, 4096))
		if err != nil {
			return false, "", err
		}
		if strings.Contains(string(body), "netferry ok") {
			return true, "health check succeeded", nil
		}
		return false, "", fmt.Errorf("address %s is already used by another service", address)
	}
	if isConnectionRefused(err) {
		return false, "connection refused", nil
	}
	return false, "", err
}

func isConnectionRefused(err error) bool {
	var netErr *net.OpError
	if !errors.As(err, &netErr) {
		return false
	}
	return strings.Contains(strings.ToLower(netErr.Err.Error()), "refused") ||
		strings.Contains(strings.ToLower(netErr.Err.Error()), "actively refused")
}

func proxyEnvironment(current []string, address string) []string {
	proxyValue := address
	localBypass := "localhost,127.0.0.1,::1"
	values := map[string]string{
		"HTTP_PROXY":  proxyValue,
		"HTTPS_PROXY": proxyValue,
		"ALL_PROXY":   proxyValue,
		"http_proxy":  proxyValue,
		"https_proxy": proxyValue,
		"all_proxy":   proxyValue,
		"NO_PROXY":    localBypass,
		"no_proxy":    localBypass,
	}

	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(current)+len(values))
	for _, item := range current {
		name, value, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		if replacement, exists := values[name]; exists {
			if name == "NO_PROXY" || name == "no_proxy" {
				replacement = mergeNoProxy(value, replacement)
			}
			result = append(result, name+"="+replacement)
			seen[name] = true
			continue
		}
		result = append(result, item)
	}

	for name, value := range values {
		if !seen[name] {
			result = append(result, name+"="+value)
		}
	}
	return result
}

func mergeNoProxy(current, defaults string) string {
	items := make([]string, 0, 8)
	seen := make(map[string]bool)
	for _, group := range []string{current, defaults} {
		for _, item := range strings.Split(group, ",") {
			item = strings.TrimSpace(item)
			if item == "" || seen[strings.ToLower(item)] {
				continue
			}
			seen[strings.ToLower(item)] = true
			items = append(items, item)
		}
	}
	return strings.Join(items, ",")
}

func normalizeAddress(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("empty address")
	}

	if strings.Contains(raw, "://") {
		parsed, err := url.Parse(raw)
		if err != nil {
			return "", err
		}
		raw = parsed.Host
	}

	if _, _, err := net.SplitHostPort(raw); err == nil {
		return raw, nil
	}
	raw = strings.TrimPrefix(strings.TrimSuffix(raw, "]"), "[")
	if strings.Count(raw, ":") > 1 {
		return net.JoinHostPort(raw, "443"), nil
	}
	if strings.Contains(raw, ":") {
		return "", fmt.Errorf("address must include a valid port")
	}
	return net.JoinHostPort(raw, "443"), nil
}

func redactProxyURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "<invalid>"
	}
	if parsed.User != nil {
		parsed.User = url.User("redacted")
	}
	return parsed.String()
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

func status(err error) string {
	if err == nil {
		return "ok"
	}
	return "failed"
}

func formatDuration(duration time.Duration) string {
	if duration <= 0 {
		return "-"
	}
	return duration.Round(time.Millisecond).String()
}

func defaultShell() string {
	if runtime.GOOS == "windows" {
		return "powershell"
	}
	return "bash"
}

func printUsage() {
	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()

	fmt.Fprintln(out, "NetFerry - Direct first, proxy fallback.")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Usage:")
	fmt.Fprintln(out, "  netferry serve   [flags]")
	fmt.Fprintln(out, "  netferry run     [flags] -- <command> [args...]")
	fmt.Fprintln(out, "  netferry doctor  [flags] [host...]")
	fmt.Fprintln(out, "  netferry env     [flags]")
	fmt.Fprintln(out, "  netferry status  [flags]")
	fmt.Fprintln(out, "  netferry config  <path|show|init|set> [flags]")
	fmt.Fprintln(out, "  netferry detect  [--json] [--timeout duration]")
	fmt.Fprintln(out, "  netferry setup   [--dry-run] [--no-service] [--no-env] [--no-detect] [--upstream url]")
	fmt.Fprintln(out, "  netferry install [--dry-run] [--no-detect] [--upstream url]")
	fmt.Fprintln(out, "  netferry uninstall [--dry-run] [--keep-env]")
	fmt.Fprintln(out, "  netferry service <start|stop|status> [--dry-run]")
	fmt.Fprintln(out, "  netferry version")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Common flags:")
	fmt.Fprintln(out, "  --config string     configuration file path")
	fmt.Fprintf(out, "  --upstream string   upstream proxy URL (default %s)\n", defaultUpstream)
	fmt.Fprintf(out, "  --listen string     local proxy address (default %s)\n", defaultListen)
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Environment:")
	fmt.Fprintln(out, "  NETFERRY_CONFIG, NETFERRY_UPSTREAM, NETFERRY_LISTEN")
	fmt.Fprintln(out, "  NETFERRY_PROBE_TIMEOUT, NETFERRY_CACHE_TTL")
}
