# SPDX-License-Identifier: GPL-2.0-or-later
"""AI Passport flashing worker (esptool). One process flashes one device; JSON lines on stdout.

Commands:
  ports        print the USB serial ports as JSON
  version      print worker / esptool versions
  self-test    check the bundled ESP32-C3 flasher stub
  (default)    read one JSON request (flash, restore or backup) from stdin and stream events;
               during a restore, a later {"command":"cancel"} line asks it to stop safely
"""
import contextlib
import hashlib
import json
import sys
import threading
from pathlib import Path

OUTPUT = sys.stdout
WORKER_VERSION = '0.2.0'
CHIP = 'esp32c3'
CHIP_ID = 0x0005          # ESP image extended header chip_id for ESP32-C3
MAX_IMAGE = 8 * 1024 * 1024
FLASH_OK = ('8MB', '16MB', '32MB')
ESPRESSIF_VID = 0x303A


class FlashError(Exception):
    """An error with a stable code the UI can translate."""

    def __init__(self, code, message):
        super().__init__(message)
        self.code = code


def emit(kind, **fields):
    OUTPUT.write(json.dumps(dict(type=kind, **fields), ensure_ascii=False) + '\n')
    OUTPUT.flush()


def ports():
    from serial.tools import list_ports
    result = []
    for port in list_ports.comports():
        if port.vid is None or port.pid is None:
            continue
        # On macOS /dev/cu.* is the callout device; never offer the /dev/tty.* alias too.
        if sys.platform == 'darwin' and not port.device.startswith('/dev/cu.'):
            continue
        result.append(dict(path=port.device, vid=port.vid, pid=port.pid,
                           serial=port.serial_number or '', label=port.description or port.device,
                           recommended=port.vid == ESPRESSIF_VID))
    result.sort(key=lambda p: (not p['recommended'], p['path']))
    return result


def logger(sink=None):
    from esptool.logger import TemplateLogger
    write = sink or (lambda text: emit('log', message=text[:2000]))

    class JsonLogger(TemplateLogger):
        def print(self, *args, **kwargs):
            text = ' '.join(str(a) for a in args).strip()
            if text:
                write(text)

        def note(self, message): self.print(message)
        def warning(self, message): self.print(message)
        def error(self, message): self.print(message)
        def stage(self, finish=False): pass
        def set_verbosity(self, verbosity): pass

        def progress_bar(self, cur_iter, total_iters, **kwargs):
            emit('progress', percent=int(cur_iter * 100 / total_iters) if total_iters else 0)
    return JsonLogger()


def validate(request):
    """Check the image before touching the serial port."""
    with open(request['image'], 'rb') as handle:
        data = handle.read(MAX_IMAGE + 1)
    if not data:
        raise FlashError('image', 'Firmware file is empty')
    if len(data) > MAX_IMAGE:
        raise FlashError('image', 'Firmware is larger than the 8 MB AI Passport flash')
    if request.get('size') and len(data) != request['size']:
        raise FlashError('checksum', 'Firmware size does not match the catalog')
    if hashlib.sha256(data).hexdigest() != str(request.get('sha256', '')).lower():
        raise FlashError('checksum', 'Firmware SHA-256 mismatch')
    if data[0] != 0xE9 or len(data) < 16:
        raise FlashError('image', 'Not an ESP firmware image')
    chip_id = data[12] | (data[13] << 8)
    if chip_id != CHIP_ID:
        raise FlashError('image_chip', 'Firmware is not built for ESP32-C3 (chip id 0x%04x)' % chip_id)
    from esptool.bin_image import LoadFirmwareImage
    boot = LoadFirmwareImage(CHIP, data[0:0x8000])
    if boot.checksum != boot.calculate_checksum():
        raise FlashError('image', 'Bootloader checksum mismatch')
    return data


def connect(port):
    from esptool import cmds
    try:
        return cmds.detect_chip(port=port, baud=115200, connect_attempts=4)
    except Exception as error:  # esptool raises FatalError with a readable message
        raise FlashError('connect', str(error))


def flash(request):
    from esptool import cmds
    from esptool.logger import EsptoolLogger
    data = validate(request)
    EsptoolLogger().set_logger(logger())
    esp = None
    try:
        emit('stage', stage='connecting')
        esp = connect(request['port'])
        chip = esp.CHIP_NAME
        if chip.lower().replace('-', '') != CHIP:
            raise FlashError('wrong_chip', 'Detected %s; AI Passport uses ESP32-C3' % chip)
        if esp.get_secure_boot_enabled() or esp.get_flash_encryption_enabled():
            raise FlashError('secure', 'Secure Boot or Flash Encryption is enabled on this device')
        mac = ':'.join('%02x' % b for b in esp.read_mac('BASE_MAC'))
        esp = esp.run_stub()
        size = cmds.detect_flash_size(esp)
        if size not in FLASH_OK:
            raise FlashError('flash_size', 'Flash size %s is smaller than the 8 MB AI Passport flash' % size)
        emit('identified', chip=chip, mac=mac, flash=size)
        baud = int(request.get('baud') or 460800)
        if baud != 115200:
            try:
                esp.change_baud(baud)
            except Exception as error:
                emit('log', message='Keeping 115200 baud: %s' % error)
        backup_cardid(esp, data, mac, request.get('backupDir'))
        emit('stage', stage='writing')
        cmds.write_flash(esp, [(0, data)], flash_size='keep', flash_mode='keep', flash_freq='keep',
                         compress=True, erase_all=False, force=False)
        emit('stage', stage='verifying')
        cmds.verify_flash(esp, [(0, data)], flash_size='keep', flash_mode='keep', flash_freq='keep')
        emit('stage', stage='resetting')
        cmds.reset_chip(esp, 'hard-reset')
        emit('complete', mac=mac)
    finally:
        if esp is not None:
            with contextlib.suppress(Exception):
                esp._port.close()


def backup_cardid(esp, data, mac, backup_dir):
    """Keep a copy of the device identity area before a play image overwrites it.

    Community images are merged from 0x0; ones longer than 0x356000 overwrite cardid
    (the device's SN and keys). The copy lets FoloToy recover the identity later.
    """
    import restore
    if not backup_dir or len(data) <= restore.CARDID_OFFSET:
        return
    from esptool import cmds
    try:
        current = cmds.read_flash(esp, restore.CARDID_OFFSET, restore.CARDID_SIZE, None, flash_size='keep', no_progress=True)
    except Exception as error:
        emit('log', message='Could not back up cardid: %s' % error)
        return
    status = restore.cardid_status(current)
    end = restore.CARDID_OFFSET + restore.CARDID_SIZE
    unchanged = data[restore.CARDID_OFFSET:end] == current
    emit('cardid', status=status, overwrite=not unchanged)
    if status in ('blank_ff', 'blank_00') or unchanged:
        return
    folder = Path(backup_dir) / 'cardid' / mac.replace(':', '')
    folder.mkdir(parents=True, exist_ok=True)
    target = folder / ('%s.bin' % hashlib.sha256(current).hexdigest()[:16])
    if not target.exists():
        tmp = target.with_suffix('.tmp')
        tmp.write_bytes(current)
        tmp.replace(target)
    emit('log', message='cardid backed up to %s' % target)


def run_restore(request):
    import restore
    from esptool.logger import EsptoolLogger
    cancel = threading.Event()
    restore.watch_cancel(sys.stdin, cancel)
    mode = 'backup' if request.get('command') == 'backup' else 'restore'
    job = restore.Restore(request.get('firmwareDir'), request['backupDir'], bool(request.get('fullBackup', True)),
                          emit, cancel, port=request.get('port') or None, mode=mode)
    EsptoolLogger().set_logger(logger(sink=job.raw))
    result = job.run()
    emit('complete', **result)


class LogStream:
    def write(self, text):
        if text.strip():
            emit('log', message=text.strip()[:2000])
        return len(text)

    def flush(self):
        pass

    def isatty(self):
        return False


def main():
    command = sys.argv[1] if len(sys.argv) > 1 else ''
    if command == 'self-test':
        from esptool.loader import StubFlasher
        from esptool.targets import ESP32C3ROM
        stub = StubFlasher(ESP32C3ROM)
        if not stub.text:
            raise SystemExit('Missing bundled ESP32-C3 flasher stub')
        import restore
        import serial  # noqa: F401
        restore.check_layout()
        print(json.dumps(dict(passed=True, chip=ESP32C3ROM.CHIP_NAME, stub_bytes=len(stub.text),
                              factory=restore.FACTORY_VERSION)))
        return
    if command == 'ports':
        print(json.dumps(ports(), ensure_ascii=False))
        return
    if command == 'version':
        import esptool
        print(json.dumps(dict(worker=WORKER_VERSION, esptool=esptool.__version__, platform=sys.platform)))
        return
    import restore
    try:
        request = json.loads(sys.stdin.readline())
        command = request.get('command')
        if command not in ('flash', 'restore', 'backup'):
            raise FlashError('protocol', 'Expected flash, restore or backup command')
        with contextlib.redirect_stdout(LogStream()), contextlib.redirect_stderr(LogStream()):
            if command in ('restore', 'backup'):
                run_restore(request)
            else:
                flash(request)
    except (FlashError, restore.RestoreError) as error:
        emit('error', code=error.code, message=str(error)[:2000])
        sys.exit(1)
    except Exception as error:
        emit('error', code='other', message=str(error)[:2000])
        sys.exit(1)


if __name__ == '__main__':
    main()
