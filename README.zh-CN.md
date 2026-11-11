# NetFerry

[English](README.md) | **简体中文**

**优先直连，失败自动回退代理。**

NetFerry 是一款与 Agent 无关的自适应网络出口网关，专为开发工具设计。当目标地址可以直连时，它会保持直连；当目标地址不可达时，它会自动回退到本地代理。

```text
Codex / Claude Code / DeepSeek harness / 任意 CLI
                    |
          HTTP_PROXY / HTTPS_PROXY
                    |
            NetFerry :17890
               /         \
            直连       上游代理
```

Agent 不需要判断当前是否需要代理。所有请求都会先经过同一个本地网关，由网关探测直连连通性、缓存路由结果，并在必要时回退到上游 SOCKS5 或 HTTP 代理。

## 项目状态

NetFerry 目前处于早期开发阶段。当前 MVP 提供以下能力：

- 仅接受本机回环地址连接的 HTTP CONNECT 代理
- 优先直连，并在失败时自动回退代理
- 支持 SOCKS5 和 HTTP 上游代理
- 按目标主机缓存路由结果
- 提供 `doctor`、`status`、`env` 和通用命令启动能力
- 持久化 TOML 配置
- 支持 Windows、macOS 和 Linux 用户级自动启动
- 不依赖 Agent SDK、MCP 服务或特定模型的 Hook

非 CONNECT 的普通 HTTP 转发、SSH `ProxyCommand` 以及操作系统级透明重定向已经列入后续路线。

## 构建

```powershell
go build -o bin/netferry.exe ./cmd/netferry
```

## 快速开始

使用监听在 `socks5h://127.0.0.1:10808` 的代理程序启动网关：

```powershell
.\bin\netferry.exe serve --upstream socks5h://127.0.0.1:10808
```

检查访问 GitHub 时的直连和代理路径：

```powershell
.\bin\netferry.exe doctor github.com api.github.com codeload.github.com
```

安装本地网关，并配置为用户登录后自动启动：

```powershell
.\bin\netferry.exe setup --dry-run
.\bin\netferry.exe setup
```

`setup` 会同时安装自动启动服务和用户级代理环境。安装后需要重启 Agent
应用，让新进程继承环境变量。使用 `--no-service` 或 `--no-env` 可以只安装其中一部分。

使用 `status` 查看代理和后台服务状态：

```powershell
.\bin\netferry.exe status
.\bin\netferry.exe status --json
```

通过临时网关启动任意编程 Agent：

```powershell
.\bin\netferry.exe run -- codex
.\bin\netferry.exe run -- claude
```

子进程会继承 `HTTP_PROXY`、`HTTPS_PROXY`、`ALL_PROXY` 以及对应的小写环境变量。如果本地 NetFerry 实例尚未监听，`run` 会自动启动一个临时的本地网关。

## 为 Codex 配置 Shell 命令

如果你希望 Codex 持久使用 NetFerry，可以将它启动的 Shell 命令指向本地网关：

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

其他工具可以使用以下命令生成环境变量配置：

```powershell
.\bin\netferry.exe env --shell powershell
```

或者：

```bash
./bin/netferry env --shell bash
```

## 命令

```text
netferry serve    启动本地自适应代理
netferry run      在代理环境中运行任意命令
netferry doctor   测试直连和上游代理路径
netferry env      输出 Shell 环境变量配置
netferry status   查看代理和后台服务状态
netferry config   查看、显示或初始化配置
netferry setup    安装自动启动和代理环境
netferry install  安装用户级自动启动
netferry uninstall 删除自动启动并恢复受管理的环境变量
netferry service  启动、停止或检查后台服务
netferry version  输出当前版本
```

默认配置：

```text
监听地址:   127.0.0.1:17890
上游代理:   socks5h://127.0.0.1:10808
探测超时:   1.5s
路由缓存:   5m
```

也可以通过以下环境变量进行配置：

```text
NETFERRY_LISTEN
NETFERRY_UPSTREAM
NETFERRY_PROBE_TIMEOUT
NETFERRY_CACHE_TTL
```

配置文件保存在当前平台的用户配置目录。可以使用 `netferry config path`
查看路径、使用 `netferry config init` 创建配置，或者复制
`netferry.example.toml`。配置优先级依次为：命令行参数、环境变量、配置文件、
内置默认值。

## 安全性

- NetFerry 只接受来自回环地址的客户端连接。
- HTTPS 流量通过 CONNECT 隧道转发，NetFerry 不会终止连接或检查 TLS 内容。
- 代理凭据不会出现在普通日志中。
- 连接建立并开始传输应用数据后，NetFerry 不会重放请求。

## 许可证

Apache-2.0
