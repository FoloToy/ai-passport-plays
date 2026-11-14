package device

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/FoloToy/ai-passport-desktop/pkg/esp"
	"github.com/FoloToy/ai-passport-desktop/pkg/factory"
)

// Factory restore, ported from FoloToy-AI-Passport-Factory-Restore v1.5.2.
//
// Safety rules carried over unchanged:
//   - write the four factory segments separately; never erase the chip, never write cardid;
//   - refuse anything that is not an ESP32-C3 with exactly 8 MB flash and security disabled;
//   - back up the partition table and cardid (and by default the whole flash) before writing;
//   - verify every segment, then require cardid to be byte-for-byte unchanged;
//   - once writing starts, a cancel request is ignored until the job finishes.
//
// All device work runs over one connection, so the port cannot change between steps.

const (
	CardIDOffset    = factory.CardIDOffset
	CardIDSize      = factory.CardIDSize
	partitionOffset = 0x8000
	partitionSize   = 0x1000
	connectTimeout  = 150 * time.Second
	scanInterval    = 15 * time.Millisecond
	romProbe        = 4 * time.Second
)

// cardIDStatus describes the identity area without exposing any of its contents.
func cardIDStatus(data []byte) string {
	switch {
	case len(data) != CardIDSize:
		return "bad_length"
	case bytes.Count(data, []byte{0xFF}) == CardIDSize:
		return "blank_ff"
	case bytes.Count(data, []byte{0x00}) == CardIDSize:
		return "blank_00"
	case bytes.Contains(data, []byte("ProductKey")) && bytes.Contains(data, []byte("DeviceKey")) && bytes.Contains(data, []byte("DeviceSecret")):
		return "keys"
	}
	return "unknown"
}

// loadFactory reads and checks the four factory segments from dir.
func loadFactory(dir string) ([]esp.Segment, error) {
	if err := factory.CheckLayout(factory.Segments); err != nil {
		return nil, coded("firmware", "%v", err)
	}
	segs := make([]esp.Segment, 0, len(factory.Segments))
	for _, s := range factory.Segments {
		b, err := os.ReadFile(filepath.Join(dir, s.File))
		if err != nil {
			return nil, coded("firmware", "Factory firmware file missing: %s", s.File)
		}
		sum := sha256.Sum256(b)
		if int64(len(b)) != s.Size || hex.EncodeToString(sum[:]) != s.SHA256 {
			return nil, coded("firmware", "Factory firmware failed its check: %s", s.File)
		}
		segs = append(segs, esp.Segment{Offset: int(s.Offset), Data: b})
	}
	return segs, nil
}

type restoreResult struct {
	MAC, Session, CardID, File string
	Restarted                  bool
}

type restoreJob struct {
	mode        string // "restore" or "backup" (read only)
	firmwareDir string
	backupRoot  string
	fullBackup  bool
	emit        func(Event)
	cancel      <-chan struct{}
	ctx         context.Context
	port        string
	connect     Connector
	candidates  func() ([]Candidate, error)
	capture     func(Candidate) bool
	hints       func() []string
	timeout     time.Duration
	interval    time.Duration
	// freeSpace is replaced in tests.
	freeSpace func(string) (uint64, error)

	writeStarted bool
	logPath      string
	lines        []string
}

// raw sends a line to the UI and appends it to the log file in the backup folder.
func (j *restoreJob) raw(msg string) {
	msg = strings.TrimRight(msg, " \r\n\t")
	if msg == "" {
		return
	}
	j.lines = append(j.lines, msg)
	j.emit(Event{Type: "log", Message: clip(msg)})
	if j.logPath != "" {
		if f, err := os.OpenFile(j.logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
			_, _ = f.WriteString(msg + "\n")
			_ = f.Close()
		}
	}
}

func (j *restoreJob) log(format string, args ...any) {
	j.raw("[" + time.Now().Format("15:04:05") + "] " + fmt.Sprintf(format, args...))
}

func (j *restoreJob) stage(name string) { j.emit(Event{Type: "stage", Stage: name}) }

func (j *restoreJob) checkCancel() error {
	if j.writeStarted {
		return nil
	}
	select {
	case <-j.cancel:
		return coded("cancelled", "Stopped before anything was written")
	default:
	}
	if j.ctx != nil && j.ctx.Err() != nil {
		return coded("cancelled", "Stopped before anything was written")
	}
	return nil
}

func (j *restoreJob) run() (restoreResult, error) {
	if err := os.MkdirAll(j.backupRoot, 0o755); err != nil {
		return restoreResult{}, coded("backup", "%v", err)
	}
	j.logPath = filepath.Join(j.backupRoot, "last-connection.log")
	_ = os.WriteFile(j.logPath, nil, 0o644)
	j.stage("preparing")
	var segs []esp.Segment
	if j.mode == "restore" {
		j.log("Factory firmware %s", factory.Version)
		var err error
		if segs, err = loadFactory(j.firmwareDir); err != nil {
			return restoreResult{}, err
		}
		j.log("Factory firmware size, SHA-256 and cardid layout checked")
	} else {
		j.log("Firmware backup (read only; nothing is written to the device)")
	}

	var d Device
	var err error
	if j.port != "" {
		j.stage("connecting")
		d, err = j.open(j.port, "default-reset", 10)
	} else {
		d, err = j.autoConnect()
	}
	if err != nil {
		return restoreResult{}, err
	}
	defer d.Close()
	if j.ctx != nil {
		defer closeOnDone(j.ctx, d)()
	}
	if j.mode == "backup" {
		return j.backup(d)
	}
	return j.restore(d, segs)
}

func (j *restoreJob) open(port, mode string, attempts int) (Device, error) {
	d, err := j.connect(port, mode, attempts, j.raw)
	if err != nil {
		if ce, ok := err.(*CodedError); ok {
			return nil, ce
		}
		return nil, coded("connect", "%v", err)
	}
	return d, nil
}

// fail wraps a device error: "write" once writing started, "connect" before.
func (j *restoreJob) fail(what string, err error) error {
	if ce, ok := err.(*CodedError); ok {
		return ce
	}
	code := "connect"
	if j.writeStarted {
		code = "write"
	}
	return coded(code, "%s failed: %v", what, err)
}

func (j *restoreJob) identify(d Device) (string, string, error) {
	if err := j.checkCancel(); err != nil {
		return "", "", err
	}
	chip, err := d.Chip()
	if err != nil {
		return "", "", j.fail("identify", err)
	}
	if !isESP32C3(chip) {
		return "", "", coded("wrong_chip", "Detected %s; AI Passport uses ESP32-C3", chip)
	}
	mac, err := d.MAC()
	if err != nil {
		return "", "", j.fail("read MAC", err)
	}
	j.log("ESP32-C3 locked on %s, MAC %s", j.port, mac)
	return chip, mac, nil
}

func (j *restoreJob) session(name, logName string) (string, error) {
	dir := filepath.Join(j.backupRoot, name)
	if err := os.Mkdir(dir, 0o755); err != nil {
		return "", coded("backup", "%v", err)
	}
	j.logPath = filepath.Join(dir, logName)
	_ = os.WriteFile(j.logPath, []byte(strings.Join(j.lines, "\n")+"\n"), 0o644)
	j.log("Backup folder: %s", dir)
	return dir, nil
}

func (j *restoreJob) free(dir string) uint64 {
	fn := j.freeSpace
	if fn == nil {
		fn = diskFree
	}
	n, err := fn(dir)
	if err != nil {
		return ^uint64(0) // unknown: do not block the job
	}
	return n
}

func (j *restoreJob) restore(d Device, segs []esp.Segment) (restoreResult, error) {
	chip, mac, err := j.identify(d)
	if err != nil {
		return restoreResult{}, err
	}
	session, err := j.session(time.Now().Format("20060102-150405")+"-"+strings.ReplaceAll(mac, ":", ""), "factory-restore.log")
	if err != nil {
		return restoreResult{}, err
	}

	j.stage("checking")
	secureBoot, encryption, err := d.Security()
	if err != nil {
		return restoreResult{}, coded("secure", "Could not confirm the security settings: %v", err)
	}
	if secureBoot || encryption {
		return restoreResult{}, coded("secure", "Secure Boot or Flash Encryption is enabled; refusing to write")
	}
	j.log("Secure Boot and Flash Encryption are both disabled")
	size, err := d.Prepare(defaultBaud)
	if err != nil {
		return restoreResult{}, j.fail("prepare", err)
	}
	if size != factory.FlashSize {
		return restoreResult{}, coded("flash_size", "The factory firmware needs exactly 8 MB flash, found %s", orUnknown(esp.SizeName(size)))
	}
	j.emit(Event{Type: "identified", Chip: chip, MAC: mac, Flash: esp.SizeName(size)})
	j.log("8 MB flash confirmed")

	if err = j.checkCancel(); err != nil {
		return restoreResult{}, err
	}
	j.stage("backup")
	j.log("Backing up the partition table")
	if _, err = j.readTo(d, partitionOffset, partitionSize, filepath.Join(session, "partition-table-before.bin")); err != nil {
		return restoreResult{}, err
	}
	j.log("Backing up the cardid identity area")
	cardid, err := j.readTo(d, CardIDOffset, CardIDSize, filepath.Join(session, "cardid-before.bin"))
	if err != nil {
		return restoreResult{}, err
	}
	cardidSum := sha256.Sum256(cardid)
	cardidHash := hex.EncodeToString(cardidSum[:])
	status := cardIDStatus(cardid)
	j.log("cardid SHA-256 %s, status %s", cardidHash, status)

	full := false
	if j.fullBackup {
		if j.free(session) < 32<<20 {
			return restoreResult{}, coded("disk_space", "Less than 32 MB free in the backup folder")
		}
		j.log("Backing up the whole 8 MB flash; this takes a few minutes")
		b, err := j.readTo(d, 0, factory.FlashSize, filepath.Join(session, "full-flash-before-restore.bin"))
		if err != nil {
			return restoreResult{}, err
		}
		sum := sha256.Sum256(b)
		j.log("Full backup SHA-256 %s", hex.EncodeToString(sum[:]))
		full = true
	}

	if err = j.checkCancel(); err != nil {
		return restoreResult{}, err
	}
	j.writeStarted = true
	j.stage("writing")
	j.log("Writing the four factory segments (no erase_flash, cardid untouched)")
	if err = d.Write(segs, percent(j.emit)); err != nil {
		return restoreResult{}, j.fail("write", err)
	}
	j.stage("verifying")
	j.log("Verifying every segment")
	if err = verify(d, segs); err != nil {
		return restoreResult{}, j.fail("verify", err)
	}

	j.stage("cardid")
	after, err := j.readTo(d, CardIDOffset, CardIDSize, filepath.Join(session, "cardid-after.bin"))
	if err != nil {
		return restoreResult{}, err
	}
	if !bytes.Equal(after, cardid) {
		return restoreResult{}, coded("cardid_changed", "cardid changed during the restore; stop using this device and contact FoloToy")
	}
	j.log("cardid is byte-for-byte unchanged")

	report := []string{
		"tool=AI Passport Plays factory restore",
		"factory_firmware=" + factory.Version,
		"time=" + time.Now().Format("2006-01-02T15:04:05"),
		"port=" + j.port,
		"chip=esp32c3",
		"mac=" + mac,
		"flash_size=8MB",
		"secure_boot=disabled",
		"flash_encryption=disabled",
		"cardid_sha256=" + cardidHash,
		"cardid_status=" + status,
		"full_backup=" + yesNo(full),
		"erase_flash=no",
		"verify_flash=passed",
		"cardid_unchanged=yes",
		"result=success",
	}
	_ = os.WriteFile(filepath.Join(session, "factory-restore-report.txt"), []byte(strings.Join(report, "\n")+"\n"), 0o644)

	j.stage("resetting")
	restarted := d.Reset() == nil
	if !restarted {
		j.log("Firmware verified, but the device did not restart by itself; unplug and replug USB")
	}
	j.log("Factory restore finished: firmware verified, cardid unchanged")
	return restoreResult{MAC: mac, Session: session, CardID: status, Restarted: restarted}, nil
}

// backup saves the device's whole flash, checked against an MD5 computed on the device.
func (j *restoreJob) backup(d Device) (restoreResult, error) {
	chip, mac, err := j.identify(d)
	if err != nil {
		return restoreResult{}, err
	}
	stamp := time.Now().Format("20060102-150405")
	compact := strings.ReplaceAll(mac, ":", "")
	session, err := j.session(stamp+"-"+compact+"-firmware-backup", "firmware-backup.log")
	if err != nil {
		return restoreResult{}, err
	}

	j.stage("checking")
	security := ""
	secureBoot, encryption, err := d.Security()
	if err != nil {
		encryption, security = false, fmt.Sprintf("unknown (%v)", err)
	} else {
		security = fmt.Sprintf("secure_boot=%s, flash_encryption=%s", enabled(secureBoot), enabled(encryption))
	}
	if encryption {
		j.log("Flash Encryption is enabled; the backup holds encrypted data and only fits this device")
	}
	size, err := d.Prepare(defaultBaud)
	if err != nil {
		return restoreResult{}, j.fail("prepare", err)
	}
	sizeText := esp.SizeName(size)
	if size == 0 {
		return restoreResult{}, coded("flash_size", "Could not detect the flash size")
	}
	j.emit(Event{Type: "identified", Chip: chip, MAC: mac, Flash: sizeText})

	if err = j.checkCancel(); err != nil {
		return restoreResult{}, err
	}
	j.stage("backup")
	name := fmt.Sprintf("AI-Passport-%s-%s-full-flash.bin", compact, stamp)
	path := filepath.Join(session, name)
	j.log("Reading the whole %s flash; this takes a few minutes", sizeText)
	if j.free(session) < uint64(size)+32<<20 {
		return restoreResult{}, coded("disk_space", "Not enough free space in the backup folder")
	}
	data, err := j.readTo(d, 0, size, path)
	if err != nil {
		return restoreResult{}, err
	}

	j.stage("verifying")
	local := md5.Sum(data)
	remote, err := d.MD5(0, size)
	if err != nil {
		return restoreResult{}, coded("backup", "Could not check the backup against the device: %v", err)
	}
	if !strings.EqualFold(remote, hex.EncodeToString(local[:])) {
		return restoreResult{}, coded("backup", "The saved backup does not match the device; try again")
	}
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	cardid := "bad_length"
	if size >= CardIDOffset+CardIDSize {
		cardid = cardIDStatus(data[CardIDOffset : CardIDOffset+CardIDSize])
	}
	_ = os.WriteFile(path+".sha256", []byte(digest+"  "+name+"\n"), 0o644)
	report := []string{
		"tool=AI Passport Plays firmware backup",
		"time=" + time.Now().Format("2006-01-02T15:04:05"),
		"port=" + j.port,
		"chip=esp32c3",
		"mac=" + mac,
		"flash_size=" + sizeText,
		"security=" + security,
		"file=" + name,
		"sha256=" + digest,
		"md5_device_match=yes",
		"cardid_status=" + cardid,
		"written_to_device=no",
		"result=success",
	}
	_ = os.WriteFile(filepath.Join(session, "firmware-backup-report.txt"), []byte(strings.Join(report, "\n")+"\n"), 0o644)
	j.log("Backup saved and checked against the device: %s", path)

	j.stage("resetting")
	restarted := d.Reset() == nil
	if !restarted {
		j.log("Backup finished, but the device did not restart by itself; unplug and replug USB")
	}
	return restoreResult{MAC: mac, Session: session, CardID: cardid, File: path, Restarted: restarted}, nil
}

// readTo reads a flash range and saves it to path.
func (j *restoreJob) readTo(d Device, offset, size int, path string) ([]byte, error) {
	var progress func(int, int)
	if size > partitionSize*4 {
		progress = percent(j.emit)
	}
	data, err := d.Read(offset, size, progress)
	if err != nil {
		return nil, j.fail("read", err)
	}
	if len(data) != size {
		return nil, coded("backup", "Backup %s has %d of %d bytes", filepath.Base(path), len(data), size)
	}
	if err = os.WriteFile(path, data, 0o644); err != nil {
		return nil, coded("backup", "%v", err)
	}
	return data, nil
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func enabled(b bool) string {
	if b {
		return "enabled"
	}
	return "disabled"
}
