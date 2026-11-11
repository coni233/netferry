package main

import (
	"bufio"
	"context"
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

	"github.com/coni233/netferry/internal/proxy"
	"github.com/coni233/netferry/internal/router"
	"github.com/coni233/netferry/internal/upstream"
)

const (
	defaultListen       = "127.0.0.1:17890"
	defaultUpstream     = "socks5h://127.0.0.1:10808"
	defaultProbeTimeout = 1500 * time.Millisecond
	defaultCacheTTL     = 5 * time.Minute
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

	listen := fs.String("listen", envOrDefault("NETFERRY_LISTEN", defaultListen), "HTTP proxy listen address")
	upstreamURL := fs.String("upstream", envOrDefault("NETFERRY_UPSTREAM", defaultUpstream), "upstream proxy URL")
	probeTimeout := fs.Duration("probe-timeout", envDuration("NETFERRY_PROBE_TIMEOUT", defaultProbeTimeout), "direct connectivity probe timeout")
	cacheTTL := fs.Duration("cache-ttl", envDuration("NETFERRY_CACHE_TTL", defaultCacheTTL), "route cache lifetime")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	handler, err := newProxyHandler(*listen, *upstreamURL, *probeTimeout, *cacheTTL, logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "netferry: %v\n", err)
		return 1
	}

	server := &http.Server{
		Addr:              *listen,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	logger.Info("netferry listening", "address", *listen, "upstream", redactProxyURL(*upstreamURL))
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintf(os.Stderr, "netferry: %v\n", err)
		return 1
	}
	return 0
}

func runCommand(args []string) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	listen := fs.String("listen", envOrDefault("NETFERRY_LISTEN", defaultListen), "HTTP proxy listen address")
	upstreamURL := fs.String("upstream", envOrDefault("NETFERRY_UPSTREAM", defaultUpstream), "upstream proxy URL")
	probeTimeout := fs.Duration("probe-timeout", envDuration("NETFERRY_PROBE_TIMEOUT", defaultProbeTimeout), "direct connectivity probe timeout")
	cacheTTL := fs.Duration("cache-ttl", envDuration("NETFERRY_CACHE_TTL", defaultCacheTTL), "route cache lifetime")

	if err := fs.Parse(args); err != nil {
		return 2
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
	stop, err := ensureServer(*listen, *upstreamURL, *probeTimeout, *cacheTTL, logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "netferry: %v\n", err)
		return 1
	}
	defer stop()

	cmd := exec.Command(command[0], command[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = proxyEnvironment(os.Environ(), proxyURL(*listen))

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

	upstreamURL := fs.String("upstream", envOrDefault("NETFERRY_UPSTREAM", defaultUpstream), "upstream proxy URL")
	timeout := fs.Duration("timeout", envDuration("NETFERRY_PROBE_TIMEOUT", defaultProbeTimeout), "probe timeout")

	if err := fs.Parse(args); err != nil {
		return 2
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

	upstreamDialer, err := upstream.New(*upstreamURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "netferry: invalid upstream: %v\n", err)
		return 1
	}

	directProbe := router.NewDirectProbe(*timeout)
	failed := false

	fmt.Printf("upstream: %s\n\n", redactProxyURL(*upstreamURL))
	for _, input := range hosts {
		address, err := normalizeAddress(input)
		if err != nil {
			fmt.Printf("%-32s invalid: %v\n", input, err)
			failed = true
			continue
		}

		directCtx, cancelDirect := context.WithTimeout(context.Background(), *timeout)
		directDuration, directErr := directProbe(directCtx, address)
		cancelDirect()

		proxyCtx, cancelProxy := context.WithTimeout(context.Background(), *timeout)
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

	listen := fs.String("listen", envOrDefault("NETFERRY_LISTEN", defaultListen), "HTTP proxy listen address")
	shell := fs.String("shell", defaultShell(), "output shell: powershell or bash")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	value := proxyURL(*listen)
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

func ensureServer(listen, upstreamURL string, probeTimeout, cacheTTL time.Duration, logger *slog.Logger) (func(), error) {
	running, err := netFerryListening(listen)
	if err == nil && running {
		return func() {}, nil
	}
	if err != nil {
		return nil, err
	}

	handler, err := newProxyHandler(listen, upstreamURL, probeTimeout, cacheTTL, logger)
	if err != nil {
		return nil, err
	}

	listener, err := net.Listen("tcp", listen)
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

func netFerryListening(address string) (bool, error) {
	conn, err := net.DialTimeout("tcp", address, 200*time.Millisecond)
	if err == nil {
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(time.Second))
		if _, err := io.WriteString(conn, "GET /__netferry/health HTTP/1.1\r\nHost: netferry.local\r\nConnection: close\r\n\r\n"); err != nil {
			return false, err
		}
		body, err := io.ReadAll(io.LimitReader(conn, 4096))
		if err != nil {
			return false, err
		}
		if strings.Contains(string(body), "netferry ok") {
			return true, nil
		}
		return false, fmt.Errorf("address %s is already used by another service", address)
	}
	if isConnectionRefused(err) {
		return false, nil
	}
	return false, err
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

func envOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func envDuration(name string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return fallback
	}
	return value
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
	fmt.Fprintln(out, "  netferry version")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Common flags:")
	fmt.Fprintf(out, "  --upstream string   upstream proxy URL (default %s)\n", defaultUpstream)
	fmt.Fprintf(out, "  --listen string     local proxy address (default %s)\n", defaultListen)
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Environment:")
	fmt.Fprintln(out, "  NETFERRY_UPSTREAM, NETFERRY_LISTEN, NETFERRY_PROBE_TIMEOUT, NETFERRY_CACHE_TTL")
}
