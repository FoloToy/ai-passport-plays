# Third-party software

AI Passport 玩法社区 uses Wails v3 (MIT) for its desktop host. Dependency
versions and module checksums are pinned in go.mod and go.sum. Wails source:
https://github.com/wailsapp/wails/tree/v3.0.0-beta.24

The separately executed flashing worker imports esptool 5.6.0
(GPL-2.0-or-later). Its Python source (`worker/main.py`, `worker/restore.py`) is licensed
GPL-2.0-or-later and shipped inside the application, together with the exact
esptool source archive and third-party license files. Esptool source:
https://github.com/espressif/esptool/tree/v5.6.0

The application also bundles Python (PSF license), pyserial (BSD), PyInstaller
(GPL with its distribution exception) and transitive dependencies. The exact
installed package list and distribution license files are preserved under
Contents/Resources/ThirdParty in the Mac application. The worker is a separate
process and communicates using JSON lines; users do not need to install Python.

The architecture follows FoloToy Magic-Flash (native Go host, embedded local
HTML/CSS/JS interface, separate esptool worker). Play content, images and
firmware are downloaded at runtime from the AI Passport community and are not
part of this repository or its installers.

The one exception is the AI Passport factory firmware
(AI-Passport-Factory-Default-v1.1.2: bootloader.bin, partition-table.bin,
trae_card.bin, imgava.bin), which is FoloToy's own firmware. It is embedded in
the application under internal/factory so a device can be restored without a
network. The factory restore flow is ported from the former
FoloToy-AI-Passport-Factory-Restore tool (v1.5.2).
