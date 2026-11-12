# Windows 7 / 8 / 8.1 使用说明

> 状态：**桌面 App 暂不支持 Windows 7 / 8 / 8.1**，以后单独做 Windows 7 版本。目前这类电脑请用浏览器在网页版刷机。
> 本文档中的驱动步骤还没有在 Windows 7 真机上验证，第一次照做时请记录结果并更新本文档。

## 1. 为什么桌面 App 不能在 Windows 7 上运行

| 组件 | 当前版本 | Windows 7 情况 |
| --- | --- | --- |
| 桌面外壳 | Go 1.26 + Wails v3 | Go 1.21 起要求 Windows 10；Go 1.20 是最后支持 Windows 7/8 的版本 |
| 界面 | WebView2 | WebView2 Runtime 109 是最后支持 Windows 7/8/8.1 的版本 |
| 刷机 | Go 实现（`internal/esp`），已不依赖 Python | 随外壳一起用 Go 1.20 编译即可 |

Windows 7 版本要用 Go 1.20、WebView2 109（或改用其他界面方案）重新打包，所以单独作为一个版本来做。刷机已经改成 Go 实现，不再需要 Python。

## 2. Windows 7 用户现在怎么刷机：Chrome 109 + 网页版

1. 安装 **Google Chrome 109**（或 Microsoft Edge 109）。这是最后支持 Windows 7/8/8.1 的版本，Chrome 110 起不再支持。
   - 网页版刷机使用 Web Serial API，Chrome 89 起支持，所以 Chrome 109 可以用。
   - Chrome 109 不再获得安全更新，只建议用于刷机。
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

## 4. 以后做 Windows 7 版本时要做的事

- 外壳改用 Go 1.20 编译（或换用 Windows 7 能运行的界面方案），界面固定 WebView2 Runtime 109 或改成本机浏览器打开。
- 刷机已改用 Go（`internal/esp`、`internal/device`），不依赖 Python。要确认这些代码和依赖（go.bug.st/serial、golang.org/x/sys）能用 Go 1.20 编译；代码里用了 Go 1.21+ 的 `min`、`for range n` 等写法，需要改写。
- 内置驱动安装：检测 `VID_303A&PID_1001&MI_00` 是否有驱动；没有就引导安装。需要 FoloToy 自己签名的 CDC INF（带 `.cat`），或在程序里用 libwdi 生成并安装。
- 用 VirtualBox 里的 Windows 7 虚拟机 + USB 直通真实 AI Passport 测试（可用 Fleet 的 `win` 机器）。

## 参考

- Zadig：https://zadig.akeo.ie/
- Windows 7/8 上 ESP32-C3 内置 USB 不可用的讨论：https://esp32.com/viewtopic.php?p=135064
- 社区提供的 Windows 7 CDC INF（针对 ESP32-S3，C3 结果不一）：https://github.com/kutukvpavel/Esp32-Win7-VCP-drivers
