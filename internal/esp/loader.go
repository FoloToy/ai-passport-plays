package esp

import (
	"bytes"
	"crypto/md5"
	_ "embed"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Commands understood by the ROM loader and the stub.
const (
	opFlashBegin     = 0x02
	opMemBegin       = 0x05
	opMemEnd         = 0x06
	opMemData        = 0x07
	opSync           = 0x08
	opWriteReg       = 0x09
	opReadReg        = 0x0A
	opSPISetParams   = 0x0B
	opSPIAttach      = 0x0D
	opChangeBaud     = 0x0F
	opFlashDeflBegin = 0x10
	opFlashDeflData  = 0x11
	opFlashDeflEnd   = 0x12
	opFlashMD5       = 0x13
	opSecurityInfo   = 0x14
	opReadFlash      = 0xD2
)

const (
	defaultTimeout   = 3 * time.Second
	syncTimeout      = 100 * time.Millisecond
	memEndROMTimeout = 200 * time.Millisecond
	maxTimeout       = 240 * time.Second
	md5PerMB         = 8 * time.Second
	writePerMB       = 40 * time.Second
	writeAttempts    = 3
	ramBlock         = 0x1800
	stubWriteBlock   = 0x4000
	sectorSize       = 0x1000
	romInvalidRecv   = 0x05
	checksumSeed     = 0xEF
)

// ESP32-C3 registers.
const (
	efuseBase         = 0x60008800
	macReg            = efuseBase + 0x044
	secureBootReg     = efuseBase + 0x038
	secureBootMask    = 1 << 20
	cryptCntReg       = efuseBase + 0x034
	cryptCntMask      = 0x7 << 18
	rtcWDTProtect     = 0x600080A8
	rtcWDTConfig0     = 0x60008090
	rtcWDTKey         = 0x50D83AA1
	rtcSWDProtect     = 0x600080B0
	rtcSWDConf        = 0x600080AC
	rtcSWDKey         = 0x8F1D312A
	rtcSWDAutoFeed    = 1 << 31
	spiBase           = 0x60002000
	spiCmdReg         = spiBase + 0x00
	spiUsrReg         = spiBase + 0x18
	spiUsr2Reg        = spiBase + 0x20
	spiMISODLenReg    = spiBase + 0x28
	spiW0Reg          = spiBase + 0x58
	spiCmdUsr         = 1 << 18
	spiUsrCommand     = 1 << 31
	spiUsrMISO        = 1 << 28
	chipMagicReg      = 0x40001000
	spiFlashRDID      = 0x9F
	chipIDESP32C3     = 5
	securityInfoBytes = 20
)

var chipIDs = map[uint32]string{
	2: "ESP32-S2", 5: "ESP32-C3", 9: "ESP32-S3", 12: "ESP32-C2", 13: "ESP32-C6",
	16: "ESP32-H2", 18: "ESP32-P4", 20: "ESP32-C61", 23: "ESP32-C5",
}

var chipMagics = map[uint32]string{
	0x00F01D83: "ESP32", 0x000007C6: "ESP32-S2", 0xFFF0C101: "ESP8266",
	0x6921506F: "ESP32-C3", 0x1B31506F: "ESP32-C3", 0x4881606F: "ESP32-C3", 0x4361606F: "ESP32-C3",
	0x6F51306F: "ESP32-C2", 0x7C41A06F: "ESP32-C2", 0x2CE0806F: "ESP32-C6", 0xD7B73E80: "ESP32-H2",
}

var flashSizes = map[uint32]int{
	0x12: 256 << 10, 0x13: 512 << 10, 0x14: 1 << 20, 0x15: 2 << 20, 0x16: 4 << 20, 0x17: 8 << 20,
	0x18: 16 << 20, 0x19: 32 << 20, 0x1A: 64 << 20, 0x1B: 128 << 20, 0x1C: 256 << 20,
	0x20: 64 << 20, 0x21: 128 << 20, 0x22: 256 << 20,
	0x32: 256 << 10, 0x33: 512 << 10, 0x34: 1 << 20, 0x35: 2 << 20, 0x36: 4 << 20, 0x37: 8 << 20,
	0x38: 16 << 20, 0x39: 32 << 20, 0x3A: 64 << 20,
}

//go:embed stub/esp32c3.json
var stubJSON []byte

type stubImage struct {
	Entry     uint32 `json:"entry"`
	Text      []byte `json:"text"`
	TextStart uint32 `json:"text_start"`
	Data      []byte `json:"data"`
	DataStart uint32 `json:"data_start"`
}

// StubSize reports the size of the embedded flasher stub (used by the self-test).
func StubSize() (int, error) {
	var s stubImage
	if err := json.Unmarshal(stubJSON, &s); err != nil {
		return 0, err
	}
	return len(s.Text) + len(s.Data), nil
}

// ErrTimeout means the chip did not answer in time.
var ErrTimeout = errors.New("timed out waiting for the chip")

// CommandError is a failure status returned by the chip.
type CommandError struct {
	What   string
	Status [2]byte
}

func (e *CommandError) Error() string {
	return fmt.Sprintf("failed to %s (status %02x %02x)", e.What, e.Status[0], e.Status[1])
}

// Loader is one connection to a chip.
type Loader struct {
	port Port
	// Log receives progress messages; it may be nil.
	Log func(string)
	// USB is true for the chip's own USB-JTAG/Serial port (VID 303A PID 1001).
	USB bool

	raw          []byte
	inFrame      bool
	escape       bool
	frame        []byte
	stub         bool
	stubDetected bool
	flashSize    int
}

// New wraps an open port.
func New(p Port) *Loader { return &Loader{port: p} }

func (l *Loader) Port() Port { return l.port }

// Close releases the port.
func (l *Loader) Close() error { return l.port.Close() }

func (l *Loader) logf(format string, args ...any) {
	if l.Log != nil {
		l.Log(fmt.Sprintf(format, args...))
	}
}

func (l *Loader) flushInput() {
	_ = l.port.ResetInputBuffer()
	l.raw, l.frame, l.inFrame, l.escape = nil, nil, false, false
}

// readPacket returns the next SLIP frame.
func (l *Loader) readPacket(timeout time.Duration) ([]byte, error) {
	deadline := time.Now().Add(timeout)
	buf := make([]byte, 8192)
	for {
		for len(l.raw) > 0 {
			b := l.raw[0]
			l.raw = l.raw[1:]
			if !l.inFrame {
				if b == 0xC0 {
					l.inFrame, l.frame = true, l.frame[:0]
				}
				continue // bytes between frames are boot messages or noise
			}
			switch {
			case l.escape:
				l.escape = false
				switch b {
				case 0xDC:
					l.frame = append(l.frame, 0xC0)
				case 0xDD:
					l.frame = append(l.frame, 0xDB)
				default:
					l.inFrame = false
					return nil, fmt.Errorf("invalid SLIP escape 0xdb 0x%02x", b)
				}
			case b == 0xDB:
				l.escape = true
			case b == 0xC0:
				if len(l.frame) == 0 { // back-to-back delimiters: treat as a new start
					continue
				}
				l.inFrame = false
				return append([]byte(nil), l.frame...), nil
			default:
				l.frame = append(l.frame, b)
			}
		}
		left := time.Until(deadline)
		if left <= 0 {
			return nil, ErrTimeout
		}
		if left > 100*time.Millisecond {
			left = 100 * time.Millisecond
		}
		if err := l.port.SetReadTimeout(left); err != nil {
			return nil, err
		}
		n, err := l.port.Read(buf)
		if err != nil {
			return nil, err
		}
		l.raw = append(l.raw, buf[:n]...)
	}
}

func (l *Loader) writePacket(data []byte) error {
	out := make([]byte, 0, len(data)+len(data)/16+2)
	out = append(out, 0xC0)
	for _, b := range data {
		switch b {
		case 0xC0:
			out = append(out, 0xDB, 0xDC)
		case 0xDB:
			out = append(out, 0xDB, 0xDD)
		default:
			out = append(out, b)
		}
	}
	out = append(out, 0xC0)
	_, err := l.port.Write(out)
	return err
}

func checksum(data []byte) uint32 {
	c := byte(checksumSeed)
	for _, b := range data {
		c ^= b
	}
	return uint32(c)
}

func clampTimeout(d time.Duration) time.Duration {
	if d < defaultTimeout {
		return defaultTimeout
	}
	if d > maxTimeout {
		return maxTimeout
	}
	return d
}

func timeoutPerMB(per time.Duration, size int) time.Duration {
	return clampTimeout(time.Duration(float64(per) * float64(size) / 1e6))
}

// command sends op and waits for its response.
func (l *Loader) command(op byte, data []byte, chk uint32, timeout time.Duration) (uint32, []byte, error) {
	pkt := make([]byte, 8, 8+len(data))
	pkt[1] = op
	binary.LittleEndian.PutUint16(pkt[2:], uint16(len(data)))
	binary.LittleEndian.PutUint32(pkt[4:], chk)
	pkt = append(pkt, data...)
	if err := l.writePacket(pkt); err != nil {
		return 0, nil, err
	}
	return l.response(op, timeout)
}

func (l *Loader) response(op byte, timeout time.Duration) (uint32, []byte, error) {
	for range 100 {
		p, err := l.readPacket(timeout)
		if err != nil {
			return 0, nil, err
		}
		if len(p) < 8 || p[0] != 1 {
			continue
		}
		val, body := binary.LittleEndian.Uint32(p[4:8]), p[8:]
		if op == 0 || p[1] == op {
			return val, body, nil
		}
		if len(body) >= 2 && body[0] != 0 && body[1] == romInvalidRecv {
			l.flushInput()
			return 0, nil, fmt.Errorf("command 0x%02x is not supported", op)
		}
	}
	return 0, nil, fmt.Errorf("no response to command 0x%02x", op)
}

// check runs a command and checks the status bytes that follow respLen bytes of data.
func (l *Loader) check(what string, op byte, data []byte, chk uint32, respLen int, timeout time.Duration) (uint32, []byte, error) {
	val, body, err := l.command(op, data, chk, timeout)
	if err != nil {
		return 0, nil, fmt.Errorf("%s: %w", what, err)
	}
	if len(body) < respLen+2 {
		if len(body) >= 2 && body[0] != 0 {
			return 0, nil, &CommandError{What: what, Status: [2]byte{body[0], body[1]}}
		}
		return 0, nil, fmt.Errorf("%s: only got a %d byte response", what, len(body))
	}
	if body[respLen] != 0 {
		return 0, nil, &CommandError{What: what, Status: [2]byte{body[respLen], body[respLen+1]}}
	}
	return val, body[:respLen], nil
}

func le(values ...uint32) []byte {
	out := make([]byte, 4*len(values))
	for i, v := range values {
		binary.LittleEndian.PutUint32(out[4*i:], v)
	}
	return out
}

func (l *Loader) sync() error {
	data := append([]byte{0x07, 0x07, 0x12, 0x20}, bytes.Repeat([]byte{0x55}, 32)...)
	val, _, err := l.command(opSync, data, 0, syncTimeout)
	if err != nil {
		return err
	}
	stub := val == 0
	for range 7 { // the ROM answers SYNC eight times
		v, _, err := l.response(0, syncTimeout)
		if err != nil {
			break
		}
		stub = stub && v == 0
	}
	l.stubDetected = stub
	return nil
}

// Connect syncs with the bootloader, cycling through resets between attempts. resets is
// empty when the chip is already in download mode (right after a fast USB capture, or
// under QEMU).
func (l *Loader) Connect(resets []Reset, attempts int) error {
	var last error = ErrTimeout
	for i := range attempts {
		if len(resets) > 0 {
			resets[i%len(resets)](l.port)
		}
		l.flushInput()
		for range 5 {
			l.flushInput()
			if last = l.sync(); last == nil {
				return l.afterConnect()
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	return fmt.Errorf("failed to connect to the chip: %w", last)
}

func (l *Loader) afterConnect() error {
	if l.stubDetected {
		l.stub = true
		l.logf("Flasher stub is already running")
		return nil
	}
	if l.USB {
		return l.disableWatchdogs()
	}
	return nil
}

// disableWatchdogs stops the RTC watchdogs, which are not reset when the chip enters
// download mode over its own USB port and would otherwise reset it mid-write.
func (l *Loader) disableWatchdogs() error {
	steps := []struct{ addr, val uint32 }{
		{rtcWDTProtect, rtcWDTKey}, {rtcWDTConfig0, 0}, {rtcWDTProtect, 0},
	}
	for _, s := range steps {
		if err := l.WriteReg(s.addr, s.val); err != nil {
			return err
		}
	}
	if err := l.WriteReg(rtcSWDProtect, rtcSWDKey); err != nil {
		return err
	}
	conf, err := l.ReadReg(rtcSWDConf)
	if err != nil {
		return err
	}
	if err = l.WriteReg(rtcSWDConf, conf|rtcSWDAutoFeed); err != nil {
		return err
	}
	return l.WriteReg(rtcSWDProtect, 0)
}

func (l *Loader) ReadReg(addr uint32) (uint32, error) {
	val, _, err := l.check("read register", opReadReg, le(addr), 0, 0, defaultTimeout)
	return val, err
}

func (l *Loader) WriteReg(addr, val uint32) error {
	_, _, err := l.check("write register", opWriteReg, le(addr, val, 0xFFFFFFFF, 0), 0, 0, defaultTimeout)
	return err
}

// Chip names the connected chip, such as "ESP32-C3".
func (l *Loader) Chip() (string, error) {
	if !l.stub {
		_, data, err := l.check("get security info", opSecurityInfo, nil, 0, securityInfoBytes, defaultTimeout)
		if err == nil {
			id := binary.LittleEndian.Uint32(data[12:16])
			if name, ok := chipIDs[id]; ok {
				return name, nil
			}
			return fmt.Sprintf("unknown chip id %d", id), nil
		}
	}
	magic, err := l.ReadReg(chipMagicReg)
	if err != nil {
		return "", err
	}
	if name, ok := chipMagics[magic]; ok {
		return name, nil
	}
	return fmt.Sprintf("unknown chip (magic 0x%08x)", magic), nil
}

// MAC reads the base MAC address from eFuse, formatted as aa:bb:cc:dd:ee:ff.
func (l *Loader) MAC() (string, error) {
	mac0, err := l.ReadReg(macReg)
	if err != nil {
		return "", err
	}
	mac1, err := l.ReadReg(macReg + 4)
	if err != nil {
		return "", err
	}
	b := []byte{byte(mac1 >> 8), byte(mac1), byte(mac0 >> 24), byte(mac0 >> 16), byte(mac0 >> 8), byte(mac0)}
	return fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x", b[0], b[1], b[2], b[3], b[4], b[5]), nil
}

// Security reports whether Secure Boot and Flash Encryption are enabled.
func (l *Loader) Security() (secureBoot, flashEncryption bool, err error) {
	sb, err := l.ReadReg(secureBootReg)
	if err != nil {
		return false, false, err
	}
	cnt, err := l.ReadReg(cryptCntReg)
	if err != nil {
		return false, false, err
	}
	ones := 0
	for v := cnt & cryptCntMask; v != 0; v &= v - 1 {
		ones++
	}
	return sb&secureBootMask != 0, ones%2 == 1, nil
}

// RunStub uploads the flasher stub to RAM and starts it.
func (l *Loader) RunStub() error {
	if l.stub {
		return nil
	}
	var s stubImage
	if err := json.Unmarshal(stubJSON, &s); err != nil {
		return fmt.Errorf("bundled flasher stub: %w", err)
	}
	l.logf("Uploading the flasher stub")
	for _, seg := range []struct {
		data []byte
		addr uint32
	}{{s.Text, s.TextStart}, {s.Data, s.DataStart}} {
		blocks := (len(seg.data) + ramBlock - 1) / ramBlock
		if _, _, err := l.check("enter RAM download mode", opMemBegin, le(uint32(len(seg.data)), uint32(blocks), ramBlock, seg.addr), 0, 0, defaultTimeout); err != nil {
			return err
		}
		for i := range blocks {
			chunk := seg.data[i*ramBlock : min((i+1)*ramBlock, len(seg.data))]
			payload := append(le(uint32(len(chunk)), uint32(i), 0, 0), chunk...)
			if _, _, err := l.check("write to RAM", opMemData, payload, checksum(chunk), 0, defaultTimeout); err != nil {
				return err
			}
		}
	}
	entryZero := uint32(0)
	if s.Entry == 0 {
		entryZero = 1
	}
	// The ROM may switch the UART before its reply is sent, so errors are ignored here.
	_, _, _ = l.check("leave RAM download mode", opMemEnd, le(entryZero, s.Entry), 0, 0, memEndROMTimeout)
	p, err := l.readPacket(defaultTimeout)
	if err != nil {
		return fmt.Errorf("flasher stub did not start: %w", err)
	}
	if string(p) != "OHAI" {
		return fmt.Errorf("flasher stub did not start: unexpected reply %x", p)
	}
	l.stub = true
	l.logf("Flasher stub running")
	return nil
}

// spiCommand runs a SPI flash command that reads readBits bits back.
func (l *Loader) spiCommand(cmd byte, readBits int) (uint32, error) {
	oldUsr, err := l.ReadReg(spiUsrReg)
	if err != nil {
		return 0, err
	}
	oldUsr2, err := l.ReadReg(spiUsr2Reg)
	if err != nil {
		return 0, err
	}
	writes := []struct{ addr, val uint32 }{
		{spiMISODLenReg, uint32(readBits - 1)},
		{spiUsrReg, spiUsrCommand | spiUsrMISO},
		{spiUsr2Reg, 7<<28 | uint32(cmd)},
		{spiW0Reg, 0},
		{spiCmdReg, spiCmdUsr},
	}
	for _, w := range writes {
		if err = l.WriteReg(w.addr, w.val); err != nil {
			return 0, err
		}
	}
	done := false
	for range 10 {
		v, err := l.ReadReg(spiCmdReg)
		if err != nil {
			return 0, err
		}
		if v&spiCmdUsr == 0 {
			done = true
			break
		}
	}
	if !done {
		return 0, errors.New("SPI flash command did not complete")
	}
	status, err := l.ReadReg(spiW0Reg)
	if err != nil {
		return 0, err
	}
	if err = l.WriteReg(spiUsrReg, oldUsr); err != nil {
		return 0, err
	}
	if err = l.WriteReg(spiUsr2Reg, oldUsr2); err != nil {
		return 0, err
	}
	return status & (1<<readBits - 1), nil
}

// FlashSize reads the JEDEC ID and returns the flash size in bytes, or 0 if unknown.
func (l *Loader) FlashSize() (int, uint32, error) {
	id, err := l.spiCommand(spiFlashRDID, 24)
	if err != nil {
		return 0, 0, err
	}
	return flashSizes[(id>>16)&0xFF], id, nil
}

// SizeName formats a flash size the way esptool does ("8MB", "512KB").
func SizeName(size int) string {
	if size >= 1<<20 && size%(1<<20) == 0 {
		return fmt.Sprintf("%dMB", size>>20)
	}
	if size > 0 {
		return fmt.Sprintf("%dKB", size>>10)
	}
	return ""
}

// AttachFlash configures the SPI flash pins and tells the stub the flash size.
func (l *Loader) AttachFlash(size int) error {
	arg := le(0)
	if !l.stub {
		arg = append(arg, 0, 0, 0, 0)
	}
	if _, _, err := l.check("attach SPI flash", opSPIAttach, arg, 0, 0, defaultTimeout); err != nil {
		return err
	}
	if _, _, err := l.check("set SPI flash parameters", opSPISetParams, le(0, uint32(size), 64<<10, 4<<10, 256, 0xFFFF), 0, 0, defaultTimeout); err != nil {
		return err
	}
	l.flashSize = size
	return nil
}

// ChangeBaud switches the chip and the port to a faster baud rate.
func (l *Loader) ChangeBaud(baud int) error {
	old := uint32(0)
	if l.stub {
		old = uint32(l.port.BaudRate())
	}
	if _, _, err := l.command(opChangeBaud, le(uint32(baud), old), 0, defaultTimeout); err != nil {
		return fmt.Errorf("change baud rate: %w", err)
	}
	if err := l.port.SetBaudRate(baud); err != nil {
		return err
	}
	time.Sleep(50 * time.Millisecond)
	l.flushInput()
	return nil
}

// FlashMD5 returns the lowercase hex MD5 of a flash range, computed by the chip.
func (l *Loader) FlashMD5(offset, size int) (string, error) {
	respLen := 32
	if l.stub {
		respLen = 16
	}
	_, data, err := l.check("calculate MD5", opFlashMD5, le(uint32(offset), uint32(size), 0, 0), 0, respLen, timeoutPerMB(md5PerMB, size))
	if err != nil {
		return "", err
	}
	if l.stub {
		return hex.EncodeToString(data), nil
	}
	return string(bytes.ToLower(data)), nil
}

// ReadFlash reads a flash range through the stub and checks it against the chip's MD5.
// progress may be nil.
func (l *Loader) ReadFlash(offset, size int, progress func(done, total int)) ([]byte, error) {
	if !l.stub {
		return nil, errors.New("reading flash needs the flasher stub")
	}
	if _, _, err := l.check("read flash", opReadFlash, le(uint32(offset), uint32(size), sectorSize, 64), 0, 0, defaultTimeout); err != nil {
		return nil, err
	}
	data := make([]byte, 0, size)
	for len(data) < size {
		p, err := l.readPacket(defaultTimeout)
		if err != nil {
			return nil, fmt.Errorf("read flash at 0x%x: %w", offset+len(data), err)
		}
		data = append(data, p...)
		if len(data) < size && len(p) < sectorSize {
			return nil, fmt.Errorf("corrupt read: expected 0x%x bytes, got 0x%x", sectorSize, len(p))
		}
		if err = l.writePacket(le(uint32(len(data)))); err != nil {
			return nil, err
		}
		if progress != nil {
			progress(len(data), size)
		}
	}
	if len(data) > size {
		return nil, errors.New("read more than requested")
	}
	digest, err := l.readPacket(defaultTimeout)
	if err != nil {
		return nil, fmt.Errorf("read flash digest: %w", err)
	}
	if len(digest) != 16 {
		return nil, fmt.Errorf("expected an MD5 digest, got %d bytes", len(digest))
	}
	if sum := md5.Sum(data); !bytes.Equal(sum[:], digest) {
		return nil, errors.New("read flash: MD5 mismatch")
	}
	return data, nil
}

// HardReset restarts the chip into its application.
func (l *Loader) HardReset() { HardReset(l.port, l.USB) }
