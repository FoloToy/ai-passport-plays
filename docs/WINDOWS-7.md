# Windows 7 / 8 / 8.1 使用说明

> 状态：**有单独的 Windows 7 / 8 / 8.1 版**（v0.4.0 起），文件名带 `Windows7`，分 x64 和 x86（32 位）。功能和 Windows 10 版一样，界面用 Chrome 打开。
> 还没有在 Windows 7 真机上验收过；第一次使用时请记录结果并更新本文档。

## 1. Windows 7 版和 Windows 10 版的区别

| 组件 | Windows 10/11、macOS 版 | Windows 7/8/8.1 版 |
| --- | --- | --- |
| 编译器 | Go 1.26 | [go-legacy-win7](https://github.com/thongtech/go-legacy-win7) 1.26.11-1（打了 Windows 7 补丁的 Go 1.26，`scripts/package-win7.sh` 固定版本并校验 SHA-256） |
| 界面 | Wails v3 + WebView2 窗口 | 同一套界面，在 Chrome 109 / Edge 109 的应用窗口里打开（`--app` 模式），只监听 `127.0.0.1` 的随机端口 |
| 程序窗口 | 普通窗口 | 多一个黑色控制台窗口；关掉它就退出。刷机时不要关 |
| 选择文件夹 | 系统对话框 | PowerShell 2.0 弹出的系统文件夹对话框 |
| 串口驱动 | 系统自带 | 程序检测到没有驱动时，界面提示"安装驱动"（见 3.2） |
| 安装方式 | Setup.exe 或 ZIP | 只有 ZIP，解压即用 |

源代码是同一份，Windows 7 版用 `-tags win7` 编译（`ui_win7.go` 代替 `ui_desktop.go`）。在 Mac 上运行 `scripts/package-win7.sh` 就能打包，不需要 Windows。

## 2. Windows 7 用户怎么用

1. 安装 **Google Chrome 109**（或 Microsoft Edge 109）。这是最后支持 Windows 7/8/8.1 的版本。Chrome 109 不再获得安全更新，建议只用于本程序。
2. 下载 `AI-Passport-Plays-<版本>-Windows7-x64.zip`（64 位）或 `-x86.zip`（32 位），完整解压后双击 `AIPassportPlays.exe`。
3. 插上 AI Passport。界面提示没有驱动时，按 3.2 安装。
4. 之后的用法和 Windows 10 版一样：更新玩法库、刷入玩法、备份固件、恢复出厂。

### 不想装程序：网页版

1. 同样需要 Chrome 109。网页版刷机使用 Web Serial API，Chrome 89 起支持。
2. 按下面第 3 节安装驱动。
3. 用 Chrome 109 打开 https://ai-passport.folotoy.cn/plays/ ，选择玩法后点"刷入"，在弹出的串口列表里选择 AI Passport。

> 待确认：网页版刷机页面在 Chrome 109 上是否完全正常（页面脚本可能用了新版浏览器才有的功能）。

## 3. 驱动

AI Passport（ESP32-C3）的 USB 口对电脑来说是一个设备、两个接口：

| 接口 | 硬件 ID | 用途 | Windows 10/11 | Windows 7/8 |
| --- | --- | --- | --- | --- |
| 串口（CDC） | `USB\VID_303A&PID_1001&MI_00` | **刷机必须用这个** | 自动装好系统自带的 `usbser.sys` | **需要手动装驱动** |
| JTAG 调试 | `USB\VID_303A&PID_1001&MI_02` | 只用于芯片调试（OpenOCD），刷机不需要 | 自动 | 需要手动装驱动 |

Windows 7/8 不会自动给 CDC 设备匹配 `usbser.sys`，所以设备管理器里会出现两个带黄色感叹号的"USB JTAG/serial debug unit"，也不会出现 COM 口。

### 3.1 仓库里的驱动包（只覆盖 JTAG 接口）

`drivers/windows7/AI-Passport-Win7-JTAG-driver-Espressif-2021-07-15.zip`，就是原来发给用户的《AI Passport 串口驱动（适合WIN10以下系统）.zip》。

- SHA-256：`84e741dbec5526e3152bded421b4f06f990cd2d1d7e83b907c40e81f9db0f30e`
- 内容：`USB_JTAG_debug_unit.inf` + `.cat`（Espressif 的 DigiCert 签名），由 libwdi 生成，给 **`MI_02`（JTAG）接口**安装 WinUSB，附带 x86/amd64 的 WinUSB、libusb0、libusbK 文件。
- **它不包含串口（`MI_00`）驱动**。只装这个包，设备仍然不会出现 COM 口，网页版也选不到串口。名字里的"串口驱动"不准确。

### 3.2 安装串口驱动（必须）

**用 Windows 7 版程序（推荐）**：插上设备后，"设备工具"页和刷机窗口会出现"安装驱动"按钮。点它 → UAC 窗口点"是" → 如果提示"Windows 无法验证此驱动程序软件的发布者"，选"始终安装此驱动程序软件" → 装好后重新插拔设备。

程序用的是 `drivers/ai-passport-usb-serial.inf`（源文件在 `internal/driver/`）：只给 `USB\VID_303A&PID_1001&MI_00`（串口接口）绑定 Windows 自带的 `usbser.sys`，用 `pnputil -i -a` 安装，不碰 JTAG 接口。INF 没有签名目录（.cat），所以 Windows 会提示一次发布者无法验证；`usbser.sys` 本身是微软签名的，64 位系统也能装。

**手动安装（程序装不上时）**：
推荐用 Zadig（libwdi 官方工具，支持 Windows 7 及以上），给 `MI_00` 接口装 Windows 自带的 `usbser.sys`：

1. 下载 Zadig：https://zadig.akeo.ie/
2. 用 USB 数据线插上 AI Passport（只插这一台）。
3. 右键 Zadig →"以管理员身份运行"。
4. 菜单 **Options → List All Devices** 打勾。
5. 下拉框里选 **USB JTAG/serial debug unit (Interface 0)**。确认下面的 USB ID 是 `303A 1001 00`。**不要选 Interface 2，也不要选其他设备**（选错会让鼠标键盘等设备失灵）。
6. 右侧目标驱动用上下箭头切到 **USB Serial (CDC)**。
7. 点 **Install Driver** / **Replace Driver**，等到提示成功。
8. 打开设备管理器，"端口 (COM 和 LPT)"下应该出现一个 COM 口。

> 如果设备没有出现在 Zadig 列表里：刷了某些玩法后 USB 会反复断开。按住设备的 BOOT 键再插 USB，让它停在下载模式后再操作。

### 3.3 安装 JTAG 驱动（可选）

只做芯片调试时才需要，刷机用不到：解压 3.1 的 zip，在设备管理器里右键带感叹号的"USB JTAG/serial debug unit"（硬件 ID 末尾是 `MI_02` 的那个）→ 更新驱动程序 → 浏览计算机 → 选择解压后的文件夹。

### 3.4 验证

1. 设备管理器"端口"里有 COM 口，没有黄色感叹号。
2. Chrome 109 打开网页版，点"刷入"后弹出的列表里能看到这个设备。
3. 刷入一个玩法并启动成功。

### 3.5 常见问题

| 现象 | 处理 |
| --- | --- |
| 装了 zip 里的驱动仍然没有 COM 口 | 正常，zip 只覆盖 JTAG 接口，按 3.2 装串口驱动 |
| Windows 提示"无法验证此驱动程序软件的发布者" | 选"始终安装此驱动程序软件" |
| 64 位 Windows 7 提示驱动签名问题 | 先装 Windows 7 SP1 和 KB3033929（SHA-2 代码签名支持）、KB4474419 |
| 换 USB 口后又出现感叹号 | Windows 7 按端口记驱动，换口后再装一次 |
| 鼠标键盘失灵 | Zadig 里选错了设备。设备管理器里找到被替换的设备 → 卸载并删除驱动 → 重新插拔 |

## 4. 待验收

- 在 Windows 7 SP1 x64 和 x86 真机（或 VirtualBox + USB 直通）上：程序能启动、Chrome 109 打开界面、选择文件夹、更新玩法库。
- 驱动：插上设备后提示"安装驱动"，安装成功出现 COM 口；刷入玩法、备份固件、恢复出厂都成功。
- 没有装 Chrome 时的提示；第二次双击程序时直接打开已经在运行的界面。

## 参考

- Zadig：https://zadig.akeo.ie/
- Windows 7/8 上 ESP32-C3 内置 USB 不可用的讨论：https://esp32.com/viewtopic.php?p=135064
- 社区提供的 Windows 7 CDC INF（针对 ESP32-S3，C3 结果不一）：https://github.com/kutukvpavel/Esp32-Win7-VCP-drivers
