# Third-party software

AI Passport 玩法社区 uses Wails v3 (MIT) for its desktop host. Dependency
versions and module checksums are pinned in go.mod and go.sum. Wails source:
https://github.com/wailsapp/wails/tree/v3.0.0-beta.24

Serial ports are opened and enumerated with go.bug.st/serial v1.8.0
(BSD-3-Clause, https://github.com/bugst/go-serial) and golang.org/x/sys
(BSD-3-Clause).

Flashing runs inside the app (`pkg/esp`, `pkg/device`); there is no
Python, PyInstaller or esptool in the application. `pkg/esp` speaks the
ESP32 serial bootloader protocol documented by Espressif
(https://docs.espressif.com/projects/esptool/en/latest/esp32c3/advanced-topics/serial-protocol.html),
using the same command sequences, reset timings and register addresses as
esptool (https://github.com/espressif/esptool, GPL-2.0-or-later).

The ESP32-C3 flasher stub uploaded to the chip's RAM
(`pkg/esp/stub/esp32c3.json`) is from esp-flasher-stub v1.4.1 as shipped
in esptool 5.6.0 (`esptool/targets/stub_flasher/2/esp32c3.json`). It is dual
licensed Apache-2.0 or MIT; both license texts are in `pkg/esp/stub/` and
in the ThirdParty folder of the packaged app.
Source: https://github.com/espressif/esp-flasher-stub

The architecture follows FoloToy Magic-Flash (native Go host, embedded local
HTML/CSS/JS interface). Play content, images and firmware are downloaded at
runtime from the AI Passport community and are not part of this repository or
its installers.

The one exception is the AI Passport factory firmware
(AI-Passport-Factory-Default-v1.1.2: bootloader.bin, partition-table.bin,
trae_card.bin, imgava.bin), which is FoloToy's own firmware. It is embedded in
the application under pkg/factory so a device can be restored without a
network. The factory restore flow is ported from the former
FoloToy-AI-Passport-Factory-Restore tool (v1.5.2).
