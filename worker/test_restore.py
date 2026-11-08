import re
import sys
import tempfile
import threading
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
import restore  # noqa: E402

ROOT = Path(__file__).resolve().parent.parent
FIRMWARE = ROOT / 'internal' / 'factory' / 'firmware'
MAC = '4c:11:ae:32:62:34'


def candidate(device='COM7', serial='ABC123', location='1-2'):
    return restore.Candidate(device, 'USB JTAG/serial debug unit', 0x303A, 0x1001, serial, location)


class FakeDevice:
    """Simulates an 8 MB AI Passport with an identity area."""

    def __init__(self, chip='ESP32-C3', size='8MB', security=(False, False), on_write=None):
        self._chip, self._size, self._security = chip, size, security
        self.flash = bytearray(b'\xff' * restore.FLASH_SIZE)
        marker = b'ProductKey\0DeviceKey\0DeviceSecret\0'
        self.flash[restore.CARDID_OFFSET:restore.CARDID_OFFSET + len(marker)] = marker
        self.calls = []
        self.on_write = on_write

    def chip(self):
        return self._chip

    def mac(self):
        return MAC

    def security(self):
        if isinstance(self._security, Exception):
            raise self._security
        return self._security

    def prepare(self):
        self.calls.append('prepare')
        return self._size

    def read(self, offset, size, path):
        self.calls.append(('read', offset, size))
        Path(path).write_bytes(bytes(self.flash[offset:offset + size]))

    def write(self, addr_files):
        self.calls.append(('write', [a for a, _ in addr_files]))
        if self.on_write:
            self.on_write(self)
        for addr, path in addr_files:
            data = Path(path).read_bytes()
            self.flash[addr:addr + len(data)] = data

    def verify(self, addr_files):
        self.calls.append('verify')
        for addr, path in addr_files:
            data = Path(path).read_bytes()
            assert bytes(self.flash[addr:addr + len(data)]) == data

    def reset(self):
        self.calls.append('reset')

    def close(self):
        pass


def make(tmp, device, full=False, cancel=None, **kw):
    events = []
    job = restore.Restore(FIRMWARE, tmp, full, lambda kind, **f: events.append(dict(type=kind, **f)),
                          cancel or threading.Event(), port=kw.pop('port', 'COM9'),
                          connect=lambda port, mode, attempts: device, **kw)
    return job, events


class Layout(unittest.TestCase):
    def test_segments_match_go_table(self):
        go = (ROOT / 'internal' / 'factory' / 'factory.go').read_text()
        rows = re.findall(r'\{0x([0-9A-Fa-f]+), "([\w.-]+)", (\d+), "([0-9a-f]{64})"\}', go)
        self.assertEqual([(int(o, 16), f, int(s), h) for o, f, s, h in rows],
                         [(s.offset, s.filename, s.size, s.sha256) for s in restore.SEGMENTS])
        self.assertIn(restore.FACTORY_VERSION, go)

    def test_segments_never_overlap_cardid(self):
        restore.check_layout()
        bad = restore.SEGMENTS + (restore.Segment(0x350000, 'merged.bin', 0x10000, ''),)
        with self.assertRaises(restore.RestoreError):
            restore.check_layout(bad)

    def test_bundled_firmware_validates(self):
        self.assertEqual(len(restore.validate_firmware(FIRMWARE)), 4)

    def test_tampered_firmware_refused(self):
        with tempfile.TemporaryDirectory() as d:
            for seg in restore.SEGMENTS:
                (Path(d) / seg.filename).write_bytes((FIRMWARE / seg.filename).read_bytes())
            (Path(d) / 'imgava.bin').write_bytes(b'\0' * restore.SEGMENTS[3].size)
            with self.assertRaises(restore.RestoreError) as ctx:
                restore.validate_firmware(d)
            self.assertEqual(ctx.exception.code, 'firmware')

    def test_cardid_status_never_reveals_contents(self):
        data = bytearray(b'\xff' * restore.CARDID_SIZE)
        marker = b'ProductKey\0DeviceKey\0DeviceSecret\0secret-value'
        data[:len(marker)] = marker
        self.assertEqual(restore.cardid_status(bytes(data)), 'keys')
        self.assertEqual(restore.cardid_status(b'\xff' * restore.CARDID_SIZE), 'blank_ff')
        self.assertEqual(restore.cardid_status(b'\0' * restore.CARDID_SIZE), 'blank_00')
        self.assertEqual(restore.cardid_status(b'x'), 'bad_length')


class Workflow(unittest.TestCase):
    def test_full_restore_preserves_cardid(self):
        dev = FakeDevice()
        cardid = bytes(dev.flash[restore.CARDID_OFFSET:restore.CARDID_OFFSET + restore.CARDID_SIZE])
        with tempfile.TemporaryDirectory() as d:
            job, events = make(d, dev, full=True)
            result = job.run()
            session = Path(result['session'])
            report = (session / 'factory-restore-report.txt').read_text()
            self.assertIn('cardid_unchanged=yes', report)
            self.assertIn('erase_flash=no', report)
            self.assertEqual((session / 'cardid-before.bin').read_bytes(), cardid)
            self.assertEqual((session / 'full-flash-before-restore.bin').stat().st_size, restore.FLASH_SIZE)
            self.assertTrue((session / 'factory-restore.log').read_text())
            self.assertEqual(result['cardid'], 'keys')
        writes = [c for c in dev.calls if isinstance(c, tuple) and c[0] == 'write']
        self.assertEqual(writes, [('write', [0x0, 0x8000, 0x10000, 0x3FA000])])
        self.assertIn('verify', dev.calls)
        self.assertEqual(dev.calls[-1], 'reset')
        stages = [e['stage'] for e in events if e['type'] == 'stage']
        self.assertEqual(stages, ['preparing', 'connecting', 'checking', 'backup', 'writing', 'verifying', 'cardid', 'resetting'])

    def test_wrong_chip_never_writes(self):
        dev = FakeDevice(chip='ESP32-S3')
        with tempfile.TemporaryDirectory() as d:
            with self.assertRaises(restore.RestoreError) as ctx:
                make(d, dev)[0].run()
        self.assertEqual(ctx.exception.code, 'wrong_chip')
        self.assertFalse(any(isinstance(c, tuple) and c[0] == 'write' for c in dev.calls))

    def test_not_exactly_8mb_never_writes(self):
        for size in ('4MB', '16MB', None):
            dev = FakeDevice(size=size)
            with tempfile.TemporaryDirectory() as d:
                with self.assertRaises(restore.RestoreError) as ctx:
                    make(d, dev)[0].run()
            self.assertEqual(ctx.exception.code, 'flash_size')
            self.assertFalse(any(isinstance(c, tuple) and c[0] == 'write' for c in dev.calls))

    def test_security_enabled_or_unknown_fails_closed(self):
        for sec in ((True, False), (False, True), RuntimeError('no answer')):
            dev = FakeDevice(security=sec)
            with tempfile.TemporaryDirectory() as d:
                with self.assertRaises(restore.RestoreError) as ctx:
                    make(d, dev)[0].run()
            self.assertEqual(ctx.exception.code, 'secure')
            self.assertNotIn('prepare', dev.calls)

    def test_cancel_before_writing_stops(self):
        dev = FakeDevice()
        cancel = threading.Event()
        cancel.set()
        with tempfile.TemporaryDirectory() as d:
            with self.assertRaises(restore.Cancelled):
                make(d, dev, cancel=cancel)[0].run()
        self.assertFalse(any(isinstance(c, tuple) and c[0] == 'write' for c in dev.calls))

    def test_cancel_after_writing_started_is_ignored(self):
        cancel = threading.Event()
        dev = FakeDevice(on_write=lambda _d: cancel.set())
        with tempfile.TemporaryDirectory() as d:
            result = make(d, dev, cancel=cancel)[0].run()
        self.assertEqual(result['mac'], MAC)
        self.assertIn('verify', dev.calls)

    def test_cardid_change_is_reported(self):
        def corrupt(dev):
            dev.flash[restore.CARDID_OFFSET] ^= 0xFF
        dev = FakeDevice(on_write=corrupt)
        with tempfile.TemporaryDirectory() as d:
            with self.assertRaises(restore.RestoreError) as ctx:
                make(d, dev)[0].run()
        self.assertEqual(ctx.exception.code, 'cardid_changed')


class AutoConnect(unittest.TestCase):
    def job(self, provider, capture, hints=lambda: [], device=None, timeout=5.0):
        connected = []

        def connect(port, mode, attempts):
            connected.append((port, mode))
            return device or FakeDevice()
        job = restore.Restore(FIRMWARE, tempfile.gettempdir(), False, lambda *a, **k: None, threading.Event(),
                              connect=connect, candidates=provider, capture=capture, hints=hints,
                              timeout=timeout, interval=0.001)
        return job, connected

    def test_usb_identity_survives_port_rename(self):
        self.assertEqual(candidate('COM7').tracking_key, candidate('COM21').tracking_key)

    def test_short_usb_window_is_captured_then_rom_port_probed(self):
        state = {'polls': 0, 'captured': False}

        def provider():
            state['polls'] += 1
            if state['captured']:
                return [candidate('COM21')]
            return [] if state['polls'] < 4 else [candidate('COM47')]

        captured = []

        def capture(c):
            captured.append(c.device)
            state['captured'] = True
            return True
        job, connected = self.job(provider, capture)
        job.auto_connect()
        self.assertEqual(captured, ['COM47'])
        self.assertEqual(connected, [('COM21', 'no-reset')])
        self.assertEqual(job.port, 'COM21')

    def test_code43_port_is_opened_without_enumeration(self):
        state = {'captured': False}
        opened = []

        def capture(c):
            opened.append(c.device)
            state['captured'] = True
            return True
        job, _ = self.job(lambda: [candidate('COM47')] if state['captured'] else [], capture, hints=lambda: ['COM47'])
        job.auto_connect()
        self.assertEqual(opened, ['COM47'])

    def test_multiple_devices_are_never_written(self):
        job, connected = self.job(lambda: [candidate('COM3'), candidate('COM4', serial='X')],
                                  lambda c: True, timeout=0.05)
        with self.assertRaises(restore.RestoreError) as ctx:
            job.auto_connect()
        self.assertEqual(ctx.exception.code, 'not_found')
        self.assertEqual(connected, [])


if __name__ == '__main__':
    unittest.main()
