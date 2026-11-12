package device

import (
	"bytes"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/FoloToy/ai-passport-plays/internal/esp"
	"github.com/FoloToy/ai-passport-plays/internal/factory"
)

const testMAC = "4c:11:ae:32:62:34"

var firmwareDir = filepath.Join("..", "factory", "firmware")

// fakeDevice simulates an AI Passport with an identity area.
type fakeDevice struct {
	chip     string
	size     int
	security error
	sb, fe   bool
	flash    []byte
	calls    []string
	writes   [][]int
	onWrite  func(*fakeDevice)
	md5      func(offset, size int) string
	resetErr error
}

func newFake() *fakeDevice {
	d := &fakeDevice{chip: "ESP32-C3", size: 8 << 20, flash: bytes.Repeat([]byte{0xFF}, 8<<20)}
	copy(d.flash[CardIDOffset:], "ProductKey\x00DeviceKey\x00DeviceSecret\x00")
	return d
}

func (d *fakeDevice) Chip() (string, error) { return d.chip, nil }
func (d *fakeDevice) MAC() (string, error)  { return testMAC, nil }
func (d *fakeDevice) Security() (bool, bool, error) {
	return d.sb, d.fe, d.security
}
func (d *fakeDevice) Prepare(int) (int, error) {
	d.calls = append(d.calls, "prepare")
	return d.size, nil
}
func (d *fakeDevice) Read(offset, size int, _ func(int, int)) ([]byte, error) {
	d.calls = append(d.calls, fmt.Sprintf("read %x %x", offset, size))
	if offset+size > len(d.flash) {
		size = len(d.flash) - offset
	}
	return append([]byte(nil), d.flash[offset:offset+size]...), nil
}
func (d *fakeDevice) Write(segs []esp.Segment, _ func(int, int)) error {
	d.calls = append(d.calls, "write")
	var offs []int
	for _, s := range segs {
		offs = append(offs, s.Offset)
	}
	d.writes = append(d.writes, offs)
	if d.onWrite != nil {
		d.onWrite(d)
	}
	for _, s := range segs {
		copy(d.flash[s.Offset:], s.Data)
	}
	return nil
}
func (d *fakeDevice) MD5(offset, size int) (string, error) {
	d.calls = append(d.calls, "md5")
	if d.md5 != nil {
		return d.md5(offset, size), nil
	}
	sum := md5.Sum(d.flash[offset : offset+size])
	return hex.EncodeToString(sum[:]), nil
}
func (d *fakeDevice) Reset() error { d.calls = append(d.calls, "reset"); return d.resetErr }
func (d *fakeDevice) Close() error { return nil }

func codeOf(err error) string {
	var ce *CodedError
	if errors.As(err, &ce) {
		return ce.Code
	}
	return ""
}

func newJob(t *testing.T, d Device, mode string, cancel chan struct{}) (*restoreJob, *[]Event) {
	events := &[]Event{}
	if cancel == nil {
		cancel = make(chan struct{})
	}
	j := &restoreJob{
		mode: mode, firmwareDir: firmwareDir, backupRoot: t.TempDir(), emit: func(e Event) { *events = append(*events, e) },
		cancel: cancel, port: "COM9",
		connect: func(string, string, int, func(string)) (Device, error) { return d, nil },
	}
	return j, events
}

func stages(events []Event) []string {
	var out []string
	for _, e := range events {
		if e.Type == "stage" {
			out = append(out, e.Stage)
		}
	}
	return out
}

// --- layout and firmware

func TestFactoryFirmwareLoads(t *testing.T) {
	segs, err := loadFactory(firmwareDir)
	if err != nil || len(segs) != 4 {
		t.Fatalf("%d segments, %v", len(segs), err)
	}
}

func TestTamperedFirmwareRefused(t *testing.T) {
	dir := t.TempDir()
	for _, s := range factory.Segments {
		b, _ := os.ReadFile(filepath.Join(firmwareDir, s.File))
		_ = os.WriteFile(filepath.Join(dir, s.File), b, 0o644)
	}
	_ = os.WriteFile(filepath.Join(dir, "imgava.bin"), make([]byte, factory.Segments[3].Size), 0o644)
	if _, err := loadFactory(dir); codeOf(err) != "firmware" {
		t.Fatalf("want firmware error, got %v", err)
	}
}

func TestCardIDStatusNeverRevealsContents(t *testing.T) {
	data := bytes.Repeat([]byte{0xFF}, CardIDSize)
	copy(data, "ProductKey\x00DeviceKey\x00DeviceSecret\x00secret-value")
	for _, c := range []struct {
		in   []byte
		want string
	}{
		{data, "keys"}, {bytes.Repeat([]byte{0xFF}, CardIDSize), "blank_ff"},
		{make([]byte, CardIDSize), "blank_00"}, {[]byte("x"), "bad_length"},
	} {
		if got := cardIDStatus(c.in); got != c.want {
			t.Fatalf("got %s want %s", got, c.want)
		}
	}
}

// --- factory restore

func TestFullRestorePreservesCardID(t *testing.T) {
	d := newFake()
	cardid := append([]byte(nil), d.flash[CardIDOffset:CardIDOffset+CardIDSize]...)
	j, events := newJob(t, d, "restore", nil)
	j.fullBackup = true
	res, err := j.run()
	if err != nil {
		t.Fatal(err)
	}
	report, _ := os.ReadFile(filepath.Join(res.Session, "factory-restore-report.txt"))
	for _, want := range []string{"cardid_unchanged=yes", "erase_flash=no", "full_backup=yes"} {
		if !strings.Contains(string(report), want) {
			t.Fatalf("report lacks %s", want)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(res.Session, "cardid-before.bin")); !bytes.Equal(b, cardid) {
		t.Fatal("cardid backup differs")
	}
	if st, err := os.Stat(filepath.Join(res.Session, "full-flash-before-restore.bin")); err != nil || st.Size() != factory.FlashSize {
		t.Fatalf("full backup %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(res.Session, "factory-restore.log")); len(b) == 0 {
		t.Fatal("empty log")
	}
	if res.CardID != "keys" {
		t.Fatalf("cardid %s", res.CardID)
	}
	if !reflect.DeepEqual(d.writes, [][]int{{0x0, 0x8000, 0x10000, 0x3FA000}}) {
		t.Fatalf("writes %v", d.writes)
	}
	if d.calls[len(d.calls)-1] != "reset" {
		t.Fatalf("last call %s", d.calls[len(d.calls)-1])
	}
	want := []string{"preparing", "connecting", "checking", "backup", "writing", "verifying", "cardid", "resetting"}
	if got := stages(*events); !reflect.DeepEqual(got, want) {
		t.Fatalf("stages %v", got)
	}
	for _, s := range factory.Segments {
		b, _ := os.ReadFile(filepath.Join(firmwareDir, s.File))
		if !bytes.Equal(d.flash[s.Offset:s.Offset+s.Size], b) {
			t.Fatalf("%s not written", s.File)
		}
	}
}

func TestRestoreRefusals(t *testing.T) {
	cases := []struct {
		name string
		mod  func(*fakeDevice)
		code string
	}{
		{"wrong chip", func(d *fakeDevice) { d.chip = "ESP32-S3" }, "wrong_chip"},
		{"4MB", func(d *fakeDevice) { d.size = 4 << 20 }, "flash_size"},
		{"16MB", func(d *fakeDevice) { d.size = 16 << 20 }, "flash_size"},
		{"unknown size", func(d *fakeDevice) { d.size = 0 }, "flash_size"},
		{"secure boot", func(d *fakeDevice) { d.sb = true }, "secure"},
		{"encryption", func(d *fakeDevice) { d.fe = true }, "secure"},
		{"security unknown", func(d *fakeDevice) { d.security = errors.New("no answer") }, "secure"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := newFake()
			c.mod(d)
			j, _ := newJob(t, d, "restore", nil)
			if _, err := j.run(); codeOf(err) != c.code {
				t.Fatalf("want %s, got %v", c.code, err)
			}
			if len(d.writes) != 0 {
				t.Fatal("wrote to the device")
			}
			if c.code == "secure" && strings.Contains(strings.Join(d.calls, ","), "prepare") {
				t.Fatal("prepared after a security failure")
			}
		})
	}
}

func TestCancelBeforeWritingStops(t *testing.T) {
	d := newFake()
	cancel := make(chan struct{})
	close(cancel)
	j, _ := newJob(t, d, "restore", cancel)
	if _, err := j.run(); codeOf(err) != "cancelled" {
		t.Fatalf("want cancelled, got %v", err)
	}
	if len(d.writes) != 0 {
		t.Fatal("wrote after cancel")
	}
}

func TestCancelAfterWritingStartedIsIgnored(t *testing.T) {
	cancel := make(chan struct{})
	d := newFake()
	d.onWrite = func(*fakeDevice) { close(cancel) }
	j, _ := newJob(t, d, "restore", cancel)
	res, err := j.run()
	if err != nil || res.MAC != testMAC {
		t.Fatalf("%+v %v", res, err)
	}
	if !strings.Contains(strings.Join(d.calls, ","), "write,md5") {
		t.Fatalf("not verified: %v", d.calls)
	}
}

func TestCardIDChangeIsReported(t *testing.T) {
	d := newFake()
	d.onWrite = func(d *fakeDevice) { d.flash[CardIDOffset] ^= 0xFF }
	j, _ := newJob(t, d, "restore", nil)
	if _, err := j.run(); codeOf(err) != "cardid_changed" {
		t.Fatalf("want cardid_changed, got %v", err)
	}
}

// --- firmware backup

func TestBackupSavesWholeFlashAndNeverWrites(t *testing.T) {
	d := newFake()
	copy(d.flash, []byte{0xE9, 0x03, 0x02, 0x3F})
	j, events := newJob(t, d, "backup", nil)
	j.firmwareDir = ""
	res, err := j.run()
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(res.File); !bytes.Equal(b, d.flash) {
		t.Fatal("backup differs from flash")
	}
	if _, err = os.Stat(res.File + ".sha256"); err != nil {
		t.Fatal(err)
	}
	report, _ := os.ReadFile(filepath.Join(res.Session, "firmware-backup-report.txt"))
	for _, want := range []string{"written_to_device=no", "md5_device_match=yes"} {
		if !strings.Contains(string(report), want) {
			t.Fatalf("report lacks %s", want)
		}
	}
	if res.CardID != "keys" || len(d.writes) != 0 {
		t.Fatalf("cardid %s writes %v", res.CardID, d.writes)
	}
	want := []string{"preparing", "connecting", "checking", "backup", "verifying", "resetting"}
	if got := stages(*events); !reflect.DeepEqual(got, want) {
		t.Fatalf("stages %v", got)
	}
}

func TestBackupNeedsNoFactoryFirmware(t *testing.T) {
	d := newFake()
	d.size, d.flash = 4<<20, d.flash[:4<<20]
	j, _ := newJob(t, d, "backup", nil)
	j.firmwareDir = ""
	res, err := j.run()
	if err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Stat(res.File); st.Size() != 4<<20 {
		t.Fatalf("size %d", st.Size())
	}
}

func TestBackupMismatchIsReported(t *testing.T) {
	d := newFake()
	d.md5 = func(int, int) string { return strings.Repeat("0", 32) }
	j, _ := newJob(t, d, "backup", nil)
	if _, err := j.run(); codeOf(err) != "backup" {
		t.Fatalf("want backup, got %v", err)
	}
}

func TestBackupRefusesOtherChips(t *testing.T) {
	d := newFake()
	d.chip = "ESP32-S3"
	j, _ := newJob(t, d, "backup", nil)
	if _, err := j.run(); codeOf(err) != "wrong_chip" {
		t.Fatalf("want wrong_chip, got %v", err)
	}
}

// --- automatic capture

func cand(dev, serial string) Candidate {
	return Candidate{Device: dev, Description: "USB JTAG/serial debug unit", VID: EspressifVID, PID: USBJTAGPID, SerialNumber: serial}
}

type connectCall struct{ port, mode string }

func autoJob(t *testing.T, provider func() []Candidate, capture func(Candidate) bool, hints func() []string, timeout time.Duration) (*restoreJob, *[]connectCall) {
	calls := &[]connectCall{}
	j, _ := newJob(t, newFake(), "restore", nil)
	j.port = ""
	j.connect = func(port, mode string, _ int, _ func(string)) (Device, error) {
		*calls = append(*calls, connectCall{port, mode})
		return newFake(), nil
	}
	j.candidates = func() ([]Candidate, error) { return provider(), nil }
	j.capture = capture
	j.hints = hints
	if j.hints == nil {
		j.hints = func() []string { return nil }
	}
	j.timeout, j.interval = timeout, time.Millisecond
	return j, calls
}

func TestUSBIdentitySurvivesPortRename(t *testing.T) {
	if cand("COM7", "ABC123").TrackingKey() != cand("COM21", "ABC123").TrackingKey() {
		t.Fatal("tracking key depends on the port name")
	}
}

func TestShortUSBWindowIsCapturedThenROMPortProbed(t *testing.T) {
	polls, captured := 0, false
	var seen []string
	j, calls := autoJob(t, func() []Candidate {
		polls++
		if captured {
			return []Candidate{cand("COM21", "ABC123")}
		}
		if polls < 4 {
			return nil
		}
		return []Candidate{cand("COM47", "ABC123")}
	}, func(c Candidate) bool { seen = append(seen, c.Device); captured = true; return true }, nil, 5*time.Second)
	if _, err := j.autoConnect(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(seen, []string{"COM47"}) || !reflect.DeepEqual(*calls, []connectCall{{"COM21", "no-reset"}}) || j.port != "COM21" {
		t.Fatalf("captured %v connected %v port %s", seen, *calls, j.port)
	}
}

func TestCode43PortIsOpenedWithoutEnumeration(t *testing.T) {
	captured := false
	var opened []string
	j, _ := autoJob(t, func() []Candidate {
		if captured {
			return []Candidate{cand("COM47", "")}
		}
		return nil
	}, func(c Candidate) bool { opened = append(opened, c.Device); captured = true; return true },
		func() []string { return []string{"COM47"} }, 5*time.Second)
	if _, err := j.autoConnect(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(opened, []string{"COM47"}) {
		t.Fatalf("opened %v", opened)
	}
}

func TestMultipleDevicesAreNeverWritten(t *testing.T) {
	j, calls := autoJob(t, func() []Candidate { return []Candidate{cand("COM3", ""), cand("COM4", "X")} },
		func(Candidate) bool { return true }, nil, 50*time.Millisecond)
	if _, err := j.autoConnect(); codeOf(err) != "not_found" {
		t.Fatalf("want not_found, got %v", err)
	}
	if len(*calls) != 0 {
		t.Fatalf("connected %v", *calls)
	}
}

// --- installing a play

func writeImage(t *testing.T, data []byte) (string, string) {
	path := filepath.Join(t.TempDir(), "firmware.bin")
	_ = os.WriteFile(path, data, 0o644)
	sum := sha256.Sum256(data)
	return path, hex.EncodeToString(sum[:])
}

// playImage builds a minimal valid ESP32-C3 image with a correct bootloader checksum.
func playImage(size int) []byte {
	data := bytes.Repeat([]byte{0xFF}, size)
	hdr := []byte{0xE9, 1, 2, 0x2F, 0, 0, 0, 0, 0xEE, 0, 0, 0, 5, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	copy(data, hdr)
	seg := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	copy(data[24:], []byte{0, 0, 0, 0x3F, byte(len(seg)), 0, 0, 0})
	copy(data[32:], seg)
	chk := byte(0xEF)
	for _, b := range seg {
		chk ^= b
	}
	pos := 40
	pos += 15 - pos%16
	data[pos] = chk
	return data
}

func flashWith(t *testing.T, d *fakeDevice, data []byte, backupDir string) ([]Event, error) {
	path, sum := writeImage(t, data)
	var events []Event
	connected := false
	err := flashImage(t.Context(), func(string, string, int, func(string)) (Device, error) { connected = true; return d, nil },
		Request{Port: "/dev/null", Image: path, SHA256: sum, BackupDir: backupDir}, func(e Event) { events = append(events, e) })
	if !connected && err == nil {
		t.Fatal("never connected")
	}
	return events, err
}

func TestChecksumFailurePrecedesSerialAccess(t *testing.T) {
	path, _ := writeImage(t, append([]byte{0xE9}, make([]byte, 63)...))
	err := flashImage(t.Context(), func(string, string, int, func(string)) (Device, error) {
		t.Fatal("connected before validating")
		return nil, nil
	}, Request{Image: path, SHA256: strings.Repeat("0", 64)}, func(Event) {})
	if codeOf(err) != "checksum" {
		t.Fatalf("want checksum, got %v", err)
	}
}

func TestImageValidation(t *testing.T) {
	s3 := make([]byte, 64)
	s3[0], s3[12] = 0xE9, 0x09
	path, sum := writeImage(t, s3)
	if _, err := validateImage(Request{Image: path, SHA256: sum}); codeOf(err) != "image_chip" {
		t.Fatalf("want image_chip, got %v", err)
	}
	big := append([]byte{0xE9}, make([]byte, maxImage)...)
	path, sum = writeImage(t, big)
	if _, err := validateImage(Request{Image: path, SHA256: sum}); codeOf(err) != "image" {
		t.Fatalf("want image, got %v", err)
	}
	bad := playImage(4096)
	bad[33] ^= 1 // corrupt a bootloader byte
	path, sum = writeImage(t, bad)
	if _, err := validateImage(Request{Image: path, SHA256: sum}); codeOf(err) != "image" {
		t.Fatalf("want bootloader checksum error, got %v", err)
	}
	path, sum = writeImage(t, playImage(4096))
	if _, err := validateImage(Request{Image: path, SHA256: sum, Size: 4096}); err != nil {
		t.Fatal(err)
	}
}

func TestBundledBootloaderChecksumValid(t *testing.T) {
	b, _ := os.ReadFile(filepath.Join(firmwareDir, "bootloader.bin"))
	if err := checkBootloader(b); err != nil {
		t.Fatal(err)
	}
}

func TestFlashRefusals(t *testing.T) {
	d := newFake()
	d.chip = "ESP32-S3"
	if _, err := flashWith(t, d, playImage(4096), ""); codeOf(err) != "wrong_chip" || len(d.writes) != 0 {
		t.Fatalf("got %v", err)
	}
	d = newFake()
	d.size = 4 << 20
	if _, err := flashWith(t, d, playImage(4096), ""); codeOf(err) != "flash_size" || len(d.writes) != 0 {
		t.Fatalf("got %v", err)
	}
}

func TestFlashHappyPath(t *testing.T) {
	d := newFake()
	img := playImage(4096)
	events, err := flashWith(t, d, img, "")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(d.writes, [][]int{{0}}) || !bytes.Equal(d.flash[:4096], img) {
		t.Fatalf("writes %v", d.writes)
	}
	if !strings.Contains(strings.Join(d.calls, ","), "write,md5,reset") {
		t.Fatalf("calls %v", d.calls)
	}
	if last := events[len(events)-1]; last.Type != "complete" || last.MAC != testMAC {
		t.Fatalf("last event %+v", last)
	}
	if got := stages(events); !reflect.DeepEqual(got, []string{"connecting", "writing", "verifying", "resetting"}) {
		t.Fatalf("stages %v", got)
	}
}

func TestCardIDSavedBeforeOverwrite(t *testing.T) {
	d := newFake()
	img := playImage(CardIDOffset + CardIDSize + 16)
	dir := t.TempDir()
	events, err := flashWith(t, d, img, dir)
	if err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "cardid", "4c11ae326234", "*.bin"))
	if len(files) != 1 {
		t.Fatalf("files %v", files)
	}
	found := false
	for _, e := range events {
		if e.Type == "cardid" && e.Status == "keys" && e.Overwrite {
			found = true
		}
	}
	if !found {
		t.Fatal("no cardid event")
	}
}

func TestShortImageOrBlankIdentityIsSkipped(t *testing.T) {
	d := newFake()
	copy(d.flash[CardIDOffset:], bytes.Repeat([]byte{0xFF}, CardIDSize))
	img := playImage(CardIDOffset + CardIDSize + 16)
	copy(img[CardIDOffset:], make([]byte, CardIDSize))
	dir := t.TempDir()
	if _, err := flashWith(t, d, img, dir); err != nil {
		t.Fatal(err)
	}
	if files, _ := filepath.Glob(filepath.Join(dir, "cardid", "*", "*.bin")); len(files) != 0 {
		t.Fatalf("files %v", files)
	}
	d = newFake()
	if _, err := flashWith(t, d, playImage(1024), "/nonexistent"); err != nil {
		t.Fatal(err)
	}
	for _, c := range d.calls {
		if strings.HasPrefix(c, "read") {
			t.Fatal("read cardid for a short image")
		}
	}
}

func TestRunnerRestoreDeliversCancel(t *testing.T) {
	cancel := make(chan struct{})
	r := Runner{
		List:    func() ([]Candidate, error) { return nil, nil },
		Connect: func(string, string, int, func(string)) (Device, error) { return nil, errors.New("unused") },
	}
	err := r.Restore(t.Context(), Request{FirmwareDir: firmwareDir, BackupDir: t.TempDir(), FullBackup: true}, func(ev Event) {
		if ev.Type == "stage" && ev.Stage == "waiting" {
			close(cancel)
		}
	}, cancel)
	if codeOf(err) != "cancelled" {
		t.Fatalf("want cancelled, got %v", err)
	}
}

func TestRestartIsReported(t *testing.T) {
	for _, stuck := range []bool{false, true} {
		d := newFake()
		if stuck {
			d.resetErr = ErrNotRestarted
		}
		events, err := flashWith(t, d, playImage(4096), "")
		if err != nil {
			t.Fatal(err) // a missed restart never fails a verified install
		}
		if last := events[len(events)-1]; last.Type != "complete" || last.Restarted == stuck {
			t.Fatalf("stuck=%v: complete %+v", stuck, last)
		}
		d = newFake()
		if stuck {
			d.resetErr = ErrNotRestarted
		}
		j, _ := newJob(t, d, "restore", nil)
		res, err := j.run()
		if err != nil || res.Restarted == stuck {
			t.Fatalf("stuck=%v: restore %+v %v", stuck, res, err)
		}
	}
}

func TestFlashRefusesMissingPort(t *testing.T) {
	connected := false
	r := Runner{
		List:    func() ([]Candidate, error) { return []Candidate{cand("/dev/cu.other", "")}, nil },
		Connect: func(string, string, int, func(string)) (Device, error) { connected = true; return newFake(), nil },
	}
	err := r.Flash(t.Context(), Request{Port: "/dev/cu.gone"}, func(Event) {})
	if codeOf(err) != "no_device" || connected {
		t.Fatalf("want no_device without connecting, got %v", err)
	}
}

func TestWaitingSaysWhenNothingWasEverSeen(t *testing.T) {
	oldEvery, oldAfter := noticeEvery, notSeenAfter
	noticeEvery, notSeenAfter = 10*time.Millisecond, 30*time.Millisecond
	defer func() { noticeEvery, notSeenAfter = oldEvery, oldAfter }()
	j, _ := autoJob(t, func() []Candidate { return nil }, func(Candidate) bool { return true }, nil, 150*time.Millisecond)
	var codes []string
	j.emit = func(e Event) {
		if e.Type == "hint" {
			codes = append(codes, e.Code)
		}
	}
	if _, err := j.autoConnect(); codeOf(err) != "not_found" {
		t.Fatalf("want not_found, got %v", err)
	}
	if len(codes) == 0 || codes[0] != "replug" || codes[len(codes)-1] != "not_seen" {
		t.Fatalf("hints %v", codes)
	}
}
