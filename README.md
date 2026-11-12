# NetFerry

**English** | [简体中文](README.zh-CN.md)

**Direct first, proxy fallback.**

NetFerry is an agent-independent adaptive egress gateway for coding tools. It
keeps the direct route when it works and automatically falls back to a local
proxy when the destination is unreachable.

```text
Terminal commands
Codex / Claude Code / DeepSeek harness / any CLI
                        |
               create a network request
                        |
  HTTP_PROXY / HTTPS_PROXY -> configured NetFerry address
                        |
                 NetFerry local gateway
                    /          \
             direct works   direct fails
                  |              |
          access target    configured upstream proxy
                                 |
                           access target
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

## Install a release

Download the archive for your operating system from
[GitHub Releases](https://github.com/coni233/netferry/releases):

- Windows: extract the `.zip`, then double-click `install.bat`. To remove
  NetFerry later, double-click `uninstall.bat`.
- Linux and macOS: extract the `.tar.gz`, then run `./install.sh`. To remove
  NetFerry later, run `./uninstall.sh`.

Release archives include this README and the license. `checksums.txt` contains
the SHA-256 digest for every archive.

## Before you begin

You do not need Go to use a release archive.

Start your proxy application before running `setup` or the installer. You do
not need to enable its system proxy setting; NetFerry probes the local
listening port directly.

The default configuration expects a local proxy at:

```text
socks5h://127.0.0.1:10808
```

`10808` is the default mixed local port in recent v2rayN versions. Other proxy
applications commonly use different ports, so verify the local listening port
in your proxy application and change the configuration when necessary.

Common defaults include:

| Proxy software | Common local port | Notes |
| --- | --- | --- |
| v2rayN (recent versions) | `10808` | Usually one mixed port for SOCKS5 and HTTP |
| Older v2rayN / V2Ray setups | SOCKS5 `10808`, HTTP `10809` | Actual values come from the configuration |
| Clash, Clash Verge, Mihomo | `7890` | Mixed HTTP/SOCKS port; split setups sometimes use SOCKS `7891` |
| Clash Verge Rev / Kiri Verge | `7897` | Default mixed HTTP/SOCKS port in recent versions |
| sing-box | No universal default | Defined by the inbound configuration; `2080` is a common example |
| NekoBox / Nekoray | No universal default | Defined by the inbound configuration; `2080` is a common example |
| Shadowsocks Windows / SSR | `1080` | Common local SOCKS5 port; builds may differ |
| V2Ray / Xray | No universal default | Defined by the configuration; SOCKS `1080` and HTTP `8080` are common examples |

These values are conventions, not guarantees. Always check the proxy
application's local listening settings.

Run the built-in detector to inspect active common proxy ports:

```powershell
.\netferry.exe detect
```

When no configuration file exists, `setup` automatically uses the first usable
detected proxy. Detection reads the operating system proxy settings (Windows
registry, macOS `scutil`, and GNOME/KDE on Linux) plus the `HTTPS_PROXY`,
`HTTP_PROXY`, and `ALL_PROXY` environment variables, then falls back to common
v2rayN, Clash/Mihomo, Shadowsocks, and sing-box/NekoBox ports. Each endpoint is
probed for SOCKS5 and HTTP support. Use `--no-detect` when running `setup` to
keep the configured default instead. If detection finds nothing, `setup` stops
with an error instead of silently writing the default. Start the proxy
application and retry, pass `--upstream <url>`, or use `--no-detect` to accept
the default.

Use the matching URL scheme:

```toml
# SOCKS5
upstream = "socks5h://127.0.0.1:10808"

# HTTP proxy
upstream = "http://127.0.0.1:10808"
```

NetFerry still works without a running upstream proxy when the destination can
be reached directly.

## One-time setup

### Windows

#### Automatic installation (recommended)

1. Download `netferry-windows-amd64.zip` from GitHub Releases.
2. Extract it to a permanent directory, such as `C:\Tools\NetFerry`.
3. Start your proxy application. Its system proxy setting can stay off.
4. Double-click `install.bat`.
5. Review the detected proxy and installation preview, then answer `Y`.
6. Restart Codex, Claude Code, DeepSeek harness, terminals, IDEs, and other
   tools that should inherit the new environment.

No additional `setup` commands are required. The script runs detection,
`setup --dry-run`, confirmation, and `setup` automatically.

If detection fails, the installer offers three choices:

- `R`: retry detection after starting the proxy application.
- `S`: skip detection and install with the default upstream; you can correct
  it later with `netferry setup --upstream <url>`.
- `Q`: quit without changing the system.

When detection succeeds, answer `Y` to confirm the installation.

#### Manual installation

If you prefer PowerShell, preview and install manually:

```powershell
.\netferry.exe setup --dry-run
.\netferry.exe setup

# Or specify the upstream directly when detection cannot find it:
.\netferry.exe setup --dry-run --upstream http://127.0.0.1:10808
.\netferry.exe setup --upstream http://127.0.0.1:10808
```

Manual installation performs the same changes as `install.bat`.

`setup` creates the default configuration, installs a current-user login task,
starts the gateway, and writes persistent user-level proxy variables. It does
not require administrator privileges.

On Windows, `netferry.exe setup` specifically:

1. Creates the NetFerry configuration file.
2. Adds a login task that starts NetFerry for the current Windows user.
3. Starts the NetFerry gateway immediately.
4. Sets persistent proxy variables for the current Windows user:
   `HTTP_PROXY`, `HTTPS_PROXY`, `ALL_PROXY`, their lowercase variants, and
   `NO_PROXY` / `no_proxy`.

It does not modify machine-wide environment variables. Already-running
processes do not receive the new environment, so restart them before testing.

### Linux and macOS

```bash
tar -xzf netferry-linux-amd64.tar.gz
cd netferry-linux-amd64
chmod +x netferry
./install.sh
```

Use the archive matching your OS and architecture. On Linux, sign out and back
in so the desktop session loads `environment.d`. On macOS, restart Agent
applications after setup.

`install.sh` shows the same `R` / `S` / `Q` prompt when no local proxy is
detected.

## Verify the installation

Open a new terminal and run:

```powershell
.\netferry.exe status
```

A working installation looks like:

```text
status:      running
listen:      127.0.0.1:17890
proxy:       http://127.0.0.1:17890
upstream:    socks5h://127.0.0.1:10808
service:     Windows logon task (installed)
environment: Windows user environment (installed)
```

Check the environment inherited by the new process:

```powershell
$env:HTTPS_PROXY
```

It should print:

```text
http://127.0.0.1:17890
```

Check direct and proxy connectivity:

```powershell
.\netferry.exe doctor github.com api.github.com codeload.github.com
```

Then test with a real command:

```powershell
git ls-remote https://github.com/coni233/netferry.git HEAD
```

## Everyday use

After setup, launch your normal tools and execute commands normally:

```powershell
git clone https://github.com/example/project.git
curl https://github.com
npm install
pip install requests
gh repo view
```

You do not need to tell an Agent to enable the proxy. Network-capable programs
that honor `HTTP_PROXY`, `HTTPS_PROXY`, or `ALL_PROXY` send requests to
`127.0.0.1:17890`. NetFerry then selects a direct connection or the configured
upstream proxy for each destination.

This is not a VPN or a command wrapper. Commands without network activity are
unaffected, and programs that ignore standard proxy variables are not captured.

## Temporary use

To test one command group without installing persistent startup or environment
variables:

```powershell
.\netferry.exe run -- git clone https://github.com/example/project.git
.\netferry.exe run -- codex
.\netferry.exe run -- claude
```

`run` starts an ephemeral gateway when no NetFerry process is already running.
The child process inherits both uppercase and lowercase proxy variables.

You can also run only the gateway:

```powershell
.\netferry.exe serve
```

## Configuration

Find the generated file:

```powershell
.\netferry.exe config path
```

To change only the upstream proxy without editing the file:

```powershell
.\netferry.exe config set upstream http://127.0.0.1:10808
```

`setup --upstream <url>` also updates an existing configuration.

Edit it with a text editor:

```toml
listen = "127.0.0.1:17890"
upstream = "socks5h://127.0.0.1:10808"
probe_timeout = "1500ms"
cache_ttl = "5m"
```

After changing the configuration, restart the gateway:

```powershell
.\netferry.exe service stop
.\netferry.exe service start
```

Configuration precedence is command-line flags, environment variables,
configuration file, then built-in defaults.

## Service control

```powershell
.\netferry.exe status
.\netferry.exe service start
.\netferry.exe service stop
.\netferry.exe service status
```

## Uninstall

### Automatic uninstall (recommended)

- Windows: double-click `uninstall.bat`.
- Linux and macOS: run `./uninstall.sh`.

The script runs `uninstall --dry-run`, shows the planned changes, and waits for
confirmation before removing the startup integration and managed environment.
No manual commands are required.

### Manual uninstall

```powershell
.\netferry.exe uninstall --dry-run
.\netferry.exe uninstall
```

Manual uninstall performs the same changes as the release script.

This removes the startup integration and restores proxy variables whose current
values are still managed by NetFerry:

- `--dry-run` only prints the restoration and removal actions.
- A variable that did not exist before setup is removed.
- A variable that existed before setup is restored to its saved value.
- A variable changed manually after setup is left unchanged.
- Only the current user's variables are affected.
- Running Agent applications retain their old environment until restarted.

Uninstall does not delete the downloaded executable, configuration file, or log
files. It also does not stop a manually launched `netferry serve` process. Use
`--keep-env` to remove only the startup integration.

## Troubleshooting

- `status` says `stopped`: run `netferry service start` and check the upstream
  proxy application.
- The upstream route fails: verify the proxy address in the configuration and
  run `netferry doctor github.com`.
- A newly installed environment is not visible: close the tool completely and
  start a new process. Existing processes do not receive environment updates.
- Git over SSH such as `git@github.com:owner/repo.git` does not use
  `HTTPS_PROXY`; HTTPS clone URLs are currently the supported path.
- `ping`, raw TCP clients, and applications with built-in networking that
  ignores standard proxy variables are not intercepted yet.

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
.\netferry.exe env --shell powershell
```

or:

```bash
./netferry env --shell bash
```

## Commands

```text
netferry serve    Run the local adaptive proxy
netferry run      Run any command with the local proxy environment
netferry doctor   Test direct and upstream routes
netferry env      Print shell environment setup
netferry status   Show proxy and service status
netferry config   Locate, show, initialize, or update configuration
netferry detect   Detect common local proxy ports and protocols
netferry setup    Install automatic startup and proxy environment [--upstream url]
netferry install  Install user-level automatic startup [--upstream url]
netferry uninstall Remove startup and managed environment
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
