# AI Passport 玩法社区（离线桌面版）开发方案

## 1. 目标

把 https://ai-passport.folotoy.cn/plays/ 做成可离线使用的桌面 App（macOS + Windows）。

- 安装包不带任何玩法、图片、固件。
- 首次打开引导用户选择存储目录并点"更新"，把全部玩法数据、图片、固件下载到该目录。
- 之后浏览、搜索、查看详情、刷机都完全离线；有网时可手动增量更新。
- 界面中英文双语。只支持 AI Passport（ESP32-C3）单台刷机。

不做：登录、评论、点赞、收藏、Remix、讨论区、发布。

## 2. 技术栈（沿用 Magic-Flash）

| 部分 | 选型 |
| --- | --- |
| 桌面外壳 | Go 1.26 + Wails v3（与 Magic-Flash 同版本） |
| 界面 | 原生 HTML/CSS/JS，`embed` 打包进二进制，不开 HTTP 端口 |
| 刷机 | Go 实现的 ESP32 串口协议（`internal/esp`）+ esp-flasher-stub（Apache/MIT），在 App 进程内运行（2026-11-12 起；之前是 PyInstaller 打包的 Python esptool worker） |
| 单实例 | 复用 Magic-Flash `internal/instance` |
| 打包 | 复用 `scripts/package-mac.sh`、`package-windows.ps1`、Inno Setup、GitHub Actions |

## 3. 数据源（已实测，无需登录）

- 列表：`GET /api/plays?limit=50&offset=N`（limit 上限 50），共 1262 条。
  响应含 `pagination.total/hasMore`、`categoryCounts`、`discoveryTags`、`discoveryTabs`、`taxonomyVersion`。
- 单个玩法字段：`id, slug, revisionId, title/description/instructions/changelog {zh,en}, categoryKey, tags,
  mustPlay, multiDevice, childFriendly, author, authorProfile, image, thumbnail, images[], video,
  firmware {url, size, sha256, format="esp-merged-0x0"}, views, downloads, hearts, publishedAt, updatedAt`。
- 图片：`/api/media/plays/...webp`；作者头像：`/api/avatars/...webp`。
- 固件：`/api/download/community/<slug>`（官方玩法为 `/api/download/official/<slug>`）。
- 注意：下载接口**不支持 Range**，断点续传只能以"文件"为单位重下。

固件特征（9 个样本全部一致）：首字节 `0xE9`，扩展头 chip_id = `0x0005`（ESP32-C3），
Flash 大小 8MB，从 `0x0` 写入。平均约 2.5MB，最大 8MB，总量约 3GB。

> 作为官方项目，建议后端补一个只读的"离线同步清单"接口（一次返回全部 id/revisionId/sha256/资源 URL），
> 并给桌面端单独的 User-Agent，便于统计和限流。首版先用现有公开接口。

## 4. 目录结构

```
ai-passport-plays/
  main.go
  frontend/            index.html app.css app.js i18n/{zh,en}.json embed.go
  internal/
    catalog/           本地索引读写、搜索、分类筛选
    sync/              拉取列表、对比差异、并发下载、校验、原子替换
    firmware/          AI Passport 固件校验（芯片、大小、sha256、格式）
    flash/             串口枚举、调用 worker 刷机、进度事件
    desktop/           前端 API 路由（同 Magic-Flash desktop/http.go，token 鉴权）
    settings/          settings.json（存储目录、语言、上次同步）
    instance/          单实例锁（复用）
    worker/            worker 进程管理（复用）
  worker/main.py       esptool：ports / validate / flash（目标固定 esp32c3）
  scripts/             打包脚本（复用改名）
```

App 私有配置：`<UserConfigDir>/FoloToy/AI Passport Plays/settings.json`，只存设置，不存玩法。

玩法存储目录（用户选择，默认 `~/Documents/AI Passport Plays`）：
```
index.json                 分类、专题、全部玩法摘要、taxonomyVersion、syncedAt
plays/<slug>/meta.json     完整数据 + revisionId
plays/<slug>/cover.webp thumb.webp gallery/N.webp
plays/<slug>/firmware.bin
avatars/<hash>.webp
.tmp/                      下载中的临时文件
```

## 5. 页面

1. **首次引导**：没有 `index.json` 时出现 → 选择目录（显示可用空间，要求 ≥ 4GB）→ "开始更新"。
2. **主页**：顶部专题区（必玩精选 / 多人玩法 / 亲子益智 / 日常办公）+ 搜索框 + 玩法卡片网格（缩略图、标题、作者、标签），布局对齐网站。
3. **分类页**：分类列表带数量（游戏、效率、资讯、学习、影音、开发者、社交 + 专题），点击筛选，支持排序（最新 / 最热）。
4. **详情页**：封面、图集、简介、玩法说明（Markdown 渲染）、作者、更新日志、"刷入设备"按钮。
5. **刷机弹窗**：选串口 → 确认 → 进度条 → 完成 / 失败原因 + 重试。
6. **设置页**：存储目录（更换时可选择迁移或重新下载）、语言（中 / EN / 跟随系统）、检查更新、上次同步时间、占用空间、版本号。
7. **更新进度页**：总进度、当前项、速度、已下载 / 总大小、失败列表；支持暂停、取消、重试失败项。

## 6. 更新逻辑

1. 分页拉取全部列表（每页 50，约 26 次请求）。
2. 和本地 `meta.json` 比较 `revisionId` 与 `firmware.sha256`，生成 新增 / 变更 / 已下架 三个清单。
3. 并发 4 个下载：先下到 `.tmp/`，固件校验 sha256 和大小，通过后原子移动到目标位置。
4. 每完成一个玩法就写入它的 `meta.json`；全部结束后再写 `index.json`（写入时先写临时文件再改名）。
5. 中途中断：下次更新自动跳过已完成的项。
6. 已下架的玩法：本地标记"已下架"，保留文件，可在设置里清理。
7. 启动时如果有网，后台静默检查数量或版本变化，有更新就在顶部提示，不自动下载。

## 7. 刷机与芯片检查

刷机前，Go 端检查：
- 文件首字节 `0xE9`，大小和 sha256 与 `meta.json` 一致。
- 镜像头 chip_id == `0x0005`（ESP32-C3），否则拒绝。

worker 端（esptool）：
- 连接后读取芯片型号，必须是 ESP32-C3，否则提示"这不是 AI Passport"并停止。
- 读取 Flash 大小，必须 ≥ 8MB。
- 从 `0x0` 写入完整镜像 → 校验 → 硬复位。波特率先用 921600，失败自动降到 115200。

其他：
- 串口列表按 USB VID/PID 过滤，优先显示 AI Passport（ESP32-C3 内置 USB-JTAG：303A:1001）。
- 同时只允许刷一台；刷机进行中禁止关闭窗口（同 Magic-Flash）。

## 8. 双语

- 界面文字放在 `i18n/zh.json` / `en.json`；玩法内容直接使用接口的 `{zh, en}` 字段，英文为空时回退中文。
- 分类和专题名称使用接口返回的 `name.zh / name.en`。

## 9. 开发步骤

| 阶段 | 内容 | 验收 |
| --- | --- | --- |
| M1 骨架 | Wails 工程、embed 前端、settings、单实例、i18n 切换 | 空壳 App 在 mac 上启动，可切换语言 |
| M2 同步 | sync 模块 + 首次引导 + 更新进度页 + 设置页选目录 | 全量下载 1262 个玩法，sha256 全部通过；中断后可继续 |
| M3 浏览 | 主页、分类、搜索、详情、图集 | 断网后浏览正常，界面布局对齐网站 |
| M4 刷机 | worker（esp32c3）、串口选择、芯片检查、进度 | 实机刷入一个玩法并启动成功；拿非 C3 板测试，应被拒绝 |
| M5 打包 | mac .app/.dmg（arm64 + x64）、Windows 安装包 + 便携 ZIP、CI | 两个平台安装后全流程跑通 |
| M6 收尾 | 增量更新、下架处理、错误提示、单元测试 | 测试和 race 检查通过 |

## 10. 已确认的决定（2026-10-23）

- 下载范围：A，首次更新全量下载。
- 平台：先做 macOS，再做 Windows；暂无签名证书（ad-hoc 签名）。
- 刷机：单台；只支持 AI Passport，刷机前检查芯片必须是 ESP32-C3。
- 设备 USB 免驱（与浏览器 Web Serial 刷机一致）。
- 界面中英文；"分类特别难"指"分类 + 专题"。
- 技术栈沿用 Magic-Flash；项目名"AI Passport 玩法社区"，仓库 FoloToy/ai-passport-plays（2026-11-13 起公开，MIT）。

## 11. 实施中发现的问题

- **固件下载接口按 IP 限流**：连续下载约 20–30 个后返回 HTTP 429（"操作过于频繁"），封禁超过 25 分钟。图片和列表接口不受限。
  对策：更新分两阶段（先下载玩法信息和图片，可以先浏览；再排队下载固件）；遇到 429 时所有固件下载一起暂停并退避（30 秒起，最长 10 分钟）；点"刷入"时，未下载的固件会插到队列最前面。
  **建议后端**：对 `User-Agent: AIPassportPlays/*` 放宽限流，或者提供 CDN / 离线数据包，否则 1,260 个固件首次全量下载会持续数小时。
- 下载接口不支持 Range 请求，断点续传以文件为单位。
- **社区玩法会覆盖设备身份区**（2026-11-08 发现）：身份区 `cardid` 在 `0x356000–0x359FFF`，保存每台设备的序列号和密钥。社区玩法是从 `0x0` 写入的合并镜像，本机已下载的 441 个固件里有 135 个超过 `0x356000`，刷入后身份区会被覆盖（网页版刷机同样如此）。
  目前的处理：刷入前把有内容的身份区备份到 `<备份目录>/cardid/<MAC>/`，并在刷机结果里提示。是否改为刷入时跳过身份区、或在恢复出厂时写回本机备份，需要产品决定。

## 12. 恢复出厂（2026-11-08 整合）

原 FoloToy-AI-Passport-Factory-Restore（Windows、Tkinter、esptool 4.12、v1.5.2）整合进本 App，之后关闭原仓库。

- 出厂固件 `AI-Passport-Factory-Default-v1.1.2` 内置在 Go 程序中（`internal/factory`），启动恢复时解压并校验。
- 恢复流程最初移植到 `worker/restore.py`（esptool 5.6 Python API），2026-11-12 改写为 Go（`internal/device`），整个流程只连接一次。安全规则不变：分段写入、不擦除、不写身份区、写前备份、写后校验、身份区前后一致。
- 自动抓取设备的逻辑（5 ms 级 DTR/RTS 复位、Windows Code 43 处理）原样保留；macOS 只使用 `/dev/cu.*`。
- 写入开始后不能停止。刷入玩法和恢复出厂共用一个任务槽，同时只能运行一个。
- 2026-11-11：新增"备份固件"（只读，读整片 Flash 并与设备端 MD5 核对），与恢复出厂放在同一个"设备工具"页，复用同一套自动找设备逻辑。

## 13. 待办

- Windows 安装包：脚本已写好（`scripts/build-on-windows.ps1`，见 `docs/WINDOWS-BUILD.md`），待在真实 Windows 上运行和验收。
- Developer ID 签名、公证；Windows 代码签名。
- 实机刷机验收（需要连接 AI Passport）。
- Go 刷机（2026-11-12 替换 Python esptool）实机验收：刷入玩法、恢复出厂、备份固件、USB-JTAG 复位进下载模式、460800 波特率切换、Windows Code 43 抓取。目前只在 QEMU（esp-develop-9.2.2）上验证过。
- Windows 7/8 版本（2026-11-12 完成，v0.4.0）：go-legacy-win7 编译、Chrome 109 显示界面、内置 usbser INF 一键安装驱动。待在 Windows 7 真机上验收，见 docs/WINDOWS-7.md 第 4 节。
- 备份固件实机验收（macOS 和 Windows）。
- 恢复出厂实机验收（macOS 和 Windows）：正常设备、USB 只短暂出现的设备、拔插后端口变化、同时连两台、端口被占用、完整恢复后身份区一致。
