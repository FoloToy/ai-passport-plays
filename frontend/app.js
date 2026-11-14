'use strict';
/* AI Passport 玩法社区 — offline desktop UI. Plain JS, no external resources. */

const $ = (s, root = document) => root.querySelector(s);
const meta = name => $(`meta[name="${name}"]`).content;
const TOKEN = meta('plays-token');

/* ---------------- i18n ---------------- */
const I18N = {
  zh: {
    nav_library: '玩法社区', nav_settings: '设置',
    sidebar_title: '玩法分类', all_plays: '全部玩法', topics: '专题',
    hero_title: '玩法社区', hero_count: '款玩法', hero_sub: '把喜欢的创意，写进你的 AI Passport',
    featured_kicker: 'Curated by AI Passport', featured: '官方精选', featured_badge: '官方精选',
    tab_all: '全部玩法', search_ph: '搜索玩法名字、简介或作者…',
    sort_newest: '最新', sort_downloads: '下载最多', sort_hearts: '点赞最多', sort_comments: '讨论最多',
    results: n => `共 ${n} 款玩法`, developer: '开发者', install: '刷入', not_ready: '未下载', removed: '已下架',
    updated_today: '今日更新', updated_yesterday: '昨日更新', updated_days: n => `${n} 天前更新`,
    empty_title: '没有找到玩法', empty_sub: '换个关键词或分类试试',
    detail_title: '玩法详情', tab_intro: '玩法介绍', tab_usage: '使用说明', tab_changelog: '更新日志',
    nothing_here: '暂无内容', flash_device: '刷入设备', flash_hint: '刷入会覆盖设备上当前的玩法。',
    open_folder: '打开文件夹', open_web: '网页版', firmware: '固件', size: '大小',
    no_firmware: '该玩法的固件还没有下载，请先更新玩法库。', removed_note: '该玩法已从社区下架，本地仍保留了一份。',
    downloads: '下载', hearts: '点赞', favorites: '收藏', comments: '评论',
    // onboarding
    welcome_title: '欢迎使用 AI Passport 玩法社区',
    welcome_sub: '首次使用需要把玩法社区的全部玩法、图片和固件下载到电脑上。下载完成后，不联网也能浏览和刷机。',
    folder_label: '玩法存储目录', change: '更改', start_update: '开始更新',
    fact_plays: '玩法数量', fact_size: '预计占用', fact_free: '可用空间', about_n: n => `约 ${n}`,
    unknown: '未知', path_ph: '输入一个完整的文件夹路径',
    // sync
    sync_title: '更新玩法库', sync_running: '正在更新…', sync_done: '更新完成', sync_failed: '更新失败', sync_cancelled: '已停止更新',
    phase_listing: '正在获取玩法列表', phase_topics: '正在获取专题', phase_planning: '正在检查本地文件',
    phase_catalog: '正在下载玩法信息和图片', phase_firmware: '正在下载固件', phase_indexing: '正在整理玩法库', phase_moving: '正在迁移玩法目录',
    throttled: s => `固件服务器限流中，${s} 后自动继续`, firmware_progress: (a, b) => `固件 ${a} / ${b}`, pill_firmware: '下载固件', pill_wait: '等待服务器',
    browse_while: '玩法信息已就绪，可以先浏览；固件会在后台继续下载。', no_fw_btn: '暂无固件',
    fetch_title: '正在下载这个玩法的固件…', fetch_wait: s => `固件服务器繁忙，请在 ${s} 后重试。`, fetch_queue: '已插队到下载队列最前面…',
    done_count: '已完成', total_count: '总数', failed_count: '失败', downloaded: '已下载', speed: '速度',
    stop: '停止', close: '关闭', retry_failed: '重新下载失败项', run_in_bg: '后台运行', continue_update: '继续更新',
    added: n => `新增 ${n}`, updated: n => `更新 ${n}`, removed_n: n => `下架 ${n}`,
    sync_pill: '更新中', update_available: '有新玩法', update_now: '立即更新',
    banner_incomplete: '上次更新没有完成，部分玩法还不能刷入。', banner_update: n => `玩法社区有 ${n} 个新玩法或更新。`,
    move_title: '迁移玩法目录', moving_pill: '迁移中',
    // flash
    flash_title: '刷入设备', step_connect: '用 USB 数据线把 AI Passport 连接到电脑，然后选择设备。',
    searching: '正在查找设备…', no_ports: '没有找到设备。请检查：① 设备已用 USB 线连到电脑；② 数据线能传数据（有些线只能充电）；③ 换一个 USB 口；④ 仍然没有时，按住 BOOT 键再插入 USB。列表会自动刷新。',
    no_passport: '没有找到 AI Passport，下面只有其他串口设备。请插好 AI Passport，列表会自动刷新；不确定时不要选其他设备。',
    r_not_seen: '还没有发现任何 AI Passport。请检查数据线能传数据、换一个 USB 口，或按住 BOOT 键再插入 USB。',
    r_waited: (s, max) => `已等待 ${s} 秒（最多 ${max} 秒）`,
    err_no_device: '没有找到设备：所选的串口已经不在了。请插好设备，点"刷新"后重新选择。',
    port_recommended: 'AI Passport', refresh: '刷新', start_flash: '开始刷入', cancel: '取消',
    flash_warn: '刷入过程中请勿拔出设备或关闭本程序。',
    stage_checking: '正在校验固件', stage_connecting: '正在连接设备', stage_writing: '正在写入固件',
    stage_verifying: '正在校验写入结果', stage_resetting: '正在重启设备',
    flash_done: '刷入完成', flash_done_sub: '设备已自动重启，正在进入新玩法。', flash_done_replug: '设备没有自动重启，请拔插一次 USB。', flash_failed: '刷入失败',
    retry: '重试', done: '完成', log: '详细日志', chip: '芯片', mac: 'MAC', flash_size: 'Flash',
    // settings
    settings_title: '设置', s_library: '玩法存储目录', s_library_sub: '玩法、图片和固件都保存在这里。',
    change_dir: '更改目录', reveal: '在访达中打开', reveal_win: '在资源管理器中打开', usage: '已占用', files: '文件数', free: '可用空间',
    s_data: '玩法数据', s_data_sub: '从 AI Passport 玩法社区下载最新的玩法。', last_sync: '上次更新',
    never: '从未更新', plays_ready: '可刷入玩法', check_update: '检查更新', checking: '正在检查…',
    up_to_date: '已是最新', status: '状态', complete: '完整', incomplete: '未完成',
    s_language: '语言', lang_auto: '跟随系统', s_about: '关于', version: '版本', worker: '刷机组件',
    worker_ok: '已就绪', worker_missing: '缺失（请使用完整安装包）', source: '数据来源',
    move_q: '要把现有的玩法一起迁移到新目录吗？', move_yes: '迁移现有玩法', move_no: '只切换目录',
    move_no_sub: '新目录中没有玩法时，需要重新下载。',
    dir_changed: '玩法目录已更改', saved: '已保存',
    // errors
    err_network: '网络连接失败，请检查网络后重试。', err_disk_space: '磁盘空间不足。', err_busy: '有任务正在进行，请稍后再试。',
    err_target: '无法写入所选目录。', err_not_empty: '新目录不是空的，请选择一个空文件夹。', err_path: '目录无效。',
    err_wrong_chip: '检测到的芯片不是 ESP32-C3，这不是 AI Passport。', err_secure: '设备开启了安全启动或 Flash 加密，无法刷入。',
    err_flash_size: '设备 Flash 容量不足 8 MB。', err_connect: '无法连接设备。请重新插拔数据线后重试；如仍失败，按住 BOOT 键再插入 USB。',
    err_checksum: '固件校验失败，请重新更新玩法库。', err_image: '固件文件无效。', err_image_chip: '该固件不是为 AI Passport (ESP32-C3) 构建的。',
    err_worker: '刷机组件不可用。', err_other: '发生错误。', err_no_picker: '请直接输入文件夹路径。',
    err_rate_limited: '固件服务器繁忙，请稍后重试。', err_no_firmware: '这个玩法没有可下载的固件。', err_not_ready: '固件还没有下载。',
    // factory restore
    nav_restore: '设备工具', tools_title: '设备工具', tools_lead: '备份或恢复 AI Passport 上的固件。两个工具都会自动找到设备，不用选串口。', restore_title: '恢复出厂',
    backup_title: '备份固件', backup_lead: '把设备上现有的固件完整读出来，保存到电脑上（8 MB）。只读取，不会改动设备。刷入新玩法或恢复出厂前，可以先备份一份。',
    backup_start: '开始备份', backup_again: '再备份一次', b_idle_sub: '连接一台 AI Passport 后点“开始备份”；等待连接时按提示拔插一次 USB。',
    b_backup: '正在读取固件', b_backup_sub: '大约需要 1–3 分钟，请保持设备连接', b_verifying: '正在核对备份', b_verifying_sub: '和设备上的内容逐字节比对',
    b_done: '备份完成', b_done_sub: '固件已保存，并与设备上的内容核对一致。', b_failed: '备份没有完成', b_saved_to: '保存位置',
    b_warn: '备份过程中请勿拔出设备。备份只读取设备，不会写入任何内容。', r_busy_backup: '正在备份固件，完成后才能开始其他操作。', r_busy_restore: '正在恢复出厂，完成后才能开始其他操作。',
    restore_lead: '把 AI Passport 恢复成出厂固件。适用于刷入玩法后无法启动、反复重启，或连接电脑后很快断开的情况。',
    restore_step1: '只连接一台需要恢复的 AI Passport', restore_step2: '点击“一键恢复出厂”；等待连接时，按提示拔插一次 USB',
    restore_start: '一键恢复出厂', restore_confirm: '请确认电脑只连接了一台 AI Passport。开始后请勿拔线，也不要关闭本程序。',
    restore_go: '开始恢复', r_warn: '恢复过程中请勿拔出设备或关闭本程序。', restore_stop: '停止', restore_stopping: '正在安全停止…', restore_again: '再次恢复',
    r_idle: '准备就绪', r_idle_sub: '无需选择串口，程序会自动找到设备。',
    r_preparing: '正在准备恢复', r_preparing_sub: '正在进行安全检查',
    r_waiting: '正在等待设备', r_replug: '如果设备没有出现，请拔插一次 USB。', r_multiple: '检测到多台设备，请只保留需要恢复的那一台。',
    r_code43: p => `Windows 报告设备 USB 异常（${p}），请拔插一次设备。`,
    r_connecting: '设备已连接', r_connecting_sub: '正在检查设备',
    r_backup: '正在备份', r_backup_sub: '请保持设备连接',
    r_writing: '正在写入出厂固件', r_writing_sub: '请勿拔线或关闭本程序',
    r_verifying: '正在校验恢复结果', r_verifying_sub: '马上就好，请勿拔线',
    r_resetting: '正在重启设备', r_resetting_sub: '马上就好',
    r_done: '恢复出厂成功', r_done_sub: '设备已自动重启。', r_done_replug: '设备没有自动重启，请拔插一次 USB。',
    r_failed: '没有恢复成功', r_failed_sub: '拔插设备后可以再试一次。',
    r_cancelled: '已停止', r_cancelled_sub: '没有写入任何内容。',
    r_busy_flash: '正在刷入玩法，完成后才能恢复出厂。',
    r_blank_id: '设备的身份区（序列号和密钥）是空的。出厂固件已写入，但设备可能无法联网认证，请联系 FoloToy 并提供 MAC 地址。',
    r_unknown_id: '没有在设备的身份区找到完整的序列号和密钥。如果设备无法联网认证，请联系 FoloToy 并提供 MAC 地址。',
    open_backup: '打开备份文件夹', r_options: '选项', r_backup_dir: '备份位置', r_full: '同时备份整片 Flash（推荐，约 8 MB，多花 1–2 分钟）',
    r_safety: '恢复时不会擦除整片 Flash，也不会改写设备的身份区（序列号和密钥）。写入前会先备份，写入后会逐段校验。',
    r_fw: '出厂固件', restore_link: '设备无法启动？试试恢复出厂',
    cardid_overwritten: '这个玩法覆盖了设备的身份区（序列号和密钥）。改动前的内容已备份到备份文件夹，需要时可联系 FoloToy 恢复。',
    err_not_found: '150 秒内没有找到设备。请关闭其他串口工具，按住 BOOT 键再插入 USB 后重试。',
    err_firmware: '内置的出厂固件校验失败，请重新安装本程序。', err_backup: '读取设备备份失败，请拔插设备后重试。',
    err_write: '写入或校验出厂固件失败。请不要拔线，直接再恢复一次。', err_cardid_changed: '严重错误：设备身份区在恢复过程中发生了变化。请停止使用这台设备，并联系 FoloToy。',
    err_backup_dir: '无法写入备份目录。', err_not_cancellable: '已开始写入，为避免设备损坏，会等写入和校验完成。', err_cancelled: '已停止。',
  },
  en: {
    nav_library: 'App Library', nav_settings: 'Settings',
    sidebar_title: 'Categories', all_plays: 'All apps', topics: 'Collections',
    hero_title: 'App Library', hero_count: 'apps', hero_sub: 'Put the ideas you love into your AI Passport',
    featured_kicker: 'Curated by AI Passport', featured: 'Staff picks', featured_badge: 'Staff pick',
    tab_all: 'All apps', search_ph: 'Search by name, description or author…',
    sort_newest: 'Newest', sort_downloads: 'Downloads', sort_hearts: 'Most liked', sort_comments: 'Most discussed',
    results: n => `${n} apps`, developer: 'by', install: 'Install', not_ready: 'Not downloaded', removed: 'Removed',
    updated_today: 'Updated today', updated_yesterday: 'Updated yesterday', updated_days: n => `Updated ${n}d ago`,
    empty_title: 'No apps found', empty_sub: 'Try another keyword or category',
    detail_title: 'App details', tab_intro: 'Overview', tab_usage: 'How to use', tab_changelog: 'Changelog',
    nothing_here: 'Nothing here yet', flash_device: 'Install to device', flash_hint: 'Installing replaces the app currently on the device.',
    open_folder: 'Show files', open_web: 'Web page', firmware: 'Firmware', size: 'Size',
    no_firmware: 'This firmware has not been downloaded yet. Update the library first.', removed_note: 'This app was removed from the community; a local copy is kept.',
    downloads: 'Downloads', hearts: 'Likes', favorites: 'Saves', comments: 'Comments',
    welcome_title: 'Welcome to the AI Passport App Library',
    welcome_sub: 'First, download every app, image and firmware from the community to this computer. After that you can browse and install apps without a network.',
    folder_label: 'Library folder', change: 'Change', start_update: 'Start download',
    fact_plays: 'Apps', fact_size: 'Estimated size', fact_free: 'Free space', about_n: n => `about ${n}`,
    unknown: 'Unknown', path_ph: 'Type a full folder path',
    sync_title: 'Update library', sync_running: 'Updating…', sync_done: 'Update complete', sync_failed: 'Update failed', sync_cancelled: 'Update stopped',
    phase_listing: 'Fetching the app list', phase_topics: 'Fetching collections', phase_planning: 'Checking local files',
    phase_catalog: 'Downloading app info and images', phase_firmware: 'Downloading firmware', phase_indexing: 'Building the library', phase_moving: 'Moving the library folder',
    throttled: s => `The firmware server is rate limiting; resuming in ${s}`, firmware_progress: (a, b) => `Firmware ${a} / ${b}`, pill_firmware: 'Firmware', pill_wait: 'Waiting',
    browse_while: 'App info is ready to browse; firmware keeps downloading in the background.', no_fw_btn: 'No firmware',
    fetch_title: 'Downloading this app\'s firmware…', fetch_wait: s => `The firmware server is busy. Try again in ${s}.`, fetch_queue: 'Moved to the front of the download queue…',
    done_count: 'Done', total_count: 'Total', failed_count: 'Failed', downloaded: 'Downloaded', speed: 'Speed',
    stop: 'Stop', close: 'Close', retry_failed: 'Retry failed items', run_in_bg: 'Run in background', continue_update: 'Continue',
    added: n => `${n} new`, updated: n => `${n} updated`, removed_n: n => `${n} removed`,
    sync_pill: 'Updating', update_available: 'New apps', update_now: 'Update now',
    banner_incomplete: 'The last update did not finish; some apps cannot be installed yet.', banner_update: n => `${n} new or updated apps are available.`,
    move_title: 'Move library', moving_pill: 'Moving',
    flash_title: 'Install to device', step_connect: 'Connect your AI Passport with a USB data cable, then pick the device.',
    searching: 'Looking for devices…', no_ports: 'No device found. Check that: ① the device is connected by USB; ② the cable carries data (some only charge); ③ another USB port helps; ④ if not, hold BOOT while plugging in USB. The list refreshes by itself.',
    no_passport: 'No AI Passport found; only other serial devices are listed. Plug in the AI Passport and the list refreshes by itself. Do not pick another device unless you are sure.',
    r_not_seen: 'No AI Passport has appeared yet. Check that the cable carries data, try another USB port, or hold BOOT while plugging in USB.',
    r_waited: (s, max) => `Waited ${s} s (up to ${max} s)`,
    err_no_device: 'Device not found: the selected port is gone. Plug in the device, click Refresh and choose it again.',
    port_recommended: 'AI Passport', refresh: 'Refresh', start_flash: 'Install', cancel: 'Cancel',
    flash_warn: 'Do not unplug the device or quit the app while installing.',
    stage_checking: 'Checking firmware', stage_connecting: 'Connecting to the device', stage_writing: 'Writing firmware',
    stage_verifying: 'Verifying', stage_resetting: 'Restarting the device',
    flash_done: 'Installed', flash_done_sub: 'The device has restarted into the new app.', flash_done_replug: 'The device did not restart by itself. Unplug and replug USB once.', flash_failed: 'Install failed',
    retry: 'Retry', done: 'Done', log: 'Details', chip: 'Chip', mac: 'MAC', flash_size: 'Flash',
    settings_title: 'Settings', s_library: 'Library folder', s_library_sub: 'Apps, images and firmware are stored here.',
    change_dir: 'Change folder', reveal: 'Show in Finder', reveal_win: 'Show in Explorer', usage: 'Used', files: 'Files', free: 'Free space',
    s_data: 'App data', s_data_sub: 'Download the latest apps from the AI Passport community.', last_sync: 'Last update',
    never: 'Never', plays_ready: 'Installable apps', check_update: 'Check for updates', checking: 'Checking…',
    up_to_date: 'Up to date', status: 'Status', complete: 'Complete', incomplete: 'Incomplete',
    s_language: 'Language', lang_auto: 'System', s_about: 'About', version: 'Version', worker: 'Flashing component',
    worker_ok: 'Ready', worker_missing: 'Missing (use the full installer)', source: 'Source',
    move_q: 'Move the existing apps to the new folder?', move_yes: 'Move apps', move_no: 'Only switch folder',
    move_no_sub: 'If the new folder has no apps, download them again.',
    dir_changed: 'Library folder changed', saved: 'Saved',
    err_network: 'Network error. Check your connection and try again.', err_disk_space: 'Not enough disk space.', err_busy: 'Another task is running. Try again later.',
    err_target: 'Cannot write to the selected folder.', err_not_empty: 'The new folder is not empty. Choose an empty folder.', err_path: 'Invalid folder.',
    err_wrong_chip: 'The detected chip is not ESP32-C3, so this is not an AI Passport.', err_secure: 'Secure Boot or Flash Encryption is enabled; cannot install.',
    err_flash_size: 'The device flash is smaller than 8 MB.', err_connect: 'Cannot connect to the device. Reconnect the cable and retry; if it still fails, hold BOOT while plugging in USB.',
    err_checksum: 'Firmware check failed. Update the library again.', err_image: 'Invalid firmware file.', err_image_chip: 'This firmware is not built for AI Passport (ESP32-C3).',
    err_worker: 'The flashing component is unavailable.', err_other: 'Something went wrong.', err_no_picker: 'Type the folder path instead.',
    err_rate_limited: 'The firmware server is busy. Try again later.', err_no_firmware: 'This app has no downloadable firmware.', err_not_ready: 'Firmware not downloaded yet.',
    // factory restore
    nav_restore: 'Device tools', tools_title: 'Device tools', tools_lead: 'Back up or restore the firmware on an AI Passport. Both tools find the device by themselves; no port to choose.', restore_title: 'Factory restore',
    backup_title: 'Back up firmware', backup_lead: 'Read the whole firmware on the device and save it on this computer (8 MB). It only reads; the device is not changed. Make one before installing an app or restoring.',
    backup_start: 'Start backup', backup_again: 'Back up again', b_idle_sub: 'Connect one AI Passport and click “Start backup”; replug USB once if asked while it waits.',
    b_backup: 'Reading the firmware', b_backup_sub: 'About 1–3 minutes; keep the device connected', b_verifying: 'Checking the backup', b_verifying_sub: 'Comparing it with the device',
    b_done: 'Backup finished', b_done_sub: 'The firmware is saved and matches the device.', b_failed: 'The backup did not finish', b_saved_to: 'Saved to',
    b_warn: 'Do not unplug the device during the backup. It only reads; nothing is written to the device.', r_busy_backup: 'A firmware backup is running. Wait for it to finish.', r_busy_restore: 'A factory restore is running. Wait for it to finish.',
    restore_lead: 'Put the factory firmware back on an AI Passport. Use it when the device will not start after installing an app, keeps rebooting, or disconnects soon after you plug it in.',
    restore_step1: 'Connect only the AI Passport you want to restore', restore_step2: 'Click “Restore factory firmware”; while it waits, unplug and replug USB once if asked',
    restore_start: 'Restore factory firmware', restore_confirm: 'Make sure only one AI Passport is connected. Do not unplug it or close the app once it starts.',
    restore_go: 'Start', r_warn: 'Do not unplug the device or quit the app during the restore.', restore_stop: 'Stop', restore_stopping: 'Stopping safely…', restore_again: 'Restore again',
    r_idle: 'Ready', r_idle_sub: 'No port to choose; the app finds the device by itself.',
    r_preparing: 'Preparing', r_preparing_sub: 'Running safety checks',
    r_waiting: 'Waiting for the device', r_replug: 'If nothing happens, unplug and replug USB once.', r_multiple: 'More than one device is connected. Keep only the one to restore.',
    r_code43: p => `Windows reports a USB error on ${p}. Unplug and replug the device once.`,
    r_connecting: 'Device connected', r_connecting_sub: 'Checking the device',
    r_backup: 'Backing up', r_backup_sub: 'Keep the device connected',
    r_writing: 'Writing factory firmware', r_writing_sub: 'Do not unplug or close the app',
    r_verifying: 'Verifying', r_verifying_sub: 'Almost done; do not unplug',
    r_resetting: 'Restarting the device', r_resetting_sub: 'Almost done',
    r_done: 'Factory firmware restored', r_done_sub: 'The device has restarted.', r_done_replug: 'The device did not restart by itself. Unplug and replug USB once.',
    r_failed: 'The restore did not finish', r_failed_sub: 'Replug the device and try again.',
    r_cancelled: 'Stopped', r_cancelled_sub: 'Nothing was written.',
    r_busy_flash: 'An app is being installed. Wait for it to finish first.',
    r_blank_id: 'The device identity area (serial number and keys) is empty. The factory firmware is installed, but the device may fail to sign in online. Contact FoloToy with the MAC address.',
    r_unknown_id: 'No complete serial number and keys were found in the identity area. If the device cannot sign in online, contact FoloToy with the MAC address.',
    open_backup: 'Open backup folder', r_options: 'Options', r_backup_dir: 'Backup folder', r_full: 'Also back up the whole flash (recommended; about 8 MB, 1–2 extra minutes)',
    r_safety: 'The restore never erases the whole flash and never rewrites the identity area (serial number and keys). It backs up first and verifies every segment afterwards.',
    r_fw: 'Factory firmware', restore_link: 'Device will not start? Try a factory restore',
    cardid_overwritten: 'This app overwrote the device identity area (serial number and keys). The previous contents are in the backup folder; contact FoloToy if you need them restored.',
    err_not_found: 'No device was found within 150 seconds. Close other serial tools, then hold BOOT while plugging in USB and try again.',
    err_firmware: 'The bundled factory firmware failed its check. Reinstall the app.', err_backup: 'Reading the backup from the device failed. Replug it and try again.',
    err_write: 'Writing or verifying the factory firmware failed. Keep it plugged in and restore again.', err_cardid_changed: 'Serious error: the identity area changed during the restore. Stop using this device and contact FoloToy.',
    err_backup_dir: 'Cannot write to the backup folder.', err_not_cancellable: 'Writing has started. To protect the device it will finish writing and verifying first.', err_cancelled: 'Stopped.',
  },
};

let lang = 'zh';
const t = (key, ...args) => {
  const v = (I18N[lang] && I18N[lang][key]) ?? I18N.zh[key] ?? key;
  return typeof v === 'function' ? v(...args) : v;
};
const pick = text => (text ? (lang === 'en' ? text.en || text.zh : text.zh || text.en) || '' : '');
const esc = v => String(v ?? '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
const media = p => (p ? '/media/' + p.split('/').map(encodeURIComponent).join('/') : '');
const errText = (code, fallback) => (I18N.zh['err_' + code] ? t('err_' + code) : fallback || t('err_other'));

function fmtBytes(n) {
  if (!n && n !== 0) return '—';
  const u = ['B', 'KB', 'MB', 'GB', 'TB'];
  let i = 0;
  while (n >= 1024 && i < u.length - 1) { n /= 1024; i++; }
  return (i >= 2 ? n.toFixed(1) : Math.round(n)) + ' ' + u[i];
}
const fmtNum = n => (n >= 10000 ? (n / 10000).toFixed(1) + (lang === 'zh' ? '万' : '0k') : String(n || 0));
const fmtTime = s => (s ? new Date(s).toLocaleString(lang === 'zh' ? 'zh-CN' : 'en-US', { hour12: false }) : t('never'));

function countdown(until) {
  const ms = new Date(until) - Date.now();
  if (!until || ms <= 0 || until[0] === '0') return '';
  const sec = Math.ceil(ms / 1000);
  return `${Math.floor(sec / 60)}:${String(sec % 60).padStart(2, '0')}`;
}

function updatedLabel(play) {
  if (!play.updatedAt) return '';
  const days = Math.floor((Date.now() - new Date(play.updatedAt)) / 86400000);
  if (days < 0 || days > 7) return '';
  return days === 0 ? t('updated_today') : days === 1 ? t('updated_yesterday') : t('updated_days', days);
}

/* ---------------- icons ---------------- */
const ICON = {
  search: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5"/></svg>',
  download: '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 4v11m0 0-4-4m4 4 4-4M5 20h14"/></svg>',
  heart: '<svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 20s-7-4.4-7-10a4 4 0 0 1 7-2.6A4 4 0 0 1 19 10c0 5.6-7 10-7 10z"/></svg>',
  bookmark: '<svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M6 4h12v16l-6-4-6 4z"/></svg>',
  chat: '<svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M4 5h16v11H8l-4 4z"/></svg>',
  close: '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2"><path d="M6 6l12 12M18 6 6 18"/></svg>',
  left: '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2"><path d="M15 6l-6 6 6 6"/></svg>',
  right: '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2"><path d="M9 6l6 6-6 6"/></svg>',
  folder: '<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M3 6h6l2 2h10v11H3z"/></svg>',
  usb: '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 3v14m0 0a2 2 0 1 0 0 4 2 2 0 0 0 0-4zm0-6 4-3V6m-4 8-4-3V8"/></svg>',
};

/* ---------------- API ---------------- */
async function api(path, body) {
  const opt = { headers: { 'X-Plays-Token': TOKEN } };
  if (body !== undefined) {
    opt.method = 'POST';
    opt.headers['Content-Type'] = 'application/json';
    opt.body = JSON.stringify(body);
  }
  const r = await fetch('/api/' + path, opt);
  const text = await r.text();
  let data = null;
  try { data = text ? JSON.parse(text) : null; } catch { data = null; }
  if (!r.ok) {
    const e = new Error((data && data.error) || text || r.statusText);
    e.code = data && data.code;
    throw e;
  }
  return data;
}

let toastTimer = null;
function toast(msg, isError) {
  const el = $('#toast');
  el.textContent = msg;
  el.className = 'toast' + (isError ? ' error' : '');
  el.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => { el.hidden = true; }, 3800);
}
const showError = e => toast(errText(e.code, e.message), true);

/* ---------------- state ---------------- */
const S = {
  restore: { full: true, confirm: false, logOpen: false },
  server: null,
  catalog: null,
  catalogKey: '',
  byId: new Map(),
  bySlug: new Map(),
  view: { cat: 'all', tab: 'all', q: '', sort: 'newest' },
  limit: 60,
  detail: { slug: null, img: 0, tab: 'intro' },
  modal: null, // 'sync' | 'flash' | 'move'
  flash: { slug: null, ports: null, port: '', scanning: false, scanTimer: null },
  pendingDir: '',
  checking: false,
};

function resolveLang() {
  const pref = S.server ? S.server.language : 'auto';
  if (pref === 'zh' || pref === 'en') return pref;
  if (S.server && S.server.systemLanguage) return S.server.systemLanguage;
  return (navigator.language || 'zh').toLowerCase().startsWith('zh') ? 'zh' : 'en';
}

async function loadCatalog() {
  try {
    S.catalog = await api('catalog');
  } catch {
    S.catalog = null;
  }
  S.byId = new Map();
  S.bySlug = new Map();
  if (S.catalog) {
    for (const p of S.catalog.plays) { S.byId.set(p.id, p); S.bySlug.set(p.slug, p); }
  }
}

function catalogKey(st) {
  const l = st.library;
  return [l.dir, l.hasIndex, l.syncedAt || ''].join('|');
}

/* ---------------- routing ---------------- */
function parseRoute() {
  const h = location.hash.replace(/^#/, '') || '/';
  const [path, query] = h.split('?');
  const params = new URLSearchParams(query || '');
  if (path.startsWith('/play/')) return { name: 'library', play: decodeURIComponent(path.slice(6)), params };
  if (path === '/settings') return { name: 'settings', params };
  if (path === '/restore') return { name: 'restore', params };
  return { name: 'library', params };
}

function libraryHash(v = S.view) {
  const p = new URLSearchParams();
  if (v.cat !== 'all') p.set('cat', v.cat);
  if (v.tab !== 'all') p.set('tab', v.tab);
  if (v.q) p.set('q', v.q);
  if (v.sort !== 'newest') p.set('sort', v.sort);
  const s = p.toString();
  return '#/' + (s ? '?' + s : '');
}

function applyRoute() {
  const r = parseRoute();
  if (r.name === 'library') {
    if (!r.play) {
      S.view = { cat: r.params.get('cat') || 'all', tab: r.params.get('tab') || 'all', q: r.params.get('q') || '', sort: r.params.get('sort') || 'newest' };
    }
    if (r.play !== S.detail.slug) S.detail = { slug: r.play || null, img: 0, tab: 'intro' };
  } else {
    S.detail.slug = null;
  }
  render();
}

/* ---------------- rendering: shell ---------------- */
function renderTop() {
  const r = parseRoute();
  $('#topnav').innerHTML = `
    <a href="${libraryHash()}" class="${r.name === 'library' ? 'active' : ''}">${t('nav_library')}</a>
    <a href="#/restore" class="${r.name === 'restore' ? 'active' : ''}">${t('nav_restore')}</a>
    <a href="#/settings" class="${r.name === 'settings' ? 'active' : ''}">${t('nav_settings')}</a>`;
  renderTopRight();
}

function renderTopRight() {
  const st = S.server;
  if (!st) return;
  let pill = '';
  const sy = st.sync;
  if (sy.running) {
    const pct = sy.total ? Math.round((sy.done / sy.total) * 100) : 0;
    const wait = countdown(sy.throttledUntil);
    const label = sy.kind === 'move' ? t('moving_pill') : wait ? `${t('pill_wait')} ${wait}` : sy.phase === 'firmware' ? `${t('pill_firmware')} ${sy.firmwareDone}/${sy.firmwareTotal}` : `${t('sync_pill')} ${pct}%`;
    pill = `<button class="pill ${wait ? 'amber' : 'green'}" data-action="open-sync"><span class="dot"></span>${label}<span class="mini-bar"><i data-w="${pct}"></i></span></button>`;
  } else if (st.update.available && st.library.hasIndex) {
    pill = `<button class="pill amber" data-action="start-sync"><span class="dot"></span>${t('update_available')} · ${t('update_now')}</button>`;
  }
  $('#topright').innerHTML = `${pill}<button class="lang-toggle" data-action="toggle-lang" title="Language">${lang === 'zh' ? 'EN' : '中文'}</button>`;
  applyWidths($('#topright'));
}

function applyWidths(root) {
  root.querySelectorAll('[data-w]').forEach(el => { el.style.width = Math.max(0, Math.min(100, Number(el.dataset.w))) + '%'; });
}

let renderedRoute = '';
function render() {
  if (!S.server) return;
  lang = resolveLang();
  document.documentElement.lang = lang === 'zh' ? 'zh-CN' : 'en';
  document.title = lang === 'zh' ? 'AI Passport 玩法社区' : 'AI Passport App Library';
  renderTop();
  const r = parseRoute();
  const lib = S.server.library;
  const app = $('#app');
  if (r.name === 'settings') {
    app.innerHTML = renderSettings();
  } else if (r.name === 'restore') {
    app.innerHTML = renderRestore();
  } else if (!lib.hasIndex || !S.catalog) {
    app.innerHTML = S.server.sync.running ? `<div class="center-page"><div class="welcome sync-card" id="sync-inline">${syncBody()}</div></div>` : renderWelcome();
  } else {
    const y = window.scrollY;
    app.innerHTML = renderLibrary();
    setupSentinel();
    if (renderedRoute === location.hash) window.scrollTo(0, y);
  }
  renderedRoute = location.hash;
  applyWidths(app);
  renderModal();
  if (r.name === 'settings') refreshUsage();
}

/* ---------------- welcome ---------------- */
function renderWelcome() {
  const lib = S.server.library;
  const dir = S.pendingDir || lib.dir || lib.defaultDir;
  const folder = S.server.native
    ? `<div class="folder">${ICON.folder}<code title="${esc(dir)}">${esc(dir)}</code><button class="btn small" data-action="choose-dir-welcome">${t('change')}</button></div>`
    : `<div class="folder">${ICON.folder}<input id="dir-input" value="${esc(dir)}" placeholder="${esc(t('path_ph'))}" spellcheck="false"></div>`;
  return `<div class="center-page"><div class="welcome">
    <img class="logo" src="/logo.svg" alt="">
    <h1>${t('welcome_title')}</h1>
    <p>${t('welcome_sub')}</p>
    <div class="section-kicker">${t('folder_label')}</div>
    <div class="sp8"></div>
    ${folder}
    <div class="facts">
      <div><b>1,200+</b><span>${t('fact_plays')}</span></div>
      <div><b>${t('about_n', '3.2 GB')}</b><span>${t('fact_size')}</span></div>
      <div><b>${lib.free ? fmtBytes(lib.free) : t('unknown')}</b><span>${t('fact_free')}</span></div>
    </div>
    ${S.server.sync.phase === 'failed' ? `<div class="notice red">${esc(errText(S.server.sync.errorCode, S.server.sync.error))}</div><div class="sp16"></div>` : ''}
    <div class="actions"><button class="btn primary large" data-action="welcome-start">${ICON.download} ${t('start_update')}</button></div>
  </div></div>`;
}

/* ---------------- library ---------------- */
function topicsList() { return (S.catalog && S.catalog.topics) || []; }
function tabTopics() { return topicsList().filter(x => x.key !== 'must-play'); }

function filtered() {
  const v = S.view;
  let list = S.catalog.plays;
  if (v.cat !== 'all') {
    const topic = topicsList().find(x => 'topic:' + x.key === v.cat);
    if (topic) { const ids = new Set(topic.ids); list = list.filter(p => ids.has(p.id)); }
    else list = list.filter(p => p.category === v.cat);
  }
  if (v.tab !== 'all') {
    const topic = topicsList().find(x => x.key === v.tab);
    if (topic) { const ids = new Set(topic.ids); list = list.filter(p => ids.has(p.id)); }
  }
  if (v.q.trim()) {
    const terms = v.q.trim().toLowerCase().split(/\s+/);
    list = list.filter(p => {
      const hay = [p.title.zh, p.title.en, p.description.zh, p.description.en, p.author.name, p.slug].join(' ').toLowerCase();
      return terms.every(x => hay.includes(x));
    });
  }
  const key = { downloads: 'downloads', hearts: 'hearts', comments: 'comments' }[v.sort];
  list = list.slice();
  if (key) list.sort((a, b) => b[key] - a[key] || b.id - a.id);
  else list.sort((a, b) => (b.publishedAt || '').localeCompare(a.publishedAt || '') || b.id - a.id);
  // Removed plays go last.
  list.sort((a, b) => (a.removed === b.removed ? 0 : a.removed ? 1 : -1));
  return list;
}

function categoryName(key) {
  const c = S.catalog.categories.find(x => x.key === key);
  return c ? pick(c.name) : key;
}

function renderLibrary() {
  const v = S.view;
  const cats = S.catalog.categories;
  const listed = S.catalog.plays.filter(p => !p.removed).length;
  const topicItems = topicsList().map(x => `<button class="nav-item ${v.cat === 'topic:' + x.key ? 'active' : ''}" data-action="cat" data-cat="topic:${esc(x.key)}"><i>${x.key === 'must-play' ? '★' : '#'}</i><span>${esc(pick(x.name))}</span><small>${x.ids.length}</small></button>`).join('');
  const sidebar = `<aside class="sidebar">
    <h2>${t('sidebar_title')}</h2>
    <button class="nav-item ${v.cat === 'all' ? 'active' : ''}" data-action="cat" data-cat="all"><i>◎</i><span>${t('all_plays')}</span><small>${listed}</small></button>
    ${cats.map(c => `<button class="nav-item ${v.cat === c.key ? 'active' : ''}" data-action="cat" data-cat="${esc(c.key)}"><i>${esc(c.icon)}</i><span>${esc(pick(c.name))}</span><small>${c.count}</small></button>`).join('')}
    ${topicItems ? `<h3>${t('topics')}</h3>${topicItems}` : ''}
  </aside>`;

  const showFeatured = v.cat === 'all' && v.tab === 'all' && !v.q;
  const list = filtered();
  const tabs = tabTopics();
  const st = S.server;
  let banner = '';
  if (st.sync.running && st.sync.phase === 'firmware') banner = `<div class="banner green"><p>${t('browse_while')}</p><button class="btn soft small" data-action="open-sync">${t('sync_title')}</button></div>`;
  else if (!st.sync.running && st.update.available) banner = `<div class="banner"><p>${t('banner_update', st.update.changes)}</p><button class="btn soft small" data-action="start-sync">${t('update_now')}</button></div>`;
  else if (!st.sync.running && !st.library.complete) banner = `<div class="banner"><p>${t('banner_incomplete')}</p><button class="btn soft small" data-action="start-sync">${t('continue_update')}</button></div>`;

  return `<div class="library">${sidebar}<section class="content">
    ${banner}
    <div class="page-head"><div>
      <h1>${t('hero_title')} <small><b>${listed}</b> ${t('hero_count')}</small></h1>
      <p>${t('hero_sub')}</p></div>
    </div>
    ${showFeatured ? renderFeatured() : ''}
    ${tabs.length ? `<div class="tabs"><button class="${v.tab === 'all' ? 'active' : ''}" data-action="tab" data-tab="all">${t('tab_all')}</button>${tabs.map(x => `<button class="${v.tab === x.key ? 'active' : ''}" data-action="tab" data-tab="${esc(x.key)}">${esc(pick(x.name))}<small>${x.ids.length}</small></button>`).join('')}</div>` : ''}
    <div class="filters">
      <label class="search">${ICON.search}<input id="search" type="search" value="${esc(v.q)}" placeholder="${esc(t('search_ph'))}" autocomplete="off"></label>
      <div class="segmented">${['newest', 'downloads', 'hearts', 'comments'].map(k => `<button class="${v.sort === k ? 'active' : ''}" data-action="sort" data-sort="${k}">${t('sort_' + k)}</button>`).join('')}</div>
    </div>
    ${v.cat !== 'all' || v.tab !== 'all' || v.q ? `<div class="result-line">${t('results', list.length)}</div>` : ''}
    <div id="grid-wrap">${renderGrid(list)}</div>
  </section></div>`;
}

function renderFeatured() {
  let ids = S.catalog.recommendations || [];
  let plays = ids.map(id => S.byId.get(id)).filter(p => p && !p.removed);
  if (plays.length < 4) {
    const must = topicsList().find(x => x.key === 'must-play');
    if (must) plays = plays.concat(must.ids.map(id => S.byId.get(id)).filter(p => p && !p.removed && !plays.includes(p)));
  }
  plays = plays.slice(0, 12);
  if (!plays.length) return '';
  return `<section class="featured">
    <div class="section-title"><div><div class="section-kicker">${t('featured_kicker')}</div><h2>${t('featured')}</h2></div></div>
    <div class="rail" id="rail">${plays.map(p => `
      <article class="feature-card" data-action="open-play" data-slug="${esc(p.slug)}">
        <div class="feature-media"><span class="badge">${t('featured_badge')}</span>${p.thumb || p.cover ? `<img src="${media(p.thumb || p.cover)}" alt="" loading="lazy">` : ''}</div>
        <div class="feature-body">
          <h3>${esc(pick(p.title))}</h3>
          <div class="meta">${esc(p.author.name)} · ${esc(categoryName(p.category))}</div>
          <p>${esc(pick(p.description))}</p>
          <div class="row">${installButton(p)}</div>
        </div>
      </article>`).join('')}</div>
    <div class="rail-nav"><button class="round" data-action="rail" data-dir="-1" aria-label="prev">${ICON.left}</button><button class="round" data-action="rail" data-dir="1" aria-label="next">${ICON.right}</button></div>
  </section>`;
}

function installButton(p, large) {
  const size = large ? 'large' : 'small';
  if (!p.ready && !p.downloadable) return `<button class="btn ${size} install-btn" data-install="${esc(p.slug)}" disabled>${t('no_fw_btn')}</button>`;
  const cls = p.ready ? 'btn soft' : 'btn';
  const tip = p.ready ? '' : ` title="${esc(t('not_ready'))}"`;
  return `<button class="${cls} ${size} install-btn" data-install="${esc(p.slug)}" data-action="flash" data-slug="${esc(p.slug)}"${tip}>${large ? t('flash_device') : t('install')} ${ICON.download}</button>`;
}

// Firmware finished in the background: refresh only the affected buttons.
async function refreshReady() {
  try {
    const r = await api('ready');
    const changed = [];
    for (const slug of r.slugs) {
      const p = S.bySlug.get(slug);
      if (p && !p.ready) { p.ready = true; changed.push(p); }
    }
    for (const p of changed) {
      document.querySelectorAll(`[data-install="${CSS.escape(p.slug)}"]`).forEach(btn => {
        btn.outerHTML = installButton(p, btn.classList.contains('large'));
      });
    }
  } catch { /* ignore */ }
}

function renderCard(p) {
  const upd = updatedLabel(p);
  return `<article class="card" data-action="open-play" data-slug="${esc(p.slug)}">
    <div class="thumb">${p.thumb || p.cover ? `<img src="${media(p.thumb || p.cover)}" alt="" loading="lazy">` : ''}</div>
    <div class="card-top">${p.removed ? `<span class="chip red">${t('removed')}</span>` : `<span class="chip">${esc(categoryName(p.category))}</span>`}${upd ? `<span class="updated">↻ ${upd}</span>` : ''}</div>
    <div class="author">${t('developer')} <b>${esc(p.author.name)}</b></div>
    <h3>${esc(pick(p.title))}</h3>
    <p>${esc(pick(p.description))}</p>
    <div class="card-foot">
      <div class="stats"><span>${ICON.download}${fmtNum(p.downloads)}</span><span>${ICON.heart}${fmtNum(p.hearts)}</span><span>${ICON.bookmark}${fmtNum(p.favorites)}</span></div>
      ${installButton(p)}
    </div>
  </article>`;
}

function renderGrid(list) {
  if (!list.length) return `<div class="empty"><h3>${t('empty_title')}</h3><p>${t('empty_sub')}</p></div>`;
  return `<div class="grid" id="grid">${list.slice(0, S.limit).map(renderCard).join('')}</div>${list.length > S.limit ? '<div class="sentinel" id="sentinel"></div>' : ''}`;
}

let observer = null;
function setupSentinel() {
  if (observer) observer.disconnect();
  const el = $('#sentinel');
  if (!el) return;
  observer = new IntersectionObserver(entries => {
    if (entries.some(e => e.isIntersecting)) {
      S.limit += 60;
      $('#grid-wrap').innerHTML = renderGrid(filtered());
      setupSentinel();
    }
  }, { rootMargin: '600px' });
  observer.observe(el);
}

function refreshGrid() {
  S.limit = 60;
  const wrap = $('#grid-wrap');
  if (!wrap) return render();
  const list = filtered();
  wrap.innerHTML = renderGrid(list);
  const line = $('.result-line');
  if (line) line.textContent = t('results', list.length);
  setupSentinel();
}

/* ---------------- markdown-ish text ---------------- */
function prose(text) {
  if (!text || !text.trim()) return `<p class="muted">${t('nothing_here')}</p>`;
  const lines = esc(text.replace(/\r\n?/g, '\n')).split('\n');
  let html = '';
  let list = null;
  let para = [];
  const inline = s => s.replace(/\*\*(.+?)\*\*/g, '<b>$1</b>').replace(/`([^`]+)`/g, '<code>$1</code>');
  const flushPara = () => { if (para.length) { html += `<p>${para.map(inline).join('<br>')}</p>`; para = []; } };
  const flushList = () => { if (list) { html += `<${list.tag}>${list.items.map(i => `<li>${inline(i)}</li>`).join('')}</${list.tag}>`; list = null; } };
  for (const raw of lines) {
    const line = raw.trim();
    let m;
    if (!line) { flushPara(); flushList(); continue; }
    if ((m = line.match(/^#{1,6}\s+(.*)$/)) || (m = line.match(/^【(.+)】$/))) { flushPara(); flushList(); html += `<h4>${inline(m[1])}</h4>`; continue; }
    if ((m = line.match(/^(?:[-*•·]|&#39;)\s+(.*)$/))) { flushPara(); if (!list || list.tag !== 'ul') { flushList(); list = { tag: 'ul', items: [] }; } list.items.push(m[1]); continue; }
    if ((m = line.match(/^\d+[.)、]\s*(.*)$/))) { flushPara(); if (!list || list.tag !== 'ol') { flushList(); list = { tag: 'ol', items: [] }; } list.items.push(m[1]); continue; }
    flushList();
    para.push(line);
  }
  flushPara(); flushList();
  return html;
}

/* ---------------- detail modal ---------------- */
function renderDetail() {
  const p = S.bySlug.get(S.detail.slug);
  if (!p) return '';
  const images = p.gallery.length ? p.gallery : [p.cover || p.thumb].filter(Boolean);
  const idx = Math.min(S.detail.img, Math.max(0, images.length - 1));
  const tabBody = { intro: pick(p.description), usage: pick(p.instructions), changelog: pick(p.changelog) }[S.detail.tab];
  const reveal = S.server.platform === 'windows' ? t('reveal_win') : t('open_folder');
  return `<div class="overlay" data-action="overlay"><div class="modal" role="dialog" aria-modal="true">
    <div class="modal-head"><h2>${t('detail_title')}</h2><button class="close" data-action="close-detail" aria-label="close">${ICON.close}</button></div>
    <div class="modal-body"><div class="detail">
      <div>
        <div class="detail-author">${p.author.avatar ? `<img class="avatar" src="${media(p.author.avatar)}" alt="">` : '<span class="avatar"></span>'}${esc(p.author.name)}</div>
        <div class="gallery-main">
          ${images.length ? `<img src="${media(images[idx])}" alt="">` : ''}
          ${images.length > 1 ? `<button class="round prev" data-action="img" data-i="${(idx - 1 + images.length) % images.length}">${ICON.left}</button><button class="round next" data-action="img" data-i="${(idx + 1) % images.length}">${ICON.right}</button><span class="gallery-count">${idx + 1} / ${images.length}</span>` : ''}
        </div>
        ${images.length > 1 ? `<div class="gallery-thumbs">${images.map((src, i) => `<button class="${i === idx ? 'active' : ''}" data-action="img" data-i="${i}"><img src="${media(src)}" alt="" loading="lazy"></button>`).join('')}</div>` : ''}
      </div>
      <div>
        <h1>${esc(pick(p.title))}</h1>
        <div class="install-box">
          <div class="install-row">${installButton(p, true)}
            <button class="btn ghost small" data-action="reveal-play" data-slug="${esc(p.slug)}">${reveal}</button>
            <button class="btn ghost small" data-action="open-web" data-slug="${esc(p.slug)}">${t('open_web')} ↗</button>
          </div>
          <p class="hint">${p.removed ? t('removed_note') : p.ready ? t('flash_hint') : t('no_firmware')}</p>
          <hr>
          <div class="stats"><span>${ICON.heart}${fmtNum(p.hearts)}</span><span>${ICON.bookmark}${fmtNum(p.favorites)}</span><span>${ICON.chat}${fmtNum(p.comments)}</span><span>${ICON.download}${fmtNum(p.downloads)}</span></div>
          <div class="sp8"></div>
          <div class="fw-meta">${t('firmware')} · ${fmtBytes(p.firmware.size)} · SHA-256 ${esc((p.firmware.sha256 || '').slice(0, 16))}… · ${esc(categoryName(p.category))}</div>
        </div>
        <div class="tabs detail-tabs">${['intro', 'usage', 'changelog'].map(k => `<button class="${S.detail.tab === k ? 'active' : ''}" data-action="detail-tab" data-tab="${k}">${t('tab_' + k)}</button>`).join('')}</div>
        <div class="prose">${prose(tabBody)}</div>
      </div>
    </div></div>
  </div></div>`;
}

/* ---------------- sync ---------------- */
function syncBody() {
  const sy = S.server.sync;
  const title = sy.running ? (sy.kind === 'move' ? t('move_title') : t('sync_running')) : sy.phase === 'done' ? t('sync_done') : sy.phase === 'failed' ? t('sync_failed') : sy.phase === 'cancelled' ? t('sync_cancelled') : t('sync_title');
  const pct = sy.kind === 'move' && sy.bytesTotal ? Math.round((sy.bytesDone / sy.bytesTotal) * 100) : sy.total ? Math.round((sy.done / sy.total) * 100) : 0;
  const phase = sy.running ? t('phase_' + sy.phase) : sy.phase === 'done' ? [t('added', sy.added), t('updated', sy.updated), sy.removed ? t('removed_n', sy.removed) : ''].filter(Boolean).join(' · ') : '';
  const wait = sy.running ? countdown(sy.throttledUntil) : '';
  const showCounts = sy.kind === 'move' || ['catalog', 'firmware', 'done', 'cancelled', 'failed'].includes(sy.phase);
  const failures = (sy.failures || []).map(f => `<div><b>${esc(pick(f.title) || f.slug)}</b><span title="${esc(f.error)}">${esc(f.error)}</span></div>`).join('');
  let buttons;
  if (sy.running) buttons = `${S.modal === 'sync' ? `<button class="btn ghost" data-action="close-modal">${t('run_in_bg')}</button>` : ''}<button class="btn danger" data-action="cancel-sync">${t('stop')}</button>`;
  else if (sy.failed > 0 || sy.phase === 'cancelled' || sy.phase === 'failed') buttons = `<button class="btn" data-action="close-modal">${t('close')}</button><button class="btn primary" data-action="start-sync">${sy.failed > 0 ? t('retry_failed') : t('continue_update')}</button>`;
  else buttons = `<button class="btn primary" data-action="close-modal">${t('done')}</button>`;
  return `<h2>${title}</h2><p class="phase">${esc(phase)}</p>
    <div class="big-progress"><i data-w="${pct}"></i></div>
    <div class="progress-meta"><span>${sy.kind === 'move' ? fmtBytes(sy.bytesDone) + ' / ' + fmtBytes(sy.bytesTotal) : sy.done + ' / ' + (sy.total || '—')}</span><span>${pct}%</span></div>
    ${wait ? `<div class="notice">${t('throttled', wait)}</div>` : ''}
    ${sy.running && sy.phase === 'firmware' ? `<p class="m-note">${t('browse_while')}</p>` : ''}
    <div class="counts" ${showCounts ? '' : 'hidden'}>
      <div><b>${sy.phase === 'firmware' || !sy.running ? sy.firmwareDone + '/' + sy.firmwareTotal : sy.done}</b><span>${sy.phase === 'firmware' || !sy.running ? t('firmware') : t('done_count')}</span></div>
      <div><b>${sy.failed}</b><span>${t('failed_count')}</span></div>
      <div><b>${fmtBytes(sy.bytesDone)}</b><span>${t('downloaded')}</span></div>
      <div><b>${sy.running ? fmtBytes(sy.speed) + '/s' : '—'}</b><span>${t('speed')}</span></div>
    </div>
    <div class="current">${esc((sy.current || []).join(' · '))}</div>
    ${sy.phase === 'failed' ? `<div class="notice red">${esc(errText(sy.errorCode, sy.error))}</div>` : ''}
    ${failures ? `<div class="failures">${failures}</div>` : ''}
    <div class="modal-foot inline">${buttons}</div>`;
}

/* ---------------- flash ---------------- */
function fetchBody(p) {
  const fe = S.server.fetch || {};
  const err = S.flash.fetchError || (fe.slug === p.slug && fe.errorCode ? { code: fe.errorCode, message: fe.error } : null);
  let inner;
  let foot = `<button class="btn" data-action="close-modal">${t('cancel')}</button>`;
  if (err) {
    const wait = err.code === 'rate_limited' ? countdown(fe.throttledUntil) : '';
    inner = `<div class="notice red">${wait ? t('fetch_wait', wait) : esc(errText(err.code, err.message))}</div>`;
    foot += `<button class="btn primary" data-action="fetch-retry">${t('retry')}</button>`;
  } else {
    inner = `<div class="searching"><span class="spinner"></span>${S.server.sync.running ? t('fetch_queue') : t('fetch_title')}</div>`;
  }
  return `<div class="steps"><span class="on"></span><span></span><span></span></div>
    <p class="m-title"><b>${esc(pick(p.title))}</b></p>
    <p class="m-sub">${t('firmware')} · ${fmtBytes(p.firmware.size)}</p>
    <div class="ports">${inner}</div>
    <div class="modal-foot inline">${foot}</div>`;
}

function requestFetch(slug) {
  S.flash.fetchError = null;
  api('firmware/fetch', { slug }).then(() => poll()).catch(e => { S.flash.fetchError = e; updateModal(); });
}

function flashBody() {
  const f = S.server.flash;
  const p = S.bySlug.get(S.flash.slug);
  const title = p ? esc(pick(p.title)) : '';
  if (p && !p.ready && (f.stage === 'idle' || f.slug !== S.flash.slug)) return fetchBody(p);
  if (f.stage === 'idle' || f.slug !== S.flash.slug) {
    const ports = S.flash.ports;
    let list;
    if (ports === null) list = `<div class="searching"><span class="spinner"></span>${t('searching')}</div>`;
    else if (!ports.length) list = `<div class="searching">${t('no_ports')}</div>`;
    else list = (ports.some(pt => pt.recommended) ? '' : `<div class="notice">${t('no_passport')}</div>`) + ports.map(pt => `<label class="port ${S.flash.port === pt.path ? 'selected' : ''}"><input type="radio" name="port" value="${esc(pt.path)}" ${S.flash.port === pt.path ? 'checked' : ''} data-action="pick-port"><span><b>${esc(pt.recommended ? t('port_recommended') : pt.label)}</b><small>${esc(pt.path)} · ${pt.vid.toString(16).padStart(4, '0')}:${pt.pid.toString(16).padStart(4, '0')}</small></span>${pt.recommended ? `<span class="chip">ESP32-C3</span>` : '<span></span>'}</label>`).join('');
    return `<div class="steps"><span class="on"></span><span></span><span></span></div>
      <p class="m-title"><b>${title}</b></p>
      <p class="m-sub">${t('step_connect')}</p>
      <div class="ports">${list}</div>
      <div class="notice">${t('flash_warn')}</div>
      <div class="modal-foot inline"><button class="btn ghost" data-action="scan-ports">${t('refresh')}</button><button class="btn" data-action="close-modal">${t('cancel')}</button><button class="btn primary" data-action="start-flash" ${S.flash.port ? '' : 'disabled'}>${t('start_flash')}</button></div>`;
  }
  const logs = f.logs && f.logs.length ? `<details class="log"><summary>${t('log')}</summary><pre>${esc(f.logs.join('\n'))}</pre></details>` : '';
  const info = f.chip ? `<dl class="kv"><dt>${t('chip')}</dt><dd>${esc(f.chip)}</dd><dt>${t('mac')}</dt><dd>${esc(f.mac)}</dd><dt>${t('flash_size')}</dt><dd>${esc(f.flash)}</dd></dl>` : '';
  if (f.stage === 'done') {
    return `<div class="steps"><span class="on"></span><span class="on"></span><span class="on"></span></div>
      <div class="result"><div class="icon">✓</div><h3>${t('flash_done')}</h3><p>${title} · ${t(f.restarted ? 'flash_done_sub' : 'flash_done_replug')}</p></div>${f.restarted ? '' : `<div class="notice">${t('flash_done_replug')}</div>`}${info}${cardidNote(f)}${logs}
      <div class="modal-foot inline"><button class="btn primary" data-action="close-modal">${t('done')}</button></div>`;
  }
  if (f.stage === 'failed') {
    return `<div class="steps"><span class="on"></span><span class="on"></span><span></span></div>
      <div class="result fail"><div class="icon">!</div><h3>${t('flash_failed')}</h3><p>${esc(errText(f.errorCode, f.error))}</p></div>${info}${cardidNote(f)}${logs}
      <p class="center-note"><a href="#/restore" data-action="go-restore">${t('restore_link')}</a></p>
      <div class="modal-foot inline"><button class="btn" data-action="close-modal">${t('close')}</button><button class="btn primary" data-action="flash-retry">${t('retry')}</button></div>`;
  }
  const pct = f.stage === 'writing' || f.stage === 'verifying' ? f.percent : f.stage === 'resetting' ? 100 : 0;
  return `<div class="steps"><span class="on"></span><span class="on"></span><span></span></div>
    <p class="m-title"><b>${title}</b></p>
    <p class="m-sub">${t('stage_' + f.stage)}…</p>
    <div class="big-progress"><i data-w="${pct}"></i></div>
    <div class="progress-meta"><span>${esc(f.port || '')}</span><span>${pct}%</span></div>
    ${info}<div class="sp12"></div><div class="notice">${t('flash_warn')}</div>${logs}`;
}

function cardidNote(f) {
  if (!f.cardidOverwritten) return '';
  return `<div class="sp12"></div><div class="notice">${t('cardid_overwritten')} <button class="link" data-action="reveal-backup">${t('open_backup')}</button></div>`;
}

/* ---------------- factory restore ---------------- */
const RESTORE_STAGES = ['preparing', 'waiting', 'connecting', 'checking', 'backup', 'writing', 'verifying', 'cardid', 'resetting'];

function restoreState() {
  const f = S.server.flash;
  return f && f.kind === 'restore' ? f : { stage: 'idle', logs: [] };
}

function renderRestore() {
  const st = S.server;
  const r = st.restore || {};
  const busy = st.flash.active;
  return `<div class="settings restore">
    <h1>${t('tools_title')}</h1>
    <p class="lead">${t('tools_lead')}</p>
    
    <section class="panel"><h2>${t('backup_title')}</h2><p>${t('backup_lead')}</p><div id="backup-main">${backupMain()}</div></section>
    <section class="panel"><h2>${t('restore_title')}</h2><p>${t('restore_lead')}</p><div id="restore-main">${restoreMain()}</div></section>
    <section class="panel">
      <h2>${t('r_options')}</h2><p>${t('r_safety')}</p>
      <dl class="kv"><dt>${t('r_backup_dir')}</dt><dd class="ellipsis" title="${esc(r.backupDir || '')}">${esc(r.backupDir || '—')}</dd><dt>${t('r_fw')}</dt><dd>${esc(r.firmware || '')}</dd></dl>
      ${st.native ? '' : `<div class="folder mt10">${ICON.folder}<input id="backup-input" value="${esc(r.backupDir || '')}" spellcheck="false"></div>`}
      <label class="check mt10"><input type="checkbox" id="restore-full" ${S.restore.full ? 'checked' : ''} ${busy ? 'disabled' : ''}> ${t('r_full')}</label>
      <div class="buttons">
        <button class="btn" data-action="choose-backup" ${busy ? 'disabled' : ''}>${t('change_dir')}</button>
        ${st.native ? `<button class="btn ghost" data-action="reveal-backup">${t('open_backup')}</button>` : ''}
      </div>
    </section>
  </div>`;
}

function restoreMain() {
  const st = S.server;
  const f = restoreState();
  const logs = logBlock(f);
  const info = deviceInfo(f);
  const steps = `<ol class="restore-steps"><li><b>1</b>${t('restore_step1')}</li><li><b>2</b>${t('restore_step2')}</li></ol>`;
  if (st.flash.active && st.flash.kind !== 'restore') {
    return `${steps}<div class="notice">${busyText(st.flash.kind)}</div>`;
  }
  if (f.stage === 'idle' || f.stage === 'cancelled') {
    const head = f.stage === 'cancelled' ? `<h3>${t('r_cancelled')}</h3><p class="m-sub">${t('r_cancelled_sub')}</p>` : `<h3>${t('r_idle')}</h3><p class="m-sub">${t('r_idle_sub')}</p>`;
    const action = S.restore.confirm
      ? `<div class="notice">${t('restore_confirm')}</div><div class="buttons"><button class="btn ghost" data-action="restore-unconfirm">${t('cancel')}</button><button class="btn primary large" data-action="restore-go" ${st.worker ? '' : 'disabled'}>${t('restore_go')}</button></div>`
      : `<div class="buttons"><button class="btn primary large wide" data-action="restore-confirm" ${st.worker ? '' : 'disabled'}>${t('restore_start')}</button></div>`;
    return `${steps}<div class="restore-status">${head}</div>${st.worker ? '' : `<div class="notice red">${t('err_worker')}</div>`}${action}${logs}`;
  }
  if (f.stage === 'done') {
    let id = '';
    if (f.cardid === 'blank_ff' || f.cardid === 'blank_00') id = `<div class="notice">${t('r_blank_id')}</div>`;
    else if (f.cardid && f.cardid !== 'keys') id = `<div class="notice">${t('r_unknown_id')}</div>`;
    return `<div class="result"><div class="icon">✓</div><h3>${t('r_done')}</h3><p>${t(f.restarted ? 'r_done_sub' : 'r_done_replug')}</p></div>${f.restarted ? '' : `<div class="notice">${t('r_done_replug')}</div>`}${info}<div class="sp12"></div>${id}
      <div class="buttons"><button class="btn" data-action="reveal-session">${t('open_backup')}</button><button class="btn ghost" data-action="restore-reset">${t('restore_again')}</button></div>${logs}`;
  }
  if (f.stage === 'failed') {
    const serious = f.errorCode === 'cardid_changed';
    return `<div class="result fail"><div class="icon">!</div><h3>${t('r_failed')}</h3><p>${esc(errText(f.errorCode, f.error))}</p></div>${info}
      ${serious ? '' : `<p class="center-note">${t('r_failed_sub')}</p>`}
      <div class="buttons center">${f.session || serious ? `<button class="btn" data-action="reveal-backup">${t('open_backup')}</button>` : ''}${serious ? '' : `<button class="btn primary" data-action="restore-retry">${t('retry')}</button>`}</div>${logs}`;
  }
  return runningBody(f, 'restore', steps);
}

const busyText = kind => t(kind === 'backup' ? 'r_busy_backup' : kind === 'restore' ? 'r_busy_restore' : 'r_busy_flash');

function logBlock(f) {
  return f.logs && f.logs.length ? `<details class="log"${S.restore.logOpen ? ' open' : ''}><summary>${t('log')}</summary><pre>${esc(f.logs.join('\n'))}</pre></details>` : '';
}

function deviceInfo(f) {
  return f.mac ? `<dl class="kv"><dt>${t('chip')}</dt><dd>${esc(f.chip)}</dd><dt>${t('mac')}</dt><dd>${esc(f.mac)}</dd><dt>${t('flash_size')}</dt><dd>${esc(f.flash)}</dd></dl>` : '';
}

// Progress view shared by the factory restore and the firmware backup.
function runningBody(f, kind, steps) {
  const label = k => (kind === 'backup' && I18N.zh['b_' + k] ? t('b_' + k) : t('r_' + k));
  let key = f.stage === 'checking' ? 'connecting' : f.stage === 'cardid' ? 'verifying' : f.stage;
  if (!I18N.zh['r_' + key]) key = 'preparing';
  let sub = label(key + '_sub');
  if (f.stage === 'waiting') sub = f.hint === 'multiple' ? t('r_multiple') : f.hint === 'code43' ? t('r_code43', f.port || 'COM') : f.hint === 'not_seen' ? t('r_not_seen') : t('r_replug');
  const waited = f.stage === 'waiting' && f.startedAt ? `<p class="m-sub">${t('r_waited', Math.max(0, Math.round((Date.now() - Date.parse(f.startedAt)) / 1000)), 150)}</p>` : '';
  const idx = Math.max(0, RESTORE_STAGES.indexOf(f.stage));
  const measured = f.stage === 'backup' || (kind === 'restore' && ['writing', 'verifying'].includes(f.stage));
  const bar = f.stage === 'waiting' || (kind === 'backup' && f.stage === 'verifying')
    ? `<div class="big-progress indeterminate"><i></i></div>`
    : `<div class="big-progress"><i data-w="${measured ? f.percent : Math.round((idx / RESTORE_STAGES.length) * 100)}"></i></div>`;
  const writing = kind === 'restore' && ['writing', 'verifying', 'cardid', 'resetting'].includes(f.stage);
  const stop = writing ? '' : `<button class="btn ghost" data-action="restore-cancel" ${f.cancelling ? 'disabled' : ''}>${f.cancelling ? t('restore_stopping') : t('restore_stop')}</button>`;
  return `${steps}<div class="restore-status"><h3><span class="spinner"></span>${label(key)}</h3>${f.hint === 'not_seen' ? `<div class="notice red">${esc(sub)}</div>` : `<p class="m-sub">${esc(sub)}</p>`}${waited}</div>
    ${bar}<div class="progress-meta"><span>${esc(f.port || '')}</span><span>${measured ? f.percent + '%' : ''}</span></div>
    ${deviceInfo(f)}<div class="sp12"></div><div class="notice">${t(kind === 'backup' ? 'b_warn' : 'r_warn')}</div><div class="buttons">${stop}</div>${logBlock(f)}`;
}

function backupMain() {
  const st = S.server;
  const f = st.flash && st.flash.kind === 'backup' ? st.flash : { stage: 'idle', logs: [] };
  if (st.flash.active && st.flash.kind !== 'backup') return `<div class="notice">${busyText(st.flash.kind)}</div>`;
  if (f.stage === 'idle' || f.stage === 'cancelled') {
    const head = f.stage === 'cancelled' ? `<p class="m-sub">${t('r_cancelled')}</p>` : `<p class="m-sub">${t('b_idle_sub')}</p>`;
    return `${head}${st.worker ? '' : `<div class="notice red mt10">${t('err_worker')}</div>`}
      <div class="buttons"><button class="btn soft large wide" data-action="backup-go" ${st.worker ? '' : 'disabled'}>${t('backup_start')}</button></div>${logBlock(f)}`;
  }
  if (f.stage === 'done') {
    return `<div class="result"><div class="icon">✓</div><h3>${t('b_done')}</h3><p>${t('b_done_sub')}</p></div>${f.restarted ? '' : `<div class="notice">${t('r_done_replug')}</div>`}${deviceInfo(f)}
      <dl class="kv"><dt>${t('b_saved_to')}</dt><dd class="ellipsis" title="${esc(f.file || '')}">${esc(f.file || f.session || '')}</dd></dl>
      <div class="buttons"><button class="btn" data-action="reveal-session">${t('open_backup')}</button><button class="btn ghost" data-action="backup-again">${t('backup_again')}</button></div>${logBlock(f)}`;
  }
  if (f.stage === 'failed') {
    return `<div class="result fail"><div class="icon">!</div><h3>${t('b_failed')}</h3><p>${esc(errText(f.errorCode, f.error))}</p></div>${deviceInfo(f)}
      <div class="buttons center"><button class="btn primary" data-action="backup-go">${t('retry')}</button></div>${logBlock(f)}`;
  }
  return runningBody(f, 'backup', '');
}

function updateRestore() {
  for (const [id, body] of [['#backup-main', backupMain], ['#restore-main', restoreMain]]) {
    const el = $(id);
    if (!el) continue;
    const pre = el.querySelector('details.log pre');
    const atBottom = pre ? pre.scrollTop + pre.clientHeight >= pre.scrollHeight - 8 : true;
    const top = pre ? pre.scrollTop : 0;
    el.innerHTML = body();
    applyWidths(el);
    const np = el.querySelector('details.log pre');
    if (np) np.scrollTop = atBottom ? np.scrollHeight : top;
  }
  const full = $('#restore-full');
  if (full) full.disabled = S.server.flash.active;
}

async function startRestore() {
  try {
    const full = $('#restore-full');
    if (full) S.restore.full = full.checked;
    S.restore.confirm = false;
    await api('restore/start', { fullBackup: S.restore.full });
    await poll(true);
  } catch (e) { showError(e); }
}

async function scanPorts() {
  if (S.flash.scanning) return;
  S.flash.scanning = true;
  try {
    const ports = await api('ports');
    S.flash.ports = ports;
    if (!ports.some(p => p.path === S.flash.port)) {
      const rec = ports.find(p => p.recommended);
      S.flash.port = rec ? rec.path : ports.length === 1 ? ports[0].path : '';
    }
  } catch (e) {
    S.flash.ports = [];
    if (e.code === 'worker') toast(t('err_worker'), true);
  } finally {
    S.flash.scanning = false;
  }
  if (S.modal === 'flash') updateModal();
}

function startPortScan() {
  stopPortScan();
  S.flash.ports = null;
  scanPorts();
  S.flash.scanTimer = setInterval(() => {
    const f = S.server.flash;
    if (S.modal === 'flash' && (f.stage === 'idle' || f.slug !== S.flash.slug)) scanPorts();
  }, 2500);
}
function stopPortScan() { clearInterval(S.flash.scanTimer); S.flash.scanTimer = null; }

/* ---------------- move dialog ---------------- */
function moveBody() {
  return `<p class="m-title"><b>${t('move_q')}</b></p>
    <div class="folder">${ICON.folder}<code>${esc(S.pendingDir)}</code></div>
    <p class="m-note">${t('move_no_sub')}</p>
    <div class="modal-foot inline"><button class="btn ghost" data-action="close-modal">${t('cancel')}</button><button class="btn" data-action="set-dir" data-mode="switch">${t('move_no')}</button><button class="btn primary" data-action="set-dir" data-mode="move">${t('move_yes')}</button></div>`;
}

/* ---------------- modal management ---------------- */
function renderModal() {
  const root = $('#modal-root');
  if (S.detail.slug && !S.modal) {
    root.innerHTML = renderDetail();
    applyWidths(root);
    return;
  }
  if (!S.modal) { root.innerHTML = ''; return; }
  const titles = { sync: S.server.sync.kind === 'move' ? t('move_title') : t('sync_title'), flash: t('flash_title'), move: t('move_title') };
  root.innerHTML = `<div class="overlay"><div class="modal narrow" role="dialog" aria-modal="true">
    <div class="modal-head"><h2>${titles[S.modal]}</h2>${canClose() ? `<button class="close" data-action="close-modal" aria-label="close">${ICON.close}</button>` : ''}</div>
    <div class="modal-body sync-card" id="modal-body"></div></div></div>`;
  updateModal();
}

function canClose() {
  if (S.modal === 'flash') return !S.server.flash.active;
  return true;
}

function updateModal() {
  const body = $('#modal-body');
  if (!body) return;
  const open = body.querySelector('details.log');
  const wasOpen = open && open.open;
  const pre = body.querySelector('details.log pre');
  const atBottom = pre ? pre.scrollTop + pre.clientHeight >= pre.scrollHeight - 8 : true;
  body.innerHTML = S.modal === 'sync' ? syncBody() : S.modal === 'flash' ? flashBody() : moveBody();
  applyWidths(body);
  const nd = body.querySelector('details.log');
  if (nd && wasOpen) {
    nd.open = true;
    const np = nd.querySelector('pre');
    if (atBottom) np.scrollTop = np.scrollHeight;
  }
  const head = $('#modal-root .modal-head');
  if (head) {
    const hasClose = !!head.querySelector('.close');
    if (hasClose !== canClose()) renderModal();
  }
}

function closeModal() {
  if (S.modal === 'flash') {
    if (S.server.flash.active) return;
    stopPortScan();
    api('flash/reset', {}).catch(() => {});
  }
  S.modal = null;
  renderModal();
}

/* ---------------- settings ---------------- */
function renderSettings() {
  const st = S.server;
  const lib = st.library;
  const reveal = st.platform === 'windows' ? t('reveal_win') : t('reveal');
  const up = st.update;
  let updateLine = '';
  if (S.checking || up.checking) updateLine = t('checking');
  else if (up.checkedAt && up.checkedAt[0] !== '0') updateLine = up.error ? errText('network') : up.available ? t('banner_update', up.changes) : t('up_to_date');
  return `<div class="settings">
    <h1>${t('settings_title')}</h1>
    <section class="panel">
      <h2>${t('s_library')}</h2><p>${t('s_library_sub')}</p>
      ${st.native
        ? `<div class="folder">${ICON.folder}<code title="${esc(lib.dir)}">${esc(lib.dir || '—')}</code></div>`
        : `<div class="folder">${ICON.folder}<input id="dir-input" value="${esc(lib.dir)}" placeholder="${esc(t('path_ph'))}" spellcheck="false"></div>`}
      <dl class="kv" id="usage"><dt>${t('usage')}</dt><dd>…</dd></dl>
      <div class="buttons">
        <button class="btn" data-action="choose-dir" ${st.sync.running || st.flash.active ? 'disabled' : ''}>${t('change_dir')}</button>
        ${st.native ? `<button class="btn ghost" data-action="reveal-library" ${lib.dir ? '' : 'disabled'}>${reveal}</button>` : ''}
      </div>
    </section>
    <section class="panel">
      <h2>${t('s_data')}</h2><p>${t('s_data_sub')}</p>
      <dl class="kv">
        <dt>${t('last_sync')}</dt><dd>${lib.hasIndex ? fmtTime(lib.syncedAt) : t('never')}</dd>
        <dt>${t('plays_ready')}</dt><dd>${lib.hasIndex ? `${lib.ready} / ${lib.plays}` : '—'}</dd>
        <dt>${t('status')}</dt><dd>${lib.hasIndex ? (lib.complete ? t('complete') : t('incomplete')) : '—'}</dd>
      </dl>
      <div class="buttons">
        <button class="btn" data-action="check-update" ${st.sync.running || !lib.hasIndex ? 'disabled' : ''}>${t('check_update')}</button>
        <button class="btn primary" data-action="start-sync" ${st.sync.running || !lib.dir ? 'disabled' : ''}>${t('update_now')}</button>
        <span class="inline-note">${esc(updateLine)}</span>
      </div>
    </section>
    <section class="panel">
      <h2>${t('s_language')}</h2>
      <div class="segmented mt10">${[['auto', t('lang_auto')], ['zh', '中文'], ['en', 'English']].map(([k, label]) => `<button class="${st.language === k ? 'active' : ''}" data-action="set-lang" data-lang="${k}">${label}</button>`).join('')}</div>
    </section>
    <section class="panel">
      <h2>${t('s_about')}</h2>
      <dl class="kv">
        <dt>${t('version')}</dt><dd>${esc(st.version)}</dd>
        <dt>${t('worker')}</dt><dd>${st.worker ? t('worker_ok') : t('worker_missing')}</dd>
        <dt>${t('source')}</dt><dd>ai-passport.folotoy.cn</dd>
      </dl>
    </section>
  </div>`;
}

async function refreshUsage() {
  const el = $('#usage');
  if (!el || !S.server.library.dir) { if (el) el.innerHTML = `<dt>${t('usage')}</dt><dd>—</dd>`; return; }
  try {
    const u = await api('library/usage');
    const cur = $('#usage');
    if (cur) cur.innerHTML = `<dt>${t('usage')}</dt><dd>${fmtBytes(u.bytes)}</dd><dt>${t('files')}</dt><dd>${u.files}</dd><dt>${t('free')}</dt><dd>${fmtBytes(u.free)}</dd>`;
  } catch { /* ignore */ }
}

/* ---------------- actions ---------------- */
async function chooseDir() {
  if (S.server.native) {
    const r = await api('library/choose', {});
    return r && r.dir ? r.dir : '';
  }
  const input = $('#dir-input');
  return input ? input.value.trim() : '';
}

async function startSync() {
  try {
    await api('sync/start', {});
    S.modal = S.server.library.hasIndex ? 'sync' : null;
    await poll();
    renderModal();
  } catch (e) { showError(e); }
}

const actions = {
  'cat': el => { S.view.cat = el.dataset.cat; S.view.tab = 'all'; location.hash = libraryHash(); },
  'tab': el => { S.view.tab = el.dataset.tab; location.hash = libraryHash(); },
  'sort': el => { S.view.sort = el.dataset.sort; location.hash = libraryHash(); },
  'open-play': el => { location.hash = '#/play/' + encodeURIComponent(el.dataset.slug); },
  'close-detail': () => closeDetail(),
  'overlay': (el, ev) => { if (ev.target === el) closeDetail(); },
  'img': el => { S.detail.img = Number(el.dataset.i); renderModal(); },
  'detail-tab': el => { S.detail.tab = el.dataset.tab; renderModal(); },
  'rail': el => { const r = $('#rail'); if (r) r.scrollBy({ left: Number(el.dataset.dir) * r.clientWidth, behavior: 'smooth' }); },
  'toggle-lang': async () => { await setLanguage(lang === 'zh' ? 'en' : 'zh'); },
  'set-lang': async el => { await setLanguage(el.dataset.lang); },
  'open-sync': () => { S.modal = 'sync'; renderModal(); },
  'start-sync': () => startSync(),
  'cancel-sync': async () => { await api('sync/cancel', {}).catch(showError); poll(); },
  'close-modal': () => closeModal(),
  'welcome-start': async () => {
    try {
      const lib = S.server.library;
      const dir = S.server.native ? (S.pendingDir || lib.dir || lib.defaultDir) : ($('#dir-input') || {}).value;
      if (dir && dir !== lib.dir) await api('library/set', { dir, mode: 'switch' });
      await poll();
      if (S.server.library.hasIndex) { await loadCatalog(); render(); return; }
      await startSync();
      render();
    } catch (e) { showError(e); }
  },
  'choose-dir-welcome': async () => {
    try { const d = await chooseDir(); if (d) { S.pendingDir = d; render(); } } catch (e) { showError(e); }
  },
  'choose-dir': async () => {
    try {
      const d = await chooseDir();
      if (!d || d === S.server.library.dir) return;
      S.pendingDir = d;
      if (S.server.library.hasIndex) { S.modal = 'move'; renderModal(); return; }
      await api('library/set', { dir: d, mode: 'switch' });
      toast(t('dir_changed'));
      await poll(true);
    } catch (e) { showError(e); }
  },
  'set-dir': async el => {
    try {
      const mode = el.dataset.mode;
      await api('library/set', { dir: S.pendingDir, mode });
      S.modal = mode === 'move' ? 'sync' : null;
      if (mode !== 'move') toast(t('dir_changed'));
      await poll(true);
      renderModal();
    } catch (e) { S.modal = null; renderModal(); showError(e); }
  },
  'reveal-library': () => api('reveal', { target: 'library' }).catch(showError),
  'reveal-play': el => api('reveal', { target: 'play', slug: el.dataset.slug }).catch(showError),
  'open-web': el => api('open-web', { slug: el.dataset.slug }).catch(showError),
  'check-update': async () => {
    S.checking = true; render();
    try { await api('update/check', {}); await new Promise(r => setTimeout(r, 1200)); } catch (e) { showError(e); }
    S.checking = false; await poll(); render();
  },
  'flash': el => {
    if (S.server.flash.active && S.server.flash.kind !== 'flash') { toast(t('err_busy'), true); return; }
    S.flash.slug = el.dataset.slug;
    S.flash.port = '';
    S.modal = 'flash';
    api('flash/reset', {}).catch(() => {});
    S.server.flash = { stage: 'idle', logs: [] };
    const p = S.bySlug.get(S.flash.slug);
    renderModal();
    if (p && !p.ready) requestFetch(p.slug);
    else startPortScan();
  },
  'fetch-retry': () => { requestFetch(S.flash.slug); updateModal(); },
  'pick-port': el => { S.flash.port = el.value; updateModal(); },
  'scan-ports': () => { S.flash.ports = null; updateModal(); scanPorts(); },
  'start-flash': async () => {
    try {
      await api('flash/start', { slug: S.flash.slug, port: S.flash.port });
      await poll();
      renderModal();
    } catch (e) { showError(e); }
  },
  'go-restore': (el, ev) => { ev.preventDefault(); if (S.server.flash.active) return; closeModal(); location.hash = '#/restore'; },
  'restore-confirm': () => { S.restore.confirm = true; updateRestore(); },
  'restore-unconfirm': () => { S.restore.confirm = false; updateRestore(); },
  'restore-go': () => startRestore(),
  'restore-retry': async () => { await api('flash/reset', {}).catch(() => {}); await startRestore(); },
  'restore-reset': async () => { await api('flash/reset', {}).catch(() => {}); await poll(true); },
  'backup-go': async () => {
    try { await api('flash/reset', {}).catch(() => {}); await api('firmware-backup/start', {}); await poll(true); } catch (e) { showError(e); }
  },
  'backup-again': async () => { await api('flash/reset', {}).catch(() => {}); await poll(true); },
  'restore-cancel': async () => { try { await api('restore/cancel', {}); } catch (e) { showError(e); } poll(); },
  'reveal-backup': () => api('reveal', { target: 'backup' }).catch(showError),
  'reveal-session': () => api('reveal', { target: 'restore-session' }).catch(showError),
  'choose-backup': async () => {
    try {
      if (S.server.native) await api('backup/choose', {});
      else await api('backup/set', { dir: (($('#backup-input') || {}).value || '').trim() });
      toast(t('saved'));
      await poll(true);
    } catch (e) { showError(e); }
  },
  'flash-retry': async () => {
    await api('flash/reset', {}).catch(() => {});
    S.server.flash = { stage: 'idle', logs: [] };
    renderModal();
    scanPorts();
  },
};

function closeDetail() {
  location.hash = libraryHash();
}

async function setLanguage(l) {
  try {
    await api('settings', { language: l });
    S.server.language = l;
    render();
  } catch (e) { showError(e); }
}

document.addEventListener('click', ev => {
  const el = ev.target.closest('[data-action]');
  if (!el) return;
  const fn = actions[el.dataset.action];
  if (!fn) return;
  if (el.dataset.action === 'pick-port') return; // handled on change
  if (el.closest('.card, .feature-card') && el.dataset.action !== 'open-play') ev.stopPropagation();
  fn(el, ev);
});
document.addEventListener('change', ev => {
  if (ev.target.matches('[data-action="pick-port"]')) actions['pick-port'](ev.target);
  if (ev.target.id === 'restore-full') S.restore.full = ev.target.checked;
});
document.addEventListener('toggle', ev => {
  if (ev.target.matches && ev.target.matches('#restore-main details.log, #backup-main details.log')) S.restore.logOpen = ev.target.open;
}, true);
let searchTimer = null;
document.addEventListener('input', ev => {
  if (ev.target.id !== 'search') return;
  clearTimeout(searchTimer);
  searchTimer = setTimeout(() => {
    S.view.q = ev.target.value;
    history.replaceState(null, '', libraryHash());
    refreshGrid();
  }, 160);
});
document.addEventListener('keydown', ev => {
  if (ev.key !== 'Escape') return;
  if (S.modal && canClose()) closeModal();
  else if (S.detail.slug) closeDetail();
});
// CSP forbids inline onerror handlers; hide broken images here instead.
document.addEventListener('error', ev => { if (ev.target.tagName === 'IMG') ev.target.classList.add('broken'); }, true);
window.addEventListener('hashchange', applyRoute);

/* ---------------- polling ---------------- */
let pollTimer = null;
let lastSyncRunning = false;
async function poll(forceRender) {
  clearTimeout(pollTimer);
  let st;
  try {
    st = await api('state');
  } catch {
    pollTimer = setTimeout(poll, 3000);
    return;
  }
  const first = !S.server;
  const prev = S.server;
  S.server = st;
  const key = catalogKey(st);
  const catalogChanged = key !== S.catalogKey;
  if (catalogChanged) {
    S.catalogKey = key;
    await loadCatalog();
  }
  if (st.ready !== S.readyVersion) {
    S.readyVersion = st.ready;
    if (!catalogChanged) await refreshReady();
  }
  if (S.modal === 'flash') {
    const p = S.bySlug.get(S.flash.slug);
    if (p && st.fetch.slug === p.slug && st.fetch.done && !p.ready) { p.ready = true; refreshReady(); }
    if (p && p.ready && !S.flash.scanTimer && st.flash.stage === 'idle') startPortScan();
  }
  const syncFinished = lastSyncRunning && !st.sync.running;
  lastSyncRunning = st.sync.running;
  const r = parseRoute();
  const stillWelcome = !st.library.hasIndex && r.name === 'library';
  if (first || forceRender || catalogChanged || syncFinished || (prev && prev.language !== st.language)) {
    render();
  } else {
    renderTopRight();
    if (stillWelcome && st.sync.running) {
      const inline = $('#sync-inline');
      if (inline) { inline.innerHTML = syncBody(); applyWidths(inline); } else render();
    }
    if (S.modal === 'sync' || S.modal === 'flash') updateModal();
    if (r.name === 'restore') updateRestore();
  }
  const busy = st.sync.running || st.flash.active || S.modal === 'flash';
  pollTimer = setTimeout(poll, busy ? 700 : 4000);
}

applyRoute();
poll(true);
