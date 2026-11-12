# Architecture

## Problem

Developer tools fail differently when access to sites such as GitHub is
intermittent. Asking an Agent in every conversation to use a proxy is fragile:
each Agent has different instruction files, tool hooks, and model behavior.

NetFerry moves route selection below the Agent layer.

## Data path

1. A client connects to the loopback HTTP proxy.
2. For HTTPS, the client sends `CONNECT host:port`.
3. NetFerry checks its per-host route cache.
4. On a cache miss it probes the direct TCP route. Port 443 also requires a TLS
   handshake so a TCP connection reset during TLS negotiation counts as a
   failed direct route.
5. NetFerry opens the selected route and returns `200 Connection Established`.
6. Bytes are relayed without decrypting the TLS session.

If the selected route fails while opening a connection, NetFerry tries the
other route and updates its cache. It does not retry or replay application data
after the tunnel has started.

## Why not MCP?

MCP tools are selected by a model. An MCP server cannot intercept every shell
command an Agent chooses to execute. NetFerry therefore uses the standard proxy
environment protocol, which is supported by Git, curl, package managers, and
most programming-language HTTP clients.

MCP can later expose diagnostics such as current routes and probe results, but
it is not part of the network data path.

## Components

```text
cmd/netferry        CLI entry point
internal/config     flat TOML configuration and precedence
internal/detect     local proxy discovery (system settings, environment, common ports)
internal/router     direct/proxy route selection and cache
internal/service    Windows logon command, macOS LaunchAgent, systemd user service
internal/upstream   HTTP CONNECT and SOCKS5 upstream dialers
internal/proxy      loopback HTTP CONNECT server
```

## Startup integration

NetFerry starts at user login rather than installing a machine-wide daemon:

- Windows uses an `HKCU\...\Run` entry and a hidden PowerShell launcher, or a
  Startup folder launcher when administrator policy blocks registry editing,
  so no administrator rights are required.
- macOS uses a per-user LaunchAgent.
- Linux uses a systemd user service.

The executable is launched with `serve --config <path>`. Installation also
creates the configuration file if it does not exist. `install --dry-run` and
`setup --dry-run` print the intended actions without changing the system.

`setup` additionally installs the standard proxy environment:

- Windows writes persistent values under the current user environment.
- macOS updates the current launchd session and installs a login LaunchAgent.
- Linux writes a dedicated `environment.d` file.

The installer records previous environment values when the platform supports
overwriting existing variables. `uninstall` only restores variables that still
match the values managed by NetFerry, so later user changes are preserved.

## Safety invariants

- Bind only to loopback by default.
- Never decrypt HTTPS.
- Never log proxy credentials.
- Only retry before sending application data.
- Treat a failed direct probe as a temporary routing hint, not proof that all
  network access is unavailable.
