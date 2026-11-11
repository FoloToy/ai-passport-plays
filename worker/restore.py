# SPDX-License-Identifier: GPL-2.0-or-later
"""AI Passport factory restore (ported from FoloToy-AI-Passport-Factory-Restore v1.5.2).

Safety rules carried over unchanged:
  * write the four factory segments separately; never erase_flash, never write cardid;
  * refuse anything that is not an ESP32-C3 with exactly 8 MB flash and security disabled;
  * back up the partition table and cardid (and by default the whole flash) before writing;
  * verify every segment, then require cardid to be byte-for-byte unchanged;
  * once writing starts, a cancel request is ignored until writing and verifying finish.

All esptool work runs over one connection, so the port cannot change between steps.
"""
from __future__ import annotations

import contextlib
import hashlib
import os
import re
import shutil
import sys
import threading
import time
from dataclasses import dataclass
from datetime import datetime
from pathlib import Path
from typing import Callable

CHIP = 'esp32c3'
FACTORY_VERSION = 'AI-Passport-Factory-Default-v1.1.2'
FLASH_SIZE = 0x800000
CARDID_OFFSET = 0x356000
CARDID_SIZE = 0x4000
PARTITION_OFFSET = 0x8000
PARTITION_SIZE = 0x1000
BAUD = 460800
CONNECT_TIMEOUT = 150.0
SCAN_INTERVAL = 0.015
ESPRESSIF_VID = 0x303A


@dataclass(frozen=True)
class Segment:
    offset: int
    filename: str
    size: int
    sha256: str

    @property
    def end(self):
        return self.offset + self.size


# Must match internal/factory/factory.go (test_worker.py checks this).
SEGMENTS = (
    Segment(0x000000, 'bootloader.bin', 22496, 'fcbfb4e8045425054639ef344d858d96b36d6540c8f2a9d26cb00c78f7af03f7'),
    Segment(0x008000, 'partition-table.bin', 3072, '590cd71c08f65e3af9cd7bc74047b6ba3eeda319ca4f1be6ebbf99cb7facea9c'),
    Segment(0x010000, 'trae_card.bin', 2665312, '94410524091d19c963ee102a2319ab9dccf424f391d25777dc9ec2cded87495f'),
    Segment(0x3FA000, 'imgava.bin', 1048576, '88f223c10302372bdfcbdf0135270958014232fa4c00854ccc11f9407b4d1481'),
)


class RestoreError(Exception):
    def __init__(self, code, message):
        super().__init__(message)
        self.code = code


class Cancelled(RestoreError):
    def __init__(self):
        super().__init__('cancelled', 'Stopped before anything was written')


@dataclass(frozen=True)
class Candidate:
    device: str
    description: str
    vid: int | None
    pid: int | None
    serial_number: str | None
    location: str | None

    @property
    def tracking_key(self):
        if self.serial_number:
            return 'serial:' + self.serial_number.lower()
        if self.location:
            return 'location:' + self.location.lower()
        return 'vidpid:%s:%s' % (self.vid, self.pid)


def sha256_file(path):
    digest = hashlib.sha256()
    with open(path, 'rb') as stream:
        for block in iter(lambda: stream.read(1 << 20), b''):
            digest.update(block)
    return digest.hexdigest()


def check_layout(segments=SEGMENTS):
    for seg in segments:
        if seg.offset < CARDID_OFFSET + CARDID_SIZE and seg.end > CARDID_OFFSET:
            raise RestoreError('firmware', '%s overlaps the cardid identity area' % seg.filename)


def validate_firmware(firmware_dir):
    check_layout()
    paths = []
    for seg in SEGMENTS:
        path = Path(firmware_dir) / seg.filename
        if not path.is_file():
            raise RestoreError('firmware', 'Factory firmware file missing: %s' % seg.filename)
        if path.stat().st_size != seg.size or sha256_file(path) != seg.sha256:
            raise RestoreError('firmware', 'Factory firmware failed its check: %s' % seg.filename)
        paths.append((seg.offset, str(path)))
    return paths


def cardid_status(data):
    """Describe the identity area without exposing any of its contents."""
    if len(data) != CARDID_SIZE:
        return 'bad_length'
    if data == b'\xff' * CARDID_SIZE:
        return 'blank_ff'
    if data == b'\x00' * CARDID_SIZE:
        return 'blank_00'
    if all(key in data for key in (b'ProductKey', b'DeviceKey', b'DeviceSecret')):
        return 'keys'
    return 'unknown'


# ---------------------------------------------------------------- USB discovery

def list_candidates():
    from serial.tools import list_ports
    found = []
    for port in list_ports.comports():
        # macOS lists each device twice; only the /dev/cu.* callout node is usable.
        if sys.platform == 'darwin' and not port.device.startswith('/dev/cu.'):
            continue
        text = ('%s %s %s' % (port.description or '', port.manufacturer or '', port.hwid or '')).lower()
        if port.vid != ESPRESSIF_VID and 'espressif' not in text:
            continue
        found.append(Candidate(port.device, port.description or 'Espressif USB', port.vid, port.pid,
                               port.serial_number, port.location))
    return sorted(found, key=lambda c: c.device)


def quick_bootloader_reset(candidate):
    """Open the USB CDC port briefly and request ROM download mode.

    Some broken firmware only keeps USB up for ~300 ms, which is shorter than esptool's own
    USB-JTAG reset (it sleeps 100 ms per step), so this uses the same order with 5 ms steps.
    """
    import serial
    port = serial.Serial()
    port.port = candidate.device
    port.baudrate = 115200
    port.timeout = 0.1
    port.write_timeout = 0.1
    port.dtr = False
    port.rts = False
    try:
        try:
            port.open()
        except (OSError, serial.SerialException):
            return False

        def set_rts(state):
            port.setRTS(state)
            port.setDTR(port.dtr)  # Windows usbser.sys only applies RTS together with DTR

        set_rts(False)
        port.setDTR(False)
        time.sleep(0.005)
        port.setDTR(True)
        set_rts(False)
        time.sleep(0.005)
        set_rts(True)
        port.setDTR(False)
        set_rts(True)
        time.sleep(0.005)
        port.setDTR(False)
        set_rts(False)
        return True
    except (OSError, serial.SerialException):
        # Opening the port is what matters; a later error usually means USB is re-enumerating.
        return True
    finally:
        if port.is_open:
            port.close()


def problem_port_hints():
    """Windows only: map a device stuck in Code 43 (VID_0000&PID_0002) back to its last COM port."""
    if os.name != 'nt':
        return []
    import ctypes
    import winreg
    from ctypes import wintypes

    def subkeys(key):
        names, i = [], 0
        while True:
            try:
                names.append(winreg.EnumKey(key, i))
            except OSError:
                return names
            i += 1

    def present(instance_id):
        devinst = wintypes.ULONG()
        locate = ctypes.WinDLL('cfgmgr32').CM_Locate_DevNodeW
        locate.argtypes = [ctypes.POINTER(wintypes.ULONG), wintypes.LPWSTR, wintypes.ULONG]
        locate.restype = wintypes.ULONG
        return locate(ctypes.byref(devinst), instance_id, 0) == 0

    enum_usb = r'SYSTEM\CurrentControlSet\Enum\USB'
    problem, parent, serial_hw = 'VID_0000&PID_0002', 'VID_303A&PID_1001', 'VID_303A&PID_1001&MI_00'
    hints = set()
    try:
        with winreg.OpenKey(winreg.HKEY_LOCAL_MACHINE, '%s\\%s' % (enum_usb, problem)) as key:
            instances = subkeys(key)
    except OSError:
        return []
    for suffix in instances:
        if not present('USB\\%s\\%s' % (problem, suffix)):
            continue
        try:
            with winreg.OpenKey(winreg.HKEY_LOCAL_MACHINE, '%s\\%s\\%s' % (enum_usb, parent, suffix)) as key:
                prefix = str(winreg.QueryValueEx(key, 'ParentIdPrefix')[0]).lower() + '&'
            with winreg.OpenKey(winreg.HKEY_LOCAL_MACHINE, '%s\\%s' % (enum_usb, serial_hw)) as root:
                serial_instances = subkeys(root)
        except OSError:
            continue
        for inst in serial_instances:
            if not inst.lower().startswith(prefix):
                continue
            try:
                with winreg.OpenKey(winreg.HKEY_LOCAL_MACHINE,
                                    '%s\\%s\\%s\\Device Parameters' % (enum_usb, serial_hw, inst)) as params:
                    name = str(winreg.QueryValueEx(params, 'PortName')[0]).upper()
            except OSError:
                continue
            if re.fullmatch(r'COM\d+', name):
                hints.add(name)
    return sorted(hints)


# ---------------------------------------------------------------- esptool access

class Device:
    """One open esptool connection. Tests replace this with a fake."""

    def __init__(self, esp):
        self.esp = esp

    @classmethod
    def connect(cls, port, mode, attempts):
        from esptool import cmds
        return cls(cmds.detect_chip(port=port, baud=115200, connect_mode=mode, connect_attempts=attempts))

    def chip(self):
        return self.esp.CHIP_NAME

    def mac(self):
        return ':'.join('%02x' % b for b in self.esp.read_mac('BASE_MAC'))

    def security(self):
        """Return (secure_boot, flash_encryption); raise if the chip will not say."""
        return bool(self.esp.get_secure_boot_enabled()), bool(self.esp.get_flash_encryption_enabled())

    def prepare(self):
        """Load the stub, check flash size and switch to the fast baud rate."""
        from esptool import cmds
        self.esp = self.esp.run_stub()
        size = cmds.detect_flash_size(self.esp)
        try:
            self.esp.change_baud(BAUD)
        except Exception:
            pass
        return size

    def read(self, offset, size, path):
        from esptool import cmds
        cmds.read_flash(self.esp, offset, size, str(path), flash_size='keep')

    def write(self, addr_files):
        from esptool import cmds
        cmds.write_flash(self.esp, addr_files, flash_freq='keep', flash_mode='keep', flash_size='keep',
                         compress=True, erase_all=False, force=False)

    def md5(self, offset, size):
        """MD5 of a flash range, computed on the device itself."""
        return self.esp.flash_md5sum(offset, size)

    def verify(self, addr_files):
        from esptool import cmds
        cmds.verify_flash(self.esp, addr_files, flash_freq='keep', flash_mode='keep', flash_size='keep')

    def reset(self):
        from esptool import cmds
        cmds.reset_chip(self.esp, 'hard-reset')

    def close(self):
        with contextlib.suppress(Exception):
            self.esp._port.close()


def parse_size(text):
    match = re.match(r'(\d+)\s*MB$', str(text or '').strip(), re.IGNORECASE)
    return int(match.group(1)) << 20 if match else None


# ---------------------------------------------------------------- workflow

class Restore:
    def __init__(self, firmware_dir, backup_root, full_backup, emit, cancel,
                 port=None, connect=Device.connect, candidates=list_candidates,
                 capture=quick_bootloader_reset, hints=problem_port_hints,
                 timeout=CONNECT_TIMEOUT, interval=SCAN_INTERVAL, mode='restore'):
        # mode 'backup' only reads the device: it saves the whole flash and never writes.
        self.mode = mode
        self.firmware_dir = Path(firmware_dir) if firmware_dir else None
        self.backup_root = Path(backup_root)
        self.full_backup = full_backup
        self.emit = emit
        self.cancel = cancel
        self.port = port or ''
        self.connect = connect
        self.candidates = candidates
        self.capture = capture
        self.hints = hints
        self.timeout = timeout
        self.interval = interval
        self.write_started = False
        self.log_path = None
        self._lines = []

    # -- logging: every line goes to the UI and to the log file in the backup folder
    def raw(self, message):
        message = str(message).rstrip()
        if not message:
            return
        self._lines.append(message)
        self.emit('log', message=message[:2000])
        if self.log_path:
            with open(self.log_path, 'a', encoding='utf-8') as stream:
                stream.write(message + '\n')

    def log(self, message):
        self.raw('[%s] %s' % (datetime.now().strftime('%H:%M:%S'), message))

    def stage(self, name):
        self.emit('stage', stage=name)

    def check_cancel(self):
        if self.cancel.is_set() and not self.write_started:
            raise Cancelled()

    def run(self):
        self.backup_root.mkdir(parents=True, exist_ok=True)
        self.log_path = self.backup_root / 'last-connection.log'
        self.log_path.write_text('', encoding='utf-8')
        self.stage('preparing')
        addr_files = None
        if self.mode == 'restore':
            self.log('Factory firmware %s' % FACTORY_VERSION)
            addr_files = validate_firmware(self.firmware_dir)
            self.log('Factory firmware size, SHA-256 and cardid layout checked')
        else:
            self.log('Firmware backup (read only; nothing is written to the device)')

        device = None
        try:
            if self.port:
                self.stage('connecting')
                device = self._open(self.port, 'default-reset', 10)
            else:
                device = self.auto_connect()
            if self.mode == 'backup':
                return self._backup(device)
            return self._restore(device, addr_files)
        finally:
            if device is not None:
                device.close()

    def _open(self, port, mode, attempts):
        try:
            return self.connect(port, mode, attempts)
        except RestoreError:
            raise
        except Exception as error:
            raise RestoreError('connect', str(error))

    def _restore(self, device, addr_files):
        self.check_cancel()
        chip = device.chip()
        if chip.lower().replace('-', '') != CHIP:
            raise RestoreError('wrong_chip', 'Detected %s; AI Passport uses ESP32-C3' % chip)
        mac = device.mac()
        self.log('ESP32-C3 locked on %s, MAC %s' % (self.port, mac))

        session = self.backup_root / ('%s-%s' % (datetime.now().strftime('%Y%m%d-%H%M%S'), mac.replace(':', '')))
        session.mkdir(parents=True, exist_ok=False)
        self.log_path = session / 'factory-restore.log'
        self.log_path.write_text('\n'.join(self._lines) + '\n', encoding='utf-8')
        self.log('Backup folder: %s' % session)

        self.stage('checking')
        try:
            secure_boot, encryption = device.security()
        except Exception as error:
            raise RestoreError('secure', 'Could not confirm the security settings: %s' % error)
        if secure_boot or encryption:
            raise RestoreError('secure', 'Secure Boot or Flash Encryption is enabled; refusing to write')
        self.log('Secure Boot and Flash Encryption are both disabled')
        size_text = device.prepare()
        if parse_size(size_text) != FLASH_SIZE:
            raise RestoreError('flash_size', 'The factory firmware needs exactly 8 MB flash, found %s' % (size_text or 'unknown'))
        self.emit('identified', chip=chip, mac=mac, flash=size_text)
        self.log('8 MB flash confirmed')

        self.check_cancel()
        self.stage('backup')
        partition = session / 'partition-table-before.bin'
        self.log('Backing up the partition table')
        self._read(device, PARTITION_OFFSET, PARTITION_SIZE, partition)
        cardid_before = session / 'cardid-before.bin'
        self.log('Backing up the cardid identity area')
        self._read(device, CARDID_OFFSET, CARDID_SIZE, cardid_before)
        cardid_hash = sha256_file(cardid_before)
        status = cardid_status(cardid_before.read_bytes())
        self.log('cardid SHA-256 %s, status %s' % (cardid_hash, status))

        full = None
        if self.full_backup:
            if shutil.disk_usage(session).free < 32 << 20:
                raise RestoreError('disk_space', 'Less than 32 MB free in the backup folder')
            full = session / 'full-flash-before-restore.bin'
            self.log('Backing up the whole 8 MB flash; this takes a few minutes')
            self._read(device, 0, FLASH_SIZE, full)
            self.log('Full backup SHA-256 %s' % sha256_file(full))

        self.check_cancel()
        self.write_started = True
        self.stage('writing')
        self.log('Writing the four factory segments (no erase_flash, cardid untouched)')
        self._esptool('write', lambda: device.write(addr_files))
        self.stage('verifying')
        self.log('Verifying every segment')
        self._esptool('verify', lambda: device.verify(addr_files))

        self.stage('cardid')
        cardid_after = session / 'cardid-after.bin'
        self._read(device, CARDID_OFFSET, CARDID_SIZE, cardid_after)
        if sha256_file(cardid_after) != cardid_hash:
            raise RestoreError('cardid_changed', 'cardid changed during the restore; stop using this device and contact FoloToy')
        self.log('cardid is byte-for-byte unchanged')

        (session / 'factory-restore-report.txt').write_text('\n'.join([
            'tool=AI Passport Plays factory restore',
            'factory_firmware=%s' % FACTORY_VERSION,
            'time=%s' % datetime.now().isoformat(timespec='seconds'),
            'port=%s' % self.port,
            'chip=%s' % CHIP,
            'mac=%s' % mac,
            'flash_size=8MB',
            'secure_boot=disabled',
            'flash_encryption=disabled',
            'cardid_sha256=%s' % cardid_hash,
            'cardid_status=%s' % status,
            'full_backup=%s' % ('yes' if full else 'no'),
            'erase_flash=no',
            'verify_flash=passed',
            'cardid_unchanged=yes',
            'result=success',
        ]) + '\n', encoding='utf-8')

        self.stage('resetting')
        try:
            device.reset()
        except Exception as error:
            self.log('Firmware verified, but the automatic reset failed (%s); unplug and replug the device' % error)
        self.log('Factory restore finished: firmware verified, cardid unchanged')
        return dict(mac=mac, session=str(session), cardid=status)

    def _backup(self, device):
        """Save the device's whole flash, checked against an MD5 computed on the device."""
        self.check_cancel()
        chip = device.chip()
        if chip.lower().replace('-', '') != CHIP:
            raise RestoreError('wrong_chip', 'Detected %s; AI Passport uses ESP32-C3' % chip)
        mac = device.mac()
        self.log('ESP32-C3 locked on %s, MAC %s' % (self.port, mac))
        stamp = datetime.now().strftime('%Y%m%d-%H%M%S')
        session = self.backup_root / ('%s-%s-firmware-backup' % (stamp, mac.replace(':', '')))
        session.mkdir(parents=True, exist_ok=False)
        self.log_path = session / 'firmware-backup.log'
        self.log_path.write_text('\n'.join(self._lines) + '\n', encoding='utf-8')
        self.log('Backup folder: %s' % session)

        self.stage('checking')
        try:
            secure_boot, encryption = device.security()
            security = 'secure_boot=%s, flash_encryption=%s' % (
                'enabled' if secure_boot else 'disabled', 'enabled' if encryption else 'disabled')
        except Exception as error:
            encryption, security = False, 'unknown (%s)' % error
        if encryption:
            self.log('Flash Encryption is enabled; the backup holds encrypted data and only fits this device')
        size_text = device.prepare()
        size = parse_size(size_text)
        if not size:
            raise RestoreError('flash_size', 'Could not detect the flash size (%s)' % (size_text or 'unknown'))
        self.emit('identified', chip=chip, mac=mac, flash=size_text)

        self.check_cancel()
        self.stage('backup')
        name = 'AI-Passport-%s-%s-full-flash.bin' % (mac.replace(':', ''), stamp)
        path = session / name
        self.log('Reading the whole %s flash; this takes a few minutes' % size_text)
        if shutil.disk_usage(session).free < size + (32 << 20):
            raise RestoreError('disk_space', 'Not enough free space in the backup folder')
        self._read(device, 0, size, path)

        self.stage('verifying')
        local_md5 = hashlib.md5(path.read_bytes()).hexdigest()
        try:
            device_md5 = device.md5(0, size).lower()
        except Exception as error:
            raise RestoreError('backup', 'Could not check the backup against the device: %s' % error)
        if device_md5 != local_md5:
            raise RestoreError('backup', 'The saved backup does not match the device; try again')
        digest = sha256_file(path)
        cardid = cardid_status(path.read_bytes()[CARDID_OFFSET:CARDID_OFFSET + CARDID_SIZE]) if size >= CARDID_OFFSET + CARDID_SIZE else 'bad_length'
        (path.parent / (name + '.sha256')).write_text('%s  %s\n' % (digest, name), encoding='ascii')
        (session / 'firmware-backup-report.txt').write_text('\n'.join([
            'tool=AI Passport Plays firmware backup',
            'time=%s' % datetime.now().isoformat(timespec='seconds'),
            'port=%s' % self.port,
            'chip=%s' % CHIP,
            'mac=%s' % mac,
            'flash_size=%s' % size_text,
            'security=%s' % security,
            'file=%s' % name,
            'sha256=%s' % digest,
            'md5_device_match=yes',
            'cardid_status=%s' % cardid,
            'written_to_device=no',
            'result=success',
        ]) + '\n', encoding='utf-8')
        self.log('Backup saved and checked against the device: %s' % path)

        self.stage('resetting')
        try:
            device.reset()
        except Exception as error:
            self.log('Backup finished, but the automatic reset failed (%s); unplug and replug the device' % error)
        return dict(mac=mac, session=str(session), cardid=cardid, file=str(path))

    def _read(self, device, offset, size, path):
        self._esptool('read', lambda: device.read(offset, size, path))
        if not path.is_file() or path.stat().st_size != size:
            got = path.stat().st_size if path.exists() else 0
            raise RestoreError('backup', 'Backup %s has %d of %d bytes' % (path.name, got, size))

    def _esptool(self, what, fn):
        try:
            fn()
        except RestoreError:
            raise
        except Exception as error:
            code = 'write' if self.write_started else 'connect'
            raise RestoreError(code, '%s failed: %s' % (what, error))

    # -- automatic device capture
    def auto_connect(self):
        """Wait up to 150 s for exactly one Espressif USB device and catch it in ROM download mode."""
        self.stage('waiting')
        self.log('Scanning for an Espressif USB device; a port that appears for only a few hundred ms is still caught')
        deadline = time.monotonic() + self.timeout
        last_notice = last_capture = next_hint = 0.0
        present = False
        hints = []
        hint_sent = multi_sent = False
        last_error = ''

        def probe():
            nonlocal last_error
            rom_deadline = min(deadline, time.monotonic() + 4.0)
            while time.monotonic() < rom_deadline:
                self.check_cancel()
                found = self.candidates()
                if len(found) == 1:
                    try:
                        device = self.connect(found[0].device, 'no-reset', 10)
                    except Exception as error:
                        last_error = 'ROM handshake: %s' % error
                        return None
                    self.port = found[0].device
                    self.stage('connecting')
                    self.log('Caught %s (%s)' % (found[0].device, found[0].description))
                    return device
                time.sleep(self.interval)
            return None

        def try_capture(candidate):
            nonlocal last_error
            try:
                captured = self.capture(candidate)
            except Exception as error:
                last_error = 'reset: %s' % error
                return None
            return probe() if captured else None

        while time.monotonic() < deadline:
            self.check_cancel()
            now = time.monotonic()
            if now >= next_hint:
                hints = self.hints()
                next_hint = now + 1.0
                if len(hints) == 1 and not hint_sent:
                    hint_sent = True
                    self.emit('hint', code='code43', port=hints[0])
                    self.log('USB Code 43 seen; watching the last known port %s. Unplug and replug the device.' % hints[0])
            found = self.candidates()
            if len(found) > 1:
                if not multi_sent or now - last_notice >= 5:
                    self.emit('hint', code='multiple')
                    self.log('More than one Espressif device is connected; keep only the one to restore')
                    multi_sent, last_notice = True, now
                time.sleep(self.interval)
                continue
            if found:
                if not present or now - last_capture >= 3.0:
                    last_capture = now
                    device = try_capture(found[0])
                    if device:
                        return device
                present = True
            else:
                present = False
                if len(hints) == 1:
                    device = try_capture(Candidate(hints[0], 'Espressif USB (Code 43)', ESPRESSIF_VID, 0x1001, None, None))
                    if device:
                        return device
            if now - last_notice >= 10:
                self.emit('hint', code='replug')
                self.log('Still waiting for the device' + (' (last error: %s)' % last_error if last_error else ''))
                last_notice = now
            time.sleep(self.interval)
        raise RestoreError('not_found', 'No device was caught within %d s. Close any serial monitor, then hold BOOT while plugging in USB.%s'
                           % (int(self.timeout), (' Last error: ' + last_error) if last_error else ''))


def watch_cancel(stream, event):
    """Set event when the app sends {"command":"cancel"} on stdin."""
    def run():
        for line in stream:
            if '"cancel"' in line:
                event.set()
    thread = threading.Thread(target=run, name='cancel-watch', daemon=True)
    thread.start()
    return thread
