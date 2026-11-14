# AI Passport 玩法社区（离线桌面版）

[AI Passport 玩法社区](https://ai-passport.folotoy.cn/plays/) 的本地桌面 App：把全部玩法、图片和固件下载到电脑上，之后不联网也能浏览玩法，并通过 USB 把玩法刷入 AI Passport（ESP32-C3）。

The offline desktop edition of the AI Passport App Library. It downloads every play, image and firmware to your computer, then lets you browse and install plays to an AI Passport over USB without a network.

支持 macOS 和 Windows 10/11。Windows 7 / 8 / 8.1 请用单独的原生客户端 [ai-passport-desktop-windows7](https://github.com/FoloToy/ai-passport-desktop-windows7)。

## 功能

- **主页**：官方精选、专题页签（多人玩法 / 亲子益智 / 日常办公）、搜索、排序（最新 / 下载 / 点赞 / 讨论），布局与网站一致。
- **分类**：左侧分类（游戏与互动、效率与工具……）和专题（必玩精选等），带数量。
- **详情**：图集、玩法介绍、使用说明、更新日志、作者。
- **刷机**：选择串口 → 校验固件 → 检测芯片必须是 ESP32-C3、Flash 至少 8 MB、未开启安全启动 → 从 0x0 写入 → 回读校验 → 重启。一次刷一台。
- **备份固件**：把设备上现有的整片 Flash（8 MB）读出来保存到电脑，只读不写，并和设备上计算的 MD5 核对。
- **恢复出厂**：一键把 AI Passport 恢复成出厂固件，不用选串口（详见下文）。取代原来的 FoloToy-AI-Passport-Factory-Restore 工具。
- **设置**：选择 / 更换玩法存储目录（可迁移已有数据）、检查更新、语言（跟随系统 / 中文 / English）。
- **首次启动**：安装包里不带任何玩法。首次打开时引导用户选择目录并下载。

## 下载更新是怎么工作的

更新分两步，第一步完成后就可以浏览：

1. **玩法信息和图片**：分页拉取全部玩法（约 1,260 个），下载封面、缩略图、图集和作者头像（约 330 MB，约 2 分钟），写入 `index.json`。
2. **固件**：按下载量从高到低排队下载（约 3 GB），每个文件都校验大小、SHA-256、ESP32-C3 镜像头，通过后才原子替换到位。

之后再更新时只下载新增或有变化的玩法（比较 `revisionId` 和固件 SHA-256）。网站上已下架的玩法会保留在本地，并标记为"已下架"。

> **服务器限流**：社区的固件下载接口（`/api/download/...`）按 IP 限流。实测连续下载约 30 个后返回 HTTP 429，封禁会持续 25 分钟以上。App 遇到 429 会让所有固件下载一起暂停并逐步退避（30 秒起，最长 10 分钟，并遵守 `Retry-After`），界面上会显示倒计时，不会把这些玩法记为失败。在固件全部下载完之前，点"刷入"会先单独下载那一个固件（排到队列最前面）。
> 要让首次全量下载顺利完成，建议后端为桌面端放宽限流（请求头带 `User-Agent: AIPassportPlays/<版本>`），或者提供 CDN / 离线数据包。

## 备份固件

在"设备工具"页点"开始备份"。和恢复出厂一样会自动找设备，然后读取整片 Flash，保存为 `<备份目录>/<时间>-<MAC>-firmware-backup/AI-Passport-<MAC>-<时间>-full-flash.bin`，旁边有 `.sha256`、日志和报告。读完后让设备自己计算 MD5 并比对，一致才算成功。不会向设备写入任何内容。

## 恢复出厂

用于刷入玩法后设备无法启动、反复重启、连上电脑后很快断开等情况。出厂固件 `AI-Passport-Factory-Default-v1.1.2` 内置在 App 里（`pkg/factory/firmware/`，约 3.7 MB），不需要联网。

流程（与原恢复工具一致）：

1. 校验内置的四个固件分段（大小、SHA-256、不与身份区重叠）。
2. 自动扫描 Espressif USB 设备（VID 303A），最多等 150 秒；只连接一台时才继续。设备 USB 只出现几百毫秒也能抓住并让它进入下载模式。Windows 上还会处理 USB Code 43。
3. 确认芯片是 ESP32-C3、Flash 正好 8 MB、未开启 Secure Boot 和 Flash 加密；无法确认就停止。
4. 备份分区表和身份区 `cardid`，默认再备份整片 8 MB Flash。
5. 分段写入（不执行 erase_flash，不写身份区），逐段回读校验，再确认 `cardid` 前后逐字节一致，最后重启。

| 偏移 | 文件 | 字节数 |
| --- | --- | ---: |
| `0x000000` | `bootloader.bin` | 22,496 |
| `0x008000` | `partition-table.bin` | 3,072 |
| `0x010000` | `trae_card.bin` | 2,665,312 |
| `0x3FA000` | `imgava.bin` | 1,048,576 |

**身份区 `cardid`**（`0x356000–0x359FFF`，16 KB）保存每台设备独有的序列号和密钥，恢复出厂永远不会写这里。开始写入后点"停止"不会中断，会等写入和校验完成，避免设备处于半写入状态。

备份默认保存在 `~/Documents/FoloToy恢复出厂备份/`（与原工具相同，可在恢复出厂页更改），每次恢复一个子目录 `<时间>-<MAC>/`，里面有日志和恢复报告。

更新出厂固件时：替换 `pkg/factory/firmware/` 里的文件，同时更新 `pkg/factory/factory.go` 里的大小、SHA-256 和版本号，再用真机验证。

> **刷入玩法和身份区**：社区玩法是从 `0x0` 写入的合并镜像。超过 `0x356000`（约 3.3 MB）的镜像会覆盖身份区。刷入这类玩法前，App 会先把设备当前的身份区备份到 `<备份目录>/cardid/<MAC>/`，覆盖了有内容的身份区时会在刷机结果里提示。

## 玩法存储目录

默认路径是 `~/Documents/AI Passport Plays`，可以在设置里更换。

```
index.json                 分类、专题、全部玩法（界面读取这个文件）
plays/<slug>/meta.json     社区原始数据和本地文件状态
plays/<slug>/cover.webp    thumb.webp  gallery/N.webp
plays/<slug>/firmware.bin  已校验的固件
avatars/<name>.webp        作者头像
```

设置本身保存在 `~/Library/Application Support/FoloToy/AI Passport Plays/settings.json`，只记录目录、备份目录和语言。恢复出厂时，内置固件会先解压到同一目录下的 `factory-firmware/` 并再次校验。

## 架构

沿用 [Magic-Flash](https://github.com/FoloToy/Magic-Flash) 的结构：

- **Go + Wails v3** 桌面外壳。界面是打包进程序的原生 HTML/CSS/JS（`frontend/`），作为 Wails 的资源处理器提供，**不监听任何网络端口**。
- `pkg/esp`：用 Go 实现的 ESP32-C3 串口刷机协议（ROM 引导程序握手、上传 flasher stub、压缩写入、读取、MD5 校验、复位），参照 esptool 的协议。内置的 flasher stub 来自 esp-flasher-stub（Apache-2.0 / MIT）。不需要 Python 或 esptool。
- `pkg/device`：在 App 进程里运行的设备任务：刷入玩法、恢复出厂、固件备份、自动抓取设备（含 Windows Code 43 处理）。
- `pkg/syncer`：社区接口、两阶段下载、限流退避、目录迁移。
- `pkg/firmware`：ESP32-C3 镜像头、大小、SHA-256 校验。
- `pkg/flasher`：单台设备任务（刷入玩法或恢复出厂）和进度，同一时间只运行一个。
- `pkg/factory`：内置出厂固件、分段表和身份区保护检查。
- `pkg/core`：和界面无关的 App 逻辑（设置、玩法库索引、开始刷机/恢复/备份前的检查）。
- `internal/desktop`：本地 JSON API（带会话 token）、媒体文件、内嵌界面，调用 `pkg/core`。

`pkg/` 下的包是公开的：Windows 7/8/8.1 的原生客户端 [ai-passport-desktop-windows7](https://github.com/FoloToy/ai-passport-desktop-windows7) 直接引用它们，所以两边的刷机和安全检查是同一份代码。

## 开发

需要 Go 1.26 和 Xcode 命令行工具。用 AI Agent 开发时先读 [AGENTS.md](AGENTS.md)（必须遵守的设备安全规则、约定、测试方法）；发版步骤见 [docs/RELEASE.md](docs/RELEASE.md)。

```sh
export GOTOOLCHAIN=go1.26.0
go test -race ./...

# 在浏览器里调试界面（只监听 127.0.0.1）
go run . -headless -listen 127.0.0.1:47811 -state-dir /tmp/plays-state

# 直接运行桌面窗口
go run .

# 检查内置 flasher stub 和出厂固件
go run . -self-test
```

### 用 QEMU 测试刷机协议

不接真机时，可以用 Espressif 的 QEMU（`esp-develop-9.2.2`，riscv32）模拟一台 8 MB 的 ESP32-C3，停在下载模式：

```sh
qemu-system-riscv32 -nographic -machine esp32c3 \
  -drive file=flash.bin,if=mtd,format=raw \
  -global driver=esp32c3.gpio,property=strap_mode,value=0x02 \
  -serial tcp:127.0.0.1:5555,server,nowait

ESP_QEMU_ADDR=127.0.0.1:5555 go test -count=1 -run QEMU ./pkg/esp/ ./pkg/device/
```

`flash.bin` 必须正好 2/4/8/16 MB。`pkg/device` 的 QEMU 测试会做一次完整的恢复出厂（含 8 MB 全量备份）和一次固件备份。

## 打包（macOS）

```sh
scripts/package-mac.sh          # 生成 arm64、x86_64、universal 三个 DMG
ALLOW_DIRTY=1 scripts/package-mac.sh   # 有未提交改动时也打包
```

一次生成三个 DMG（App 分别放在 `dist/arm64/`、`dist/x86_64/`、`dist/universal/`）：

| 文件 | 适用 |
| --- | --- |
| `AI-Passport-Plays-<版本>-macOS-arm64.dmg` | Apple 芯片（M 系列） |
| `AI-Passport-Plays-<版本>-macOS-x86_64.dmg` | Intel 芯片 |
| `AI-Passport-Plays-<版本>-macOS-universal.dmg` | 两种都能用，体积大一倍 |

在 Apple 芯片的 Mac 上就能交叉编译出 Intel 版；装了 Rosetta 时，脚本会用 Rosetta 跑一遍 Intel 版的自检。

目前**没有 Developer ID 签名和公证**，只做了 ad-hoc 签名。别人第一次打开时 macOS 会拦截，需要在"系统设置 → 隐私与安全性"里点"仍要打开"，或者右键 App 选择"打开"。

## 打包（Windows）

在 Windows 10/11 x64 上运行（这个版本只支持 Windows 10 及以上）：

```powershell
powershell -ExecutionPolicy Bypass -File scripts\build-on-windows.ps1          # 安装依赖并打包
powershell -ExecutionPolicy Bypass -File scripts\build-on-windows.ps1 -Upload  # 打包并上传到 Release
```

会生成 `dist\windows\` 下的 Setup.exe 安装包和免安装 ZIP。完整步骤和排错见 [docs/WINDOWS-BUILD.md](docs/WINDOWS-BUILD.md)。

## 许可证

本项目的源代码以 [MIT 许可证](LICENSE) 发布。第三方组件、内置的 FoloToy 出厂固件（`pkg/factory/firmware/`）不在 MIT 范围内，各自的来源和许可见 [THIRD_PARTY.md](THIRD_PARTY.md)。
