# Windows 打包说明（交接文档）

本文档写给在 Windows 机器上执行打包的人或 AI。目标：在 Windows 10/11 x64 上打出安装包和免安装 ZIP，上传到 GitHub Release `v0.4.1`。

## 当前状态

- macOS 版已经完成，`v0.4.1` Release 里有 DMG（v0.2.0 起新增恢复出厂，v0.3.0 新增备份固件，v0.4.0 刷机改用 Go、不再需要 Python，取代原 FoloToy-AI-Passport-Factory-Restore）：https://github.com/FoloToy/ai-passport-plays/releases/tag/v0.3.0
- Windows 版的代码已经在 Mac 上验证过：交叉编译通过，`GOOS=windows go vet` 通过，全部测试代码能在 Windows 下编译。
- **Windows 打包脚本还没有在真实 Windows 上跑过**，第一次运行可能需要修小问题，见文末"可能遇到的问题"。

## 产物

打包完成后，`dist\windows\` 下会生成：

| 文件 | 说明 |
| --- | --- |
| `AI-Passport-Plays-0.4.1-Windows-x64-Setup.exe` | 安装版（Inno Setup，按用户安装，不需要管理员权限） |
| `AI-Passport-Plays-0.4.1-Windows-x64.zip` | 免安装版 |
| `*.sha256` | 校验和 |
| `AI Passport Plays\` | 打包用的临时目录（不需要上传） |

安装后的结构：`AIPassportPlays.exe`（刷机功能已经内置，用 Go 实现，不再有单独的刷机程序），旁边是 `runtime\MicrosoftEdgeWebview2Setup.exe`、`ThirdParty\`。

## 步骤

在 Windows 上打开 **PowerShell**（不需要管理员权限，winget 安装软件时可能会弹出 UAC 窗口）。

```powershell
# 1. 安装 GitHub CLI 并登录（用来克隆代码和上传 Release）
winget install -e --id GitHub.cli --accept-package-agreements --accept-source-agreements
# 重新打开一个 PowerShell 窗口，让 PATH 生效
gh auth login            # 选 GitHub.com → HTTPS → 浏览器登录；上传 Release 需要仓库的写权限

# 2. 克隆代码
gh repo clone FoloToy/ai-passport-plays
cd ai-passport-plays

# 3. 一键安装依赖、打包、上传到 Release v0.4.1
powershell -ExecutionPolicy Bypass -File scripts\build-on-windows.ps1 -Upload
```

`scripts\build-on-windows.ps1` 会做这些事：

1. 用 winget 安装缺少的工具：Git、Go、Inno Setup 6（如果带了 `-Upload`，还会装 GitHub CLI）。不需要 Python。
2. 设置国内可用的镜像：`GOPROXY=https://goproxy.cn,...`。
3. 运行 `scripts\package-windows.ps1`，依次执行：
   - `go test ./...`，然后 `go build -H windowsgui`。Go 版本由 `GOTOOLCHAIN=go1.26.0` 自动下载。
   - 下载微软官方的 WebView2 安装程序，并校验它的签名。
   - 自检打包好的程序：`AIPassportPlays.exe -self-test`（检查内置的 ESP32-C3 flasher stub 和出厂固件）。
   - 生成 ZIP，用 Inno Setup 生成 Setup.exe，写入 sha256。
4. 带 `-Upload` 时，执行 `gh release upload v0.4.1 ... --clobber`。

只打包、不上传：去掉 `-Upload`。已经装好工具时，可以直接运行：

```powershell
powershell -ExecutionPolicy Bypass -File scripts\package-windows.ps1
# 有未提交的改动也要打包时加 -AllowDirty
```

## 打包完成后的验收

1. 运行 Setup.exe 安装，打开"AI Passport 玩法社区"。窗口应该显示深色界面和欢迎页。
2. 点"更改"能打开系统的文件夹选择框；选一个空文件夹后点"开始更新"，应该能开始下载。
   - 如果报"目录无效 / Invalid folder"，说明 POST 请求体没有传到 Go，看 `internal/desktop/server.go` 里读取请求体的那段。macOS 上就出过这个问题（WebKit 不带 Content-Length），已经修复。
3. 玩法信息和图片下载完后能浏览；固件下载如果遇到限流，会显示"等待服务器"倒计时，属于正常现象。
4. 有 AI Passport 的话，插上 USB，点"刷入"，串口列表里应该出现标着 "AI Passport" 的设备（USB VID 303A）。ESP32-C3 内置的 USB-Serial-JTAG 在 Windows 10/11 上不需要装驱动。
5. 顶部点"设备工具"，先试"开始备份"：不插设备时显示"正在等待设备"，点"停止"后停下；插上设备应生成 `*-firmware-backup` 文件夹和 8 MB 的 `.bin`。
6. 同一页里点"恢复出厂"→"一键恢复出厂"→"开始恢复"。不插设备时应显示"正在等待设备"，点"停止"后显示"已停止"。插上设备应能自动完成，并在 `文档\FoloToy恢复出厂备份\` 下生成备份和报告。自检输出里应该有 `"factory": "AI-Passport-Factory-Default-v1.1.2"`。

## 可能遇到的问题

| 现象 | 处理 |
| --- | --- |
| `winget` 不存在 | 在 Microsoft Store 安装"应用安装程序"（App Installer） |
| 刚装完工具，提示命令找不到 | 关掉 PowerShell 重新打开，再运行一次脚本 |
| `go: downloading go1.26.0` 很慢或失败 | 确认 `GOPROXY` 是 goproxy.cn；也可以直接装 Go 1.26 并设置 `GOTOOLCHAIN=local` |
| `Commit the source tree before packaging` | 工作区有改动。提交后再打包，或者加 `-AllowDirty` |
| 某个 Go 测试只在 Windows 上失败 | 多半是路径或文件占用的问题，修好后提交；不要跳过测试直接发布 |
| 打出的程序被杀毒软件拦截 | 把仓库目录加入 Windows 安全中心的排除项 |
| 安装包没有签名 | 预期内。运行时 SmartScreen 会提示，点"更多信息 → 仍要运行" |

## 相关文件

- `scripts/build-on-windows.ps1`：一键脚本（装依赖 + 打包 + 上传）
- `scripts/package-windows.ps1`：打包脚本
- `scripts/windows-installer.iss`：Inno Setup 配置
- `rsrc_windows_amd64.syso` / `rsrc_windows_arm64.syso`：exe 的图标、版本信息和清单（DPI 感知等），由 `go-winres` 根据 `assets/icon.png` 生成。重新生成的命令：
  ```sh
  go run github.com/tc-hib/go-winres@v0.3.3 simply --icon assets/icon.png --manifest gui \
    --product-name "AI Passport 玩法社区" --file-description "AI Passport 玩法社区" --copyright FoloToy \
    --product-version 0.4.1.0 --file-version 0.4.1.0 --original-filename AIPassportPlays.exe --arch amd64,arm64,386
  ```
- 改了版本号时：同时改 `VERSION` 和上面 go-winres 命令里的版本号
