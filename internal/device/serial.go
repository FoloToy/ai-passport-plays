package device

import (
	"fmt"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.bug.st/serial/enumerator"

	"github.com/FoloToy/ai-passport-plays/internal/esp"
)

const (
	EspressifVID = 0x303A
	USBJTAGPID   = 0x1001
)

// Candidate is a USB serial port that may be an AI Passport.
type Candidate struct {
	Device       string
	Description  string
	VID, PID     int
	SerialNumber string
}

// TrackingKey identifies the USB device even when its port name changes, which
// happens on Windows when the chip re-enumerates in download mode.
func (c Candidate) TrackingKey() string {
	if c.SerialNumber != "" {
		return "serial:" + strings.ToLower(c.SerialNumber)
	}
	return fmt.Sprintf("vidpid:%d:%d", c.VID, c.PID)
}

func listCandidates() ([]Candidate, error) {
	list, err := enumerator.GetDetailedPortsList()
	if err != nil {
		return nil, err
	}
	out := []Candidate{}
	for _, p := range list {
		// macOS lists each device twice; only the /dev/cu.* callout node is usable.
		if runtime.GOOS == "darwin" && !strings.HasPrefix(p.Name, "/dev/cu.") {
			continue
		}
		if !p.IsUSB {
			continue
		}
		vid, _ := strconv.ParseUint(p.VID, 16, 16)
		pid, _ := strconv.ParseUint(p.PID, 16, 16)
		desc := p.Product
		if desc == "" {
			desc = p.Name
		}
		out = append(out, Candidate{Device: p.Name, Description: desc, VID: int(vid), PID: int(pid), SerialNumber: p.SerialNumber})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Device < out[j].Device })
	return out, nil
}

// espressif keeps only Espressif USB devices.
func espressif(all []Candidate) []Candidate {
	out := []Candidate{}
	for _, c := range all {
		if c.VID == EspressifVID || strings.Contains(strings.ToLower(c.Description), "espressif") {
			out = append(out, c)
		}
	}
	return out
}

func sortPorts(ports []Port) {
	sort.SliceStable(ports, func(i, j int) bool {
		if ports[i].Recommended != ports[j].Recommended {
			return ports[i].Recommended
		}
		return ports[i].Path < ports[j].Path
	})
}

// isUSBJTAG reports whether path is the chip's own USB-JTAG/Serial port. Ports that do
// not enumerate (Windows Code 43) are AI Passports by construction, so they count too.
func isUSBJTAG(path string) bool {
	list, err := listCandidates()
	if err != nil {
		return true
	}
	for _, c := range list {
		if strings.EqualFold(c.Device, path) {
			return c.VID == EspressifVID && c.PID == USBJTAGPID
		}
	}
	return true
}

// ConnectSerial opens a real serial port and syncs with the ROM bootloader.
func ConnectSerial(path, mode string, attempts int, log func(string)) (Device, error) {
	port, err := esp.OpenSerial(path, 115200)
	if err != nil {
		return nil, err
	}
	l := esp.New(port)
	l.Log = log
	l.USB = isUSBJTAG(path)
	var resets []esp.Reset
	if mode != "no-reset" {
		resets = esp.DefaultResets(l.USB)
	}
	if err = l.Connect(resets, attempts); err != nil {
		_ = l.Close()
		return nil, err
	}
	return &serialDevice{l: l}, nil
}

type serialDevice struct{ l *esp.Loader }

func (d *serialDevice) Chip() (string, error) { return d.l.Chip() }
func (d *serialDevice) MAC() (string, error)  { return d.l.MAC() }
func (d *serialDevice) Security() (bool, bool, error) {
	return d.l.Security()
}

func (d *serialDevice) Prepare(baud int) (int, error) {
	if err := d.l.RunStub(); err != nil {
		return 0, err
	}
	size, id, err := d.l.FlashSize()
	if err != nil {
		return 0, err
	}
	if d.l.Log != nil {
		d.l.Log(fmt.Sprintf("Flash ID %06x, size %s", id, orUnknown(esp.SizeName(size))))
	}
	if size == 0 {
		return 0, nil
	}
	if err = d.l.AttachFlash(size); err != nil {
		return 0, err
	}
	if baud != 0 && baud != d.l.Port().BaudRate() {
		if err = d.l.ChangeBaud(baud); err != nil && d.l.Log != nil {
			d.l.Log(fmt.Sprintf("Keeping 115200 baud: %v", err))
		}
	}
	return size, nil
}

func (d *serialDevice) Read(offset, size int, progress func(int, int)) ([]byte, error) {
	return d.l.ReadFlash(offset, size, progress)
}

func (d *serialDevice) Write(segs []esp.Segment, progress func(int, int)) error {
	return d.l.WriteFlash(segs, progress)
}

func (d *serialDevice) MD5(offset, size int) (string, error) { return d.l.FlashMD5(offset, size) }
func (d *serialDevice) Reset() error                         { d.l.HardReset(); return nil }
func (d *serialDevice) Close() error                         { return d.l.Close() }

func orUnknown(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}

// quickBootloaderReset opens the USB CDC port briefly and requests ROM download mode.
// Some broken firmware only keeps USB up for ~300 ms, shorter than esptool's own
// USB-JTAG reset (100 ms per step), so this uses the same order with 5 ms steps.
func quickBootloaderReset(c Candidate) bool {
	port, err := esp.OpenSerial(c.Device, 115200)
	if err != nil {
		return false
	}
	defer port.Close()
	// Errors after opening usually mean USB is already re-enumerating, which is fine.
	_ = port.SetReadTimeout(100 * time.Millisecond)
	esp.USBJTAGReset(5 * time.Millisecond)(port)
	return true
}
