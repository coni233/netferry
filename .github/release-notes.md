## 该下载哪个版本？ / Which file should I download?

| 系统 | 下载文件 | 适用情况 |
| --- | --- | --- |
| Windows | `netferry-windows-amd64.zip` | 绝大多数 Intel/AMD 64 位 Windows |
| Windows on ARM | `netferry-windows-arm64.zip` | ARM 版 Windows 设备 |
| macOS（Apple 芯片） | `netferry-darwin-arm64.tar.gz` | M1 / M2 / M3 / M4 |
| macOS（Intel） | `netferry-darwin-amd64.tar.gz` | Intel 芯片 Mac |
| Linux x86_64 | `netferry-linux-amd64.tar.gz` | 大多数桌面和服务器 Linux |
| Linux ARM64 | `netferry-linux-arm64.tar.gz` | ARM64 Linux |
| 校验文件 | `checksums.txt` | 可选，用于校验 SHA-256 |

不确定的话：Windows 打开“设置 → 系统 → 关于 → 系统类型”；macOS 打开
“苹果菜单 → 关于本机”；Linux 执行 `uname -m`（`x86_64` 对应 amd64，
`aarch64` 对应 arm64）。

安装：Windows 解压 `.zip` 后双击 `install.bat`；Linux/macOS 解压 `.tar.gz`
后运行 `./install.sh`。安装前可以先启动代理软件，但不需要打开系统代理。

---
