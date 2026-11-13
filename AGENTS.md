# AGENTS.md

写给在这个仓库里工作的 AI Agent（Claude Code、Codex、Cursor 等）和新加入的开发者。先读这一页，再看 `README.md`（面向用户和开发者的说明）和 `docs/`。

## 项目是什么

**AI Passport 玩法社区（离线桌面版）**：把 https://ai-passport.folotoy.cn/plays/ 的玩法全部下载到本地，离线浏览，并把玩法固件刷进 AI Passport（ESP32-C3，8 MB Flash）。另外有"设备工具"：备份固件（只读）和恢复出厂固件。

- 仓库：`FoloToy/ai-passport-plays`（公开，MIT 许可证，见 `LICENSE`），默认分支 `main`，PR 也提到 `main`。
- 平台：macOS（arm64 / x86_64 / universal）、Windows 10/11 x64、Windows 7/8/8.1（x64 / x86，单独的版本）。
- 界面中英双语（`frontend/app.js` 里的 `I18N.zh` / `I18N.en`）。
- 一次只操作一台设备，只支持 ESP32-C3。

## 技术栈

| 部分 | 实现 |
| --- | --- |
| 外壳 | Go 1.26.0（`GOTOOLCHAIN=go1.26.0`）+ Wails v3.0.0-beta.24（`ui_desktop.go`，构建标签 `!win7`） |
| Windows 7 外壳 | 同一份代码，`-tags win7`（`ui_win7.go`），用 go-legacy-win7 编译，界面在 Chrome 109 里打开 |
| 界面 | 原生 HTML/CSS/JS，没有构建步骤，`embed` 进二进制（`frontend/`） |
| 刷机 | 纯 Go：`internal/esp`（ESP 串口协议 + esp-flasher-stub）、`internal/device`（任务），在 App 进程内运行。**没有 Python、没有 esptool** |
| 串口 | `go.bug.st/serial` v1.8.0 |

## 目录地图

```
main.go               启动、命令行参数（-headless -listen -state-dir -origin -self-test）、127.0.0.1 调试服务
ui_desktop.go         Wails 窗口（macOS / Win10+）
ui_win7.go            Windows 7 版：Chrome --app 窗口、PowerShell 选文件夹、驱动安装
frontend/             index.html app.css app.js（单文件 SPA，含全部中英文案）
internal/
  esp/                ESP32-C3 串口协议：SLIP、命令、同步、复位、stub、压缩写、读、MD5、看门狗复位
    stub/             esp32c3.json（esp-flasher-stub v1.4.1，Apache-2.0/MIT）+ 许可证
  device/             设备任务：刷入玩法、恢复出厂、备份固件、自动抓设备、Code 43、重启确认
  driver/             Windows 7/8 串口驱动检测与安装（内置 usbser INF）
  flasher/            任务管理器：同一时间一个任务、状态快照、取消规则
  factory/            内置出厂固件（4 段）、分段表、身份区保护
  firmware/           玩法镜像校验（0xE9、chip_id 0x0005、大小、SHA-256）
  syncer/             社区接口、两阶段下载、限流退避
  library/            本地玩法库目录结构
  desktop/            本地 JSON API（/api/*，X-Plays-Token 鉴权）、媒体文件、内嵌界面
  settings/ instance/ 设置文件、单实例锁
scripts/              打包脚本（见 docs/RELEASE.md）
docs/                 WINDOWS-BUILD.md（Win10+ 打包）、WINDOWS-7.md（Win7 版和驱动）、RELEASE.md（发版）
drivers/windows7/     原始的 JTAG 驱动 zip（只覆盖 MI_02 接口）
PLAN.md               最初的开发方案、已确认的决定、待办
```

## 常用命令

```sh
export GOTOOLCHAIN=go1.26.0
go vet ./... && go test -race ./...            # 改完代码必跑
GOOS=windows go vet ./...                      # Windows 专用代码（driver、hints、diskfree）
go run . -headless -listen 127.0.0.1:47811 -state-dir /tmp/plays-state   # 浏览器里调界面
go run . -self-test                            # 检查内置 stub 和出厂固件
node -e "new Function(require('fs').readFileSync('frontend/app.js','utf8'))"  # 改 app.js 后做语法检查
```

Windows 7 构建检查：`scripts/package-win7.sh` 会下载固定版本的 go-legacy-win7 并 `vet` + 编译 x64 和 x86。

macOS 链接时出现大量 `ld: warning: ... built for newer 'macOS' version` 是正常的，可以忽略。

### 不接真机的测试：QEMU

Espressif QEMU（`espressif/qemu` 的 `esp-develop-9.2.2`，riscv32）可以模拟停在下载模式的 8 MB ESP32-C3，命令见 README"用 QEMU 测试刷机协议"。然后：

```sh
ESP_QEMU_ADDR=127.0.0.1:5555 go test -count=1 -run QEMU ./internal/esp/ ./internal/device/
```

`internal/device` 的 QEMU 测试做一次完整恢复出厂（含 8 MB 备份，约 1.5 分钟）和一次备份。QEMU 测不到 DTR/RTS 复位和真实波特率切换。

### 真机测试（只在用户同意后）

`internal/device/real_test.go` 默认跳过：

```sh
ESP_REAL_PORT=/dev/cu.usbmodem1101 go test -count=1 -v -run TestRealDevice ./internal/device/   # 只读：连接、stub、复位
ESP_REAL_BACKUP_DIR=<目录> go test -count=1 -v -run TestRealDeviceBackup ./internal/device/   # 只读：完整备份
```

## 必须遵守的规则

### 设备安全（最重要）

- **恢复出厂永远不写身份区 `cardid`（`0x356000–0x359FFF`）**，不做整片擦除（erase_flash），四段分别写入。`factory.CheckLayout` 和测试会检查，不要绕过。
- 恢复出厂顺序固定：识别芯片 → 检查安全启动/Flash 加密 → 只接受正好 8 MB → 备份分区表和 cardid（默认还有整片）→ 写入 → 逐段 MD5 校验 → cardid 前后逐字节一致 → 复位。测试 `TestFullRestorePreservesCardID` 断言了阶段顺序。
- **写入开始后不能取消**（`flasher.Cancel` 和 `restoreJob.checkCancel`）；同一时间只有一个设备任务；检测到多台 Espressif 设备时不写。
- 非 ESP32-C3、开启了 Secure Boot 或 Flash 加密、或无法确认安全设置时，一律拒绝写入（fail closed）。
- 刷入玩法前，如果镜像会覆盖非空的 cardid，先把它备份到 `<备份目录>/cardid/<MAC>/`。
- **不要在没有用户明确同意的情况下往真机写东西**（刷入玩法、恢复出厂）。只读操作（连接、读 Flash、备份）也会让设备重启，做之前先说明。
- 备份文件里有设备的序列号和密钥：不要提交、不要上传、不要贴到日志或对话里，测试完就删。`.gitignore` 已经忽略 `*.bin`（出厂固件除外）。

### 界面和后端的约定

- 后端事件类型：`stage`、`identified`、`progress`、`log`、`hint`、`cardid`、`complete`；错误用 `device.CodedError{Code}`。前端用 `err_<code>` 翻译错误（`errText`），新增错误码时**同时加中英文案**。
- 阶段名（`preparing waiting connecting checking backup writing verifying cardid resetting done failed cancelled`）被前端直接使用，改名要同步改 `frontend/app.js`。
- 所有用户可见的文字都要中英两份。
- 界面必须能在 **Chrome 109** 上跑（Windows 7 版）：不要用 Chrome 110 以后才有的 JS/CSS（例如 `Array.prototype.toSorted`、CSS 嵌套、`@scope`、`text-wrap: balance`）。
- API 只监听 `127.0.0.1`（headless / Win7 版），请求要带 `X-Plays-Token`。

### 文件格式

- `*.ps1`、`*.iss` 是 **UTF-8 带 BOM + CRLF**，`*.inf` 是 CRLF（见 `.gitattributes`）。用脚本按字节修改，保留 BOM 和换行，不要用会改写换行的编辑方式。
- Go 代码提交前 `gofmt`。

### 社区接口

- 固件下载接口 `/api/download/...` 按 IP 限流（大约 20–30 次后返回 429，封 30 分钟以上）。测试时不要反复全量下载；列表和图片接口不限流。

### 许可证

- `internal/esp` 是自己写的 Go 代码，但协议细节参照 esptool（GPL-2.0-or-later）；不要从 esptool 复制源代码进来。stub 只用 `stub/esp32c3.json`（v2，Apache/MIT），**不要用 esptool 的 v1 stub（GPL）**。改动依赖时更新 `THIRD_PARTY.md`。

### Git

- 提交信息用 [Conventional Commits](https://www.conventionalcommits.org/)（`feat:`、`fix:`、`docs:`、`chore:`…）。
- 只有用户要求时才提交、推送或发版。
- 不要提交 `dist/`、`.runtime/`、备份文件、本机路径或个人信息。仓库是公开的，提交的一切都会被所有人看到。

## 发版

步骤、要改版本号的位置、各平台怎么打包、发布说明怎么写，见 [docs/RELEASE.md](docs/RELEASE.md)。Release 页面只保留最新的一个版本。

## 还没验证的部分（改相关代码时要特别小心）

- 刷入玩法和恢复出厂的**写入流程还没有在真机上跑过**（只在 QEMU 上验证）；读、备份、两种自动重启已在真机验证。
- Intel Mac 版只在 Rosetta 下自检过。
- Windows 7 版没有在 Windows 7 真机上验收；驱动 INF 没有签名目录（.cat）。
- 待办以 `PLAN.md` 第 13 节为准，完成一项就更新那里。
