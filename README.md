# NetFerry

**English** | [简体中文](README.zh-CN.md)

**Direct first, proxy fallback.**

NetFerry is an agent-independent adaptive egress gateway for coding tools. It
keeps the direct route when it works and automatically falls back to a local
proxy when the destination is unreachable.

```text
Codex / Claude Code / DeepSeek harness / any CLI
                    |
          HTTP_PROXY / HTTPS_PROXY
                    |
            NetFerry :17890
               /         \
          direct       upstream proxy
```

The agent does not need to decide whether a proxy is required. Every request
starts at the same local gateway, which caches route decisions, probes direct
connectivity, and falls back to an upstream SOCKS5 or HTTP proxy.

## Status

NetFerry is in early development. The current MVP provides:

- A loopback-only HTTP CONNECT proxy
- Direct-first routing with automatic proxy fallback
- SOCKS5 and HTTP upstream proxies
- Per-host route caching
- `doctor`, `status`, `env`, and generic command-launch commands
- Persistent TOML configuration
- User-level automatic startup on Windows, macOS, and Linux
- No dependency on an Agent SDK, MCP server, or model-specific hook

HTTP proxying for non-CONNECT requests, SSH `ProxyCommand`, and transparent
OS-level redirection are on the roadmap.

## Build

```powershell
go build -o bin/netferry.exe ./cmd/netferry
```

## Quick start

Start the gateway with the proxy application listening on
`socks5h://127.0.0.1:10808`:

```powershell
.\bin\netferry.exe serve --upstream socks5h://127.0.0.1:10808
```

Check both routes for GitHub:

```powershell
.\bin\netferry.exe doctor github.com api.github.com codeload.github.com
```

Install the local gateway and configure it to start after user login:

```powershell
.\bin\netferry.exe install --dry-run
.\bin\netferry.exe install
```

Use `status` to inspect the proxy and service:

```powershell
.\bin\netferry.exe status
.\bin\netferry.exe status --json
```

Launch any coding agent through a temporary gateway:

```powershell
.\bin\netferry.exe run -- codex
.\bin\netferry.exe run -- claude
```

The child process inherits `HTTP_PROXY`, `HTTPS_PROXY`, `ALL_PROXY`, and their
lowercase variants. `run` starts an ephemeral gateway when no local NetFerry
instance is already listening.

## Configure Codex shell commands

For persistent use with Codex, point spawned shell commands at NetFerry:

```toml
[shell_environment_policy]
inherit = "core"
set = {
  HTTP_PROXY = "http://127.0.0.1:17890",
  HTTPS_PROXY = "http://127.0.0.1:17890",
  ALL_PROXY = "http://127.0.0.1:17890",
  NO_PROXY = "localhost,127.0.0.1,::1"
}
```

Other tools can use the output of:

```powershell
.\bin\netferry.exe env --shell powershell
```

or:

```bash
./bin/netferry env --shell bash
```

## Commands

```text
netferry serve    Run the local adaptive proxy
netferry run      Run any command with the local proxy environment
netferry doctor   Test direct and upstream routes
netferry env      Print shell environment setup
netferry status   Show proxy and service status
netferry config   Locate, show, or initialize configuration
netferry install  Install user-level automatic startup
netferry service  Start, stop, or inspect the startup service
netferry version  Print the version
```

Defaults:

```text
listen:   127.0.0.1:17890
upstream: socks5h://127.0.0.1:10808
timeout:  1.5s
cache:    5m
```

Configuration can also be provided through `NETFERRY_LISTEN`,
`NETFERRY_UPSTREAM`, `NETFERRY_PROBE_TIMEOUT`, and `NETFERRY_CACHE_TTL`.

The configuration file is stored in the platform user configuration directory.
Use `netferry config path` to locate it, `netferry config init` to create it, or
copy `netferry.example.toml`. Precedence is command-line flags, environment
variables, configuration file, then built-in defaults.

## Security

- NetFerry only accepts clients from loopback addresses.
- HTTPS is tunneled with CONNECT; NetFerry does not terminate or inspect TLS.
- Proxy credentials are redacted from normal logs.
- Application requests are never replayed after traffic has started.

## License

Apache-2.0
