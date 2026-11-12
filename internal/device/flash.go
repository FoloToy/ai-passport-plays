package device

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/FoloToy/ai-passport-plays/internal/esp"
)

const (
	chipIDESP32C3 = 0x0005 // ESP image extended header chip_id for ESP32-C3
	maxImage      = 8 << 20
	defaultBaud   = 460800
	imageHeader   = 24 // 8-byte image header + 16-byte extended header
)

// flashSizesOK are the flash sizes a play image may be written to.
var flashSizesOK = map[int]bool{8 << 20: true, 16 << 20: true, 32 << 20: true}

// validateImage checks the image before the serial port is touched.
func validateImage(req Request) ([]byte, error) {
	f, err := os.Open(req.Image)
	if err != nil {
		return nil, coded("image", "%v", err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxImage+1))
	if err != nil {
		return nil, coded("image", "%v", err)
	}
	if len(data) == 0 {
		return nil, coded("image", "Firmware file is empty")
	}
	if len(data) > maxImage {
		return nil, coded("image", "Firmware is larger than the 8 MB AI Passport flash")
	}
	if req.Size != 0 && int64(len(data)) != req.Size {
		return nil, coded("checksum", "Firmware size does not match the catalog")
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != strings.ToLower(req.SHA256) {
		return nil, coded("checksum", "Firmware SHA-256 mismatch")
	}
	if len(data) < 16 || data[0] != 0xE9 {
		return nil, coded("image", "Not an ESP firmware image")
	}
	if chip := binary.LittleEndian.Uint16(data[12:14]); chip != chipIDESP32C3 {
		return nil, coded("image_chip", "Firmware is not built for ESP32-C3 (chip id 0x%04x)", chip)
	}
	if err := checkBootloader(data[:min(len(data), 0x8000)]); err != nil {
		return nil, coded("image", "%v", err)
	}
	return data, nil
}

// checkBootloader verifies the checksum of the bootloader image at the start of a
// merged image, as esptool's LoadFirmwareImage does.
func checkBootloader(b []byte) error {
	if len(b) < imageHeader {
		return fmt.Errorf("bootloader image is truncated")
	}
	pos, sum := imageHeader, byte(0xEF)
	for i := range int(b[1]) {
		if pos+8 > len(b) {
			return fmt.Errorf("bootloader segment %d is truncated", i)
		}
		n := int(binary.LittleEndian.Uint32(b[pos+4:]))
		pos += 8
		if n > len(b)-pos {
			return fmt.Errorf("bootloader segment %d is truncated", i)
		}
		for _, c := range b[pos : pos+n] {
			sum ^= c
		}
		pos += n
	}
	pos += 15 - pos%16 // the checksum is the last byte of a 16-byte aligned block
	if pos >= len(b) {
		return fmt.Errorf("bootloader checksum is missing")
	}
	if b[pos] != sum {
		return fmt.Errorf("bootloader checksum mismatch")
	}
	return nil
}

func flashImage(ctx context.Context, connect Connector, req Request, on func(Event)) error {
	data, err := validateImage(req)
	if err != nil {
		return err
	}
	log := func(s string) { on(Event{Type: "log", Message: clip(s)}) }
	on(Event{Type: "stage", Stage: "connecting"})
	d, err := connect(req.Port, "default-reset", 4, log)
	if err != nil {
		return coded("connect", "%v", err)
	}
	defer d.Close()
	defer closeOnDone(ctx, d)()

	chip, err := d.Chip()
	if err != nil {
		return coded("connect", "%v", err)
	}
	if !isESP32C3(chip) {
		return coded("wrong_chip", "Detected %s; AI Passport uses ESP32-C3", chip)
	}
	secureBoot, encryption, err := d.Security()
	if err != nil {
		return coded("secure", "Could not confirm the security settings: %v", err)
	}
	if secureBoot || encryption {
		return coded("secure", "Secure Boot or Flash Encryption is enabled on this device")
	}
	mac, err := d.MAC()
	if err != nil {
		return coded("connect", "%v", err)
	}
	baud := req.Baud
	if baud == 0 {
		baud = defaultBaud
	}
	size, err := d.Prepare(baud)
	if err != nil {
		return coded("connect", "%v", err)
	}
	if !flashSizesOK[size] {
		return coded("flash_size", "Flash size %s is smaller than the 8 MB AI Passport flash", orUnknown(esp.SizeName(size)))
	}
	on(Event{Type: "identified", Chip: chip, MAC: mac, Flash: esp.SizeName(size)})
	backupCardID(d, data, mac, req.BackupDir, on)

	on(Event{Type: "stage", Stage: "writing"})
	if err = d.Write([]esp.Segment{{Offset: 0, Data: data}}, percent(on)); err != nil {
		return coded("write", "Write failed: %v", err)
	}
	on(Event{Type: "stage", Stage: "verifying"})
	if err = verify(d, []esp.Segment{{Offset: 0, Data: data}}); err != nil {
		return coded("write", "%v", err)
	}
	on(Event{Type: "progress", Percent: 100})
	on(Event{Type: "stage", Stage: "resetting"})
	if err = d.Reset(); err != nil {
		log(fmt.Sprintf("Automatic reset failed (%v); unplug and replug the device", err))
	}
	on(Event{Type: "complete", MAC: mac})
	return nil
}

// verify compares each segment with an MD5 the chip computes.
func verify(d Device, segs []esp.Segment) error {
	for _, s := range segs {
		got, err := d.MD5(s.Offset, len(s.Data))
		if err != nil {
			return fmt.Errorf("verify at 0x%x: %v", s.Offset, err)
		}
		want := md5.Sum(s.Data)
		if !strings.EqualFold(got, hex.EncodeToString(want[:])) {
			return fmt.Errorf("verify at 0x%x failed: the flash does not match", s.Offset)
		}
	}
	return nil
}

// backupCardID keeps a copy of the device identity area before a play image overwrites
// it. Community images are merged from 0x0; ones longer than 0x356000 overwrite cardid
// (the device's SN and keys). The copy lets FoloToy recover the identity later.
func backupCardID(d Device, data []byte, mac, dir string, on func(Event)) {
	if dir == "" || len(data) <= CardIDOffset {
		return
	}
	current, err := d.Read(CardIDOffset, CardIDSize, nil)
	if err != nil {
		on(Event{Type: "log", Message: clip("Could not back up cardid: " + err.Error())})
		return
	}
	status := cardIDStatus(current)
	end := min(len(data), CardIDOffset+CardIDSize)
	unchanged := bytes.Equal(data[CardIDOffset:end], current[:end-CardIDOffset]) && end == CardIDOffset+CardIDSize
	on(Event{Type: "cardid", Status: status, Overwrite: !unchanged})
	if status == "blank_ff" || status == "blank_00" || unchanged {
		return
	}
	folder := filepath.Join(dir, "cardid", strings.ReplaceAll(mac, ":", ""))
	if err = os.MkdirAll(folder, 0o755); err != nil {
		on(Event{Type: "log", Message: clip("Could not back up cardid: " + err.Error())})
		return
	}
	sum := sha256.Sum256(current)
	target := filepath.Join(folder, hex.EncodeToString(sum[:])[:16]+".bin")
	if _, err = os.Stat(target); err != nil {
		tmp := strings.TrimSuffix(target, ".bin") + ".tmp"
		if err = os.WriteFile(tmp, current, 0o644); err == nil {
			err = os.Rename(tmp, target)
		}
		if err != nil {
			on(Event{Type: "log", Message: clip("Could not back up cardid: " + err.Error())})
			return
		}
	}
	on(Event{Type: "log", Message: clip("cardid backed up to " + target)})
}

func clip(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 2000 {
		return s[:2000]
	}
	return s
}
