# AI Passport 玩法社区（离线桌面版）

[AI Passport 玩法社区](https://ai-passport.folotoy.cn/plays/) 的本地桌面 App：把全部玩法、图片和固件下载到电脑上，之后不联网也能浏览玩法，并通过 USB 把玩法刷入 AI Passport（ESP32-C3）。

The offline desktop edition of the AI Passport App Library. It downloads every play, image and firmware to your computer, then lets you browse and install plays to an AI Passport over USB without a network.

## 功能

- **主页**：官方精选、专题页签（多人玩法 / 亲子益智 / 日常办公）、搜索、排序（最新 / 下载 / 点赞 / 讨论），布局与网站一致。
- **分类**：左侧分类（游戏与互动、效率与工具……）和专题（必玩精选等），带数量。
- **详情**：图集、玩法介绍、使用说明、更新日志、作者。
- **刷机**：选择串口 → 校验固件 → 检测芯片必须是 ESP32-C3、Flash 至少 8 MB、未开启安全启动 → 从 0x0 写入 → 回读校验 → 重启。一次刷一台。
- **设置**：选择 / 更换玩法存储目录（可迁移已有数据）、检查更新、语言（跟随系统 / 中文 / English）。
- **首次启动**：安装包里不带任何玩法。首次打开时引导用户选择目录并下载。

## 下载更新是怎么工作的

更新分两步，第一步完成后就可以浏览：

1. **玩法信息和图片**：分页拉取全部玩法（约 1,260 个），下载封面、缩略图、图集和作者头像（约 330 MB，约 2 分钟），写入 `index.json`。
2. **固件**：按下载量从高到低排队下载（约 3 GB），每个文件都校验大小、SHA-256、ESP32-C3 镜像头，通过后才原子替换到位。

之后再更新时只下载新增或有变化的玩法（比较 `revisionId` 和固件 SHA-256）。网站上已下架的玩法会保留在本地，并标记为"已下架"。

> **服务器限流**：社区的固件下载接口（`/api/download/...`）按 IP 限流。实测连续下载约 30 个后返回 HTTP 429，封禁会持续 25 分钟以上。App 遇到 429 会让所有固件下载一起暂停并逐步退避（30 秒起，最长 10 分钟，并遵守 `Retry-After`），界面上会显示倒计时，不会把这些玩法记为失败。在固件全部下载完之前，点"刷入"会先单独下载那一个固件（排到队列最前面）。
> 要让首次全量下载顺利完成，建议后端为桌面端放宽限流（请求头带 `User-Agent: AIPassportPlays/<版本>`），或者提供 CDN / 离线数据包。

## 玩法存储目录

默认路径是 `~/Documents/AI Passport Plays`，可以在设置里更换。

```
index.json                 分类、专题、全部玩法（界面读取这个文件）
plays/<slug>/meta.json     社区原始数据和本地文件状态
plays/<slug>/cover.webp    thumb.webp  gallery/N.webp
plays/<slug>/firmware.bin  已校验的固件
avatars/<name>.webp        作者头像
```

设置本身保存在 `~/Library/Application Support/FoloToy/AI Passport Plays/settings.json`，只记录目录和语言。

## 架构

沿用 [Magic-Flash](https://github.com/FoloToy/Magic-Flash) 的结构：

- **Go + Wails v3** 桌面外壳。界面是打包进程序的原生 HTML/CSS/JS（`frontend/`），作为 Wails 的资源处理器提供，**不监听任何网络端口**。
- **Python esptool 刷机程序**（`worker/main.py`），用 PyInstaller 打包进 App。它是独立进程，和 Go 之间用 JSON lines 通信，用户不需要安装 Python。
- `internal/syncer`：社区接口、两阶段下载、限流退避、目录迁移。
- `internal/firmware`：ESP32-C3 镜像头、大小、SHA-256 校验。
- `internal/flasher`：单台刷机任务和进度。
- `internal/desktop`：本地 JSON API（带会话 token）、媒体文件、内嵌界面。

## 开发

需要 Go 1.26、Python 3.12+ 和 Xcode 命令行工具。

```sh
export GOTOOLCHAIN=go1.26.0
go test -race ./...

python3.12 -m venv .runtime/worker-venv
.runtime/worker-venv/bin/pip install -r worker/requirements.lock
.runtime/worker-venv/bin/python -m unittest worker/test_worker.py

# 在浏览器里调试界面（只监听 127.0.0.1）
go run . -headless -listen 127.0.0.1:47811 \
  -worker .runtime/worker-venv/bin/python -worker-script worker/main.py \
  -state-dir /tmp/plays-state

# 直接运行桌面窗口
go run . -worker .runtime/worker-venv/bin/python -worker-script worker/main.py
```

## 打包（macOS）

```sh
scripts/package-mac.sh          # 生成 dist/AI Passport 玩法社区.app 和 DMG
ALLOW_DIRTY=1 scripts/package-mac.sh   # 有未提交改动时也打包
```

打出的包与当前 Mac 的架构一致（arm64 或 x86_64）。GitHub Actions 会同时构建两种架构。

目前**没有 Developer ID 签名和公证**，只做了 ad-hoc 签名。别人第一次打开时 macOS 会拦截，需要在"系统设置 → 隐私与安全性"里点"仍要打开"，或者右键 App 选择"打开"。

Windows 版的代码已经可以交叉编译（串口、单实例锁、磁盘空间都有 Windows 实现），安装包会在下一阶段补上。

## 许可证

见 [THIRD_PARTY.md](THIRD_PARTY.md)。刷机程序 `worker/main.py` 使用 GPL-2.0-or-later（与 esptool 相同）。
