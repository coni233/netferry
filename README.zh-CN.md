# NetFerry

[English](README.md) | **简体中文**

**优先直连，失败自动回退代理。**

NetFerry 是一款与 Agent 无关的自适应网络出口网关，专为开发工具设计。当目标地址可以直连时，它会保持直连；当目标地址不可达时，它会自动回退到本地代理。

```text
终端命令
Codex / Claude Code / DeepSeek harness / 任意 CLI
                        |
                  产生网络请求
                        |
  HTTP_PROXY / HTTPS_PROXY -> 配置的 NetFerry 本地地址
                        |
                 NetFerry 本地网关
                    /          \
              直连可用         直连失败
                  |                |
            直接访问目标      配置的上游代理
                                     |
                                 访问目标
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

## 安装正式版本

从 [GitHub Releases](https://github.com/coni233/netferry/releases) 下载与系统对应的压缩包：

- Windows：解压 `.zip`，双击 `install.bat`。以后需要卸载时双击 `uninstall.bat`。
- Linux 和 macOS：解压 `.tar.gz`，运行 `./install.sh`。
  以后需要卸载时运行 `./uninstall.sh`。

发布包中包含 README 和许可证文件。`checksums.txt` 包含每个压缩包的 SHA-256 校验值。

## 使用前准备

使用正式发布包时不需要安装 Go。

运行 `setup` 或安装脚本前，请先启动代理软件。系统代理开关不需要打开，
NetFerry 会直接探测本地监听端口。

默认配置假定本机代理地址为：

```text
socks5h://127.0.0.1:10808
```

`10808` 是较新版本 v2rayN 默认的本地混合监听端口。其他代理软件通常使用不同端口，
因此需要在代理软件中确认本地监听端口，并在必要时修改 NetFerry 配置。

常见默认端口如下：

| 代理软件 | 常见本地端口 | 说明 |
| --- | --- | --- |
| v2rayN（较新版本） | `10808` | 通常是 SOCKS5 和 HTTP 共用的混合端口 |
| 较旧 v2rayN / V2Ray 配置 | SOCKS5 `10808`、HTTP `10809` | 实际端口由配置决定 |
| Clash、Clash Verge、Mihomo | `7890` | 常见 HTTP/SOCKS 混合端口；拆分配置有时使用 SOCKS `7891` |
| Clash Verge Rev / Kiri Verge | `7897` | 较新版本的默认 HTTP/SOCKS 混合端口 |
| sing-box | 没有统一默认值 | 由 inbound 配置决定，`2080` 是常见示例 |
| NekoBox / Nekoray | 没有统一默认值 | 由 inbound 配置决定，`2080` 是常见示例 |
| Shadowsocks Windows / SSR | `1080` | 常见本地 SOCKS5 端口，不同版本可能不同 |
| V2Ray / Xray | 没有统一默认值 | 由配置决定，SOCKS `1080` 和 HTTP `8080` 是常见示例 |

这些端口是常见约定，并不是所有版本都保证如此。请以代理软件显示的本地监听设置为准。

可以运行内置检测命令查看当前可用的常见代理端口：

```powershell
.\netferry.exe detect
```

如果配置文件尚不存在，`setup` 会自动使用检测到的第一个可用代理。
检测会先读取操作系统代理设置（Windows 注册表、macOS `scutil`、Linux 的
GNOME/KDE 设置）以及 `HTTPS_PROXY`、`HTTP_PROXY`、`ALL_PROXY` 环境变量，再回退到
v2rayN、Clash/Mihomo、Shadowsocks、sing-box/NekoBox 的常见端口，并逐个探测 SOCKS5
和 HTTP 支持情况。如果什么都没检测到，`setup` 会直接报错并停止，不会静默写入默认值。
启动代理软件后重试，或者传入 `--upstream <url>` 手动指定；也可以使用 `--no-detect`
接受默认值。

如果配置文件已经存在，`setup` 会直接复用，不会再次自动检测。如果配置的上游代理
当前不可达，会输出警告；可以用 `netferry config set upstream <url>` 或
`setup --upstream <url>` 更新。

配置时要使用与端口匹配的协议：

```toml
# SOCKS5
upstream = "socks5h://127.0.0.1:10808"

# HTTP 代理
upstream = "http://127.0.0.1:10808"
```

即使上游代理程序没有运行，目标可以直连时 NetFerry 仍然可以正常工作。

## 一次性安装

### Windows

#### 自动安装（推荐）

1. 从 GitHub Releases 下载 `netferry-windows-amd64.zip`。
2. 解压到长期保留的目录，例如 `C:\Tools\NetFerry`。
3. 启动代理软件，系统代理开关可以保持关闭。
4. 双击 `install.bat`。
5. 查看检测到的代理和安装预览，然后输入 `Y` 确认。
6. 完全退出并重新启动 Codex、Claude Code、DeepSeek harness、终端、IDE
   以及其他需要继承新环境的程序。

不需要再手动执行 `setup`。`install.bat` 会自动完成代理检测、
`setup --dry-run`、确认和正式 `setup`。

如果检测失败，安装程序会提供三个选项：

- `R`：启动代理软件后重新检测。
- `S`：跳过检测，使用默认上游继续安装；之后可以用
  `netferry setup --upstream <url>` 修正。
- `Q`：退出安装，不修改系统。

检测成功时，输入 `Y` 确认安装。

#### 手动安装

如果希望在 PowerShell 中手动操作：

```powershell
.\netferry.exe setup --dry-run
.\netferry.exe setup

# 检测不到时可以手动指定上游代理：
.\netferry.exe setup --dry-run --upstream http://127.0.0.1:10808
.\netferry.exe setup --upstream http://127.0.0.1:10808
```

手动安装与双击 `install.bat` 执行的操作相同。

`setup` 会创建默认配置、注册当前用户的登录启动命令、启动网关，并写入持久的
用户级代理环境变量，不需要管理员权限。

在 Windows 上，`netferry.exe setup` 具体会：

1. 创建 NetFerry 配置文件。
2. 添加只对当前 Windows 用户生效的 `HKCU\...\Run` 登录启动命令。
3. 立即启动 NetFerry 网关。
4. 为当前 Windows 用户设置持久代理环境变量：
   `HTTP_PROXY`、`HTTPS_PROXY`、`ALL_PROXY`、对应的小写变量，以及
   `NO_PROXY` / `no_proxy`。

它不会修改整台电脑所有用户共用的机器级环境变量。已经运行的程序不会自动获得
新环境，因此安装后需要重新启动它们再测试。

### Linux 和 macOS

```bash
tar -xzf netferry-linux-amd64.tar.gz
cd netferry-linux-amd64
chmod +x netferry
./install.sh
```

请选择匹配系统和 CPU 架构的压缩包。Linux 用户需要注销并重新登录，使桌面会话
加载 `environment.d`。macOS 用户安装后需要重启 Agent 应用。

检测不到本地代理时，`install.sh` 也会显示同样的 `R` / `S` / `Q` 选项。

## 验证安装

打开一个新的终端：

```powershell
.\netferry.exe status
```

正常状态类似：

```text
status:      running
listen:      127.0.0.1:17890
proxy:       http://127.0.0.1:17890
upstream:    socks5h://127.0.0.1:10808
service:     Windows logon command (installed)
environment: Windows user environment (installed)
```

检查新进程是否继承了环境变量：

```powershell
$env:HTTPS_PROXY
```

应当输出：

```text
http://127.0.0.1:17890
```

检查直连和代理路径：

```powershell
.\netferry.exe doctor github.com api.github.com codeload.github.com
```

最后使用真实命令测试：

```powershell
git ls-remote https://github.com/coni233/netferry.git HEAD
```

## 日常使用

安装完成后，按照平常习惯启动工具并执行命令：

```powershell
git clone https://github.com/example/project.git
curl https://github.com
npm install
pip install requests
gh repo view
```

你不需要再提醒 Agent 开启代理。只要程序支持标准代理环境变量，它们就会把
请求发送到 `127.0.0.1:17890`。NetFerry 再根据每个目标决定直连还是使用上游代理。

NetFerry 不是 VPN，也不是命令包装器。没有网络活动的命令不受影响；
完全忽略标准代理变量的程序不会被自动接管。

## 临时使用

如果不想安装持久启动项和环境变量，可以只临时运行某一组命令：

```powershell
.\netferry.exe run -- git clone https://github.com/example/project.git
.\netferry.exe run -- codex
.\netferry.exe run -- claude
```

如果本地 NetFerry 尚未运行，`run` 会自动启动临时网关。子进程会继承大写和小写的
代理环境变量。

也可以只启动网关：

```powershell
.\netferry.exe serve
```

## 修改配置

查看自动生成的配置文件：

```powershell
.\netferry.exe config path
```

如果只想更新上游代理，不必手动编辑文件：

```powershell
.\netferry.exe config set upstream http://127.0.0.1:10808
```

`setup --upstream <url>` 也会更新已有配置。

使用文本编辑器修改：

```toml
listen = "127.0.0.1:17890"
upstream = "socks5h://127.0.0.1:10808"
probe_timeout = "1500ms"
cache_ttl = "5m"
```

修改后重启网关：

```powershell
.\netferry.exe service stop
.\netferry.exe service start
```

配置优先级依次为：命令行参数、环境变量、配置文件、内置默认值。

## 控制后台服务

```powershell
.\netferry.exe status
.\netferry.exe service start
.\netferry.exe service stop
.\netferry.exe service status
```

## 卸载

### 自动卸载（推荐）

- Windows：双击 `uninstall.bat`。
- Linux 和 macOS：运行 `./uninstall.sh`。

脚本会自动执行 `uninstall --dry-run`，显示准备修改的内容，并在确认后删除自动启动项
和恢复受管理的代理环境变量。不需要再手动执行其他卸载命令。

### 手动卸载

```powershell
.\netferry.exe uninstall --dry-run
.\netferry.exe uninstall
```

手动卸载与双击 `uninstall.bat` 执行的操作相同。

卸载会删除自动启动项，并恢复仍由 NetFerry 管理的用户级代理环境变量：

- `--dry-run` 只显示准备恢复和删除的操作，不修改电脑。
- 安装前不存在的变量会被删除。
- 安装前已经存在的变量会恢复为保存的原值。
- 安装后手动修改过的变量会保留新值，不会被覆盖。
- 只会处理当前 Windows 用户的环境变量。
- 已经运行的 Agent 和终端会保留旧环境，重新启动后才会生效。

卸载不会删除下载的 `netferry.exe`、配置文件或日志文件，也不会停止没有 PID
记录的手动 `netferry serve` 进程。
使用 `--keep-env` 可以只删除自动启动项并保留代理环境变量。

## 常见问题与限制

- `status` 显示 `stopped`：执行 `netferry service start`，并确认上游代理程序正在运行。
- Windows 自启动失败或网关没有启动：查看 `%APPDATA%\netferry\netferry.log`。
- 上游代理路径失败：检查配置中的代理地址，并运行 `netferry doctor github.com`。
- 安装后环境变量没有出现：完全退出原程序并重新启动；已经运行的进程不会自动更新环境。
- `git@github.com:owner/repo.git` 这类 SSH 地址不使用 `HTTPS_PROXY`，
  当前应使用 HTTPS clone 地址。
- `ping`、原始 TCP 客户端以及内置网络实现完全忽略标准代理变量的程序目前不会被接管。

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
.\netferry.exe env --shell powershell
```

或者：

```bash
./netferry env --shell bash
```

## 命令

```text
netferry serve    启动本地自适应代理
netferry run      在代理环境中运行任意命令
netferry doctor   测试直连和上游代理路径
netferry env      输出 Shell 环境变量配置
netferry status   查看代理和后台服务状态
netferry config   查看、显示、初始化或更新配置
netferry detect   检测常见本地代理端口和协议
netferry setup    安装自动启动和代理环境 [--upstream url]
netferry install  安装用户级自动启动 [--upstream url]
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
