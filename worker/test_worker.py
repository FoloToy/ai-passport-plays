import hashlib
import importlib.util
from pathlib import Path
import sys
import tempfile
import unittest
from unittest import mock

sys.path.insert(0, str(Path(__file__).parent))  # main.py imports restore.py
spec = importlib.util.spec_from_file_location('worker', Path(__file__).with_name('main.py'))
worker = importlib.util.module_from_spec(spec)
spec.loader.exec_module(worker)


def write_image(directory, data):
    path = Path(directory) / 'firmware.bin'
    path.write_bytes(data)
    return str(path), hashlib.sha256(data).hexdigest()


class WorkerSafety(unittest.TestCase):
    def test_checksum_failure_precedes_serial_access(self):
        with tempfile.TemporaryDirectory() as d:
            image, _ = write_image(d, b'\xe9' + b'\0' * 63)
            with mock.patch('esptool.cmds.detect_chip') as detect:
                with self.assertRaises(worker.FlashError) as ctx:
                    worker.flash(dict(port='/dev/null', image=image, sha256='0' * 64))
                self.assertEqual(ctx.exception.code, 'checksum')
                detect.assert_not_called()

    def test_wrong_image_chip_refused(self):
        data = bytearray(64)
        data[0], data[12] = 0xE9, 0x09  # ESP32-S3 image
        with tempfile.TemporaryDirectory() as d:
            image, digest = write_image(d, bytes(data))
            with self.assertRaises(worker.FlashError) as ctx:
                worker.validate(dict(image=image, sha256=digest))
            self.assertEqual(ctx.exception.code, 'image_chip')

    def test_oversize_refused(self):
        with tempfile.TemporaryDirectory() as d:
            image, digest = write_image(d, b'\xe9' + b'\0' * worker.MAX_IMAGE)
            with self.assertRaises(worker.FlashError) as ctx:
                worker.validate(dict(image=image, sha256=digest))
            self.assertEqual(ctx.exception.code, 'image')

    def test_wrong_device_chip_never_writes(self):
        data = bytearray(64)
        data[0], data[12] = 0xE9, 0x05
        esp = mock.MagicMock()
        esp.CHIP_NAME = 'ESP32-S3'
        with tempfile.TemporaryDirectory() as d:
            image, digest = write_image(d, bytes(data))
            with mock.patch.object(worker, 'validate', return_value=bytes(data)), \
                 mock.patch('esptool.cmds.detect_chip', return_value=esp), \
                 mock.patch('esptool.cmds.write_flash') as write:
                with self.assertRaises(worker.FlashError) as ctx:
                    worker.flash(dict(port='/dev/null', image=image, sha256=digest))
                self.assertEqual(ctx.exception.code, 'wrong_chip')
                write.assert_not_called()

    def test_small_flash_never_writes(self):
        data = bytearray(64)
        data[0], data[12] = 0xE9, 0x05
        esp = mock.MagicMock()
        esp.CHIP_NAME = 'ESP32-C3'
        esp.get_secure_boot_enabled.return_value = False
        esp.get_flash_encryption_enabled.return_value = False
        esp.read_mac.return_value = b'\x01\x02\x03\x04\x05\x06'
        esp.run_stub.return_value = esp
        with mock.patch.object(worker, 'validate', return_value=bytes(data)), \
             mock.patch('esptool.cmds.detect_chip', return_value=esp), \
             mock.patch('esptool.cmds.detect_flash_size', return_value='4MB'), \
             mock.patch('esptool.cmds.write_flash') as write:
            with self.assertRaises(worker.FlashError) as ctx:
                worker.flash(dict(port='/dev/null', image='x', sha256='y'))
            self.assertEqual(ctx.exception.code, 'flash_size')
            write.assert_not_called()

    def test_happy_path_writes_verifies_and_resets(self):
        data = bytearray(64)
        data[0], data[12] = 0xE9, 0x05
        esp = mock.MagicMock()
        esp.CHIP_NAME = 'ESP32-C3'
        esp.get_secure_boot_enabled.return_value = False
        esp.get_flash_encryption_enabled.return_value = False
        esp.read_mac.return_value = b'\x01\x02\x03\x04\x05\x06'
        esp.run_stub.return_value = esp
        with mock.patch.object(worker, 'validate', return_value=bytes(data)), \
             mock.patch.object(worker, 'emit') as emit, \
             mock.patch('esptool.cmds.detect_chip', return_value=esp), \
             mock.patch('esptool.cmds.detect_flash_size', return_value='8MB'), \
             mock.patch('esptool.cmds.write_flash') as write, \
             mock.patch('esptool.cmds.verify_flash') as verify, \
             mock.patch('esptool.cmds.reset_chip') as reset:
            worker.flash(dict(port='/dev/null', image='x', sha256='y'))
            write.assert_called_once()
            self.assertEqual(write.call_args.args[1][0][0], 0)
            verify.assert_called_once()
            reset.assert_called_once()
            self.assertEqual(emit.call_args_list[-1].args[0], 'complete')


class CardIDBackup(unittest.TestCase):
    def image(self, cardid_bytes):
        data = bytearray(b'\xff' * (worker_restore().CARDID_OFFSET + worker_restore().CARDID_SIZE + 16))
        r = worker_restore()
        data[r.CARDID_OFFSET:r.CARDID_OFFSET + r.CARDID_SIZE] = cardid_bytes
        return bytes(data)

    def run_backup(self, device_cardid, image):
        events = []
        with tempfile.TemporaryDirectory() as d, \
             mock.patch('esptool.cmds.read_flash', return_value=device_cardid), \
             mock.patch.object(worker, 'emit', lambda kind, **f: events.append(dict(type=kind, **f))):
            worker.backup_cardid(mock.MagicMock(), image, '4c:11:ae:00:00:01', d)
            files = sorted(p.name for p in Path(d).rglob('*.bin'))
        return events, files

    def test_identity_saved_before_overwrite(self):
        r = worker_restore()
        keys = (b'ProductKey\0DeviceKey\0DeviceSecret\0').ljust(r.CARDID_SIZE, b'\xff')
        events, files = self.run_backup(keys, self.image(b'\xff' * r.CARDID_SIZE))
        self.assertEqual(len(files), 1)
        self.assertIn(dict(type='cardid', status='keys', overwrite=True), events)

    def test_short_image_or_blank_identity_is_skipped(self):
        r = worker_restore()
        events, files = self.run_backup(b'\xff' * r.CARDID_SIZE, self.image(b'\0' * r.CARDID_SIZE))
        self.assertEqual(files, [])
        with mock.patch('esptool.cmds.read_flash') as read:
            worker.backup_cardid(mock.MagicMock(), b'\xe9' * 1024, 'aa', '/nonexistent')
            read.assert_not_called()


def worker_restore():
    import restore
    return restore


if __name__ == '__main__':
    unittest.main()
