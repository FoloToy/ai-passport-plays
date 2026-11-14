// Package device installs plays on an AI Passport (ESP32-C3), restores its factory
// firmware and backs up its flash. It runs in the app process and talks to the chip
// with the Go loader in internal/esp; no Python or esptool is needed.
package device

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/FoloToy/ai-passport-desktop/pkg/esp"
)

// Port is a serial port offered to the user.
type Port struct {
	Path        string `json:"path"`
	VID         int    `json:"vid"`
	PID         int    `json:"pid"`
	Serial      string `json:"serial"`
	Label       string `json:"label"`
	Recommended bool   `json:"recommended"`
}

type Request struct {
	Command string `json:"command"`
	Port    string `json:"port,omitempty"`
	Image   string `json:"image,omitempty"`
	SHA256  string `json:"sha256,omitempty"`
	Size    int64  `json:"size,omitempty"`
	Baud    int    `json:"baud,omitempty"`
	// BackupDir receives cardid copies (flash) or the restore backup sessions (restore).
	BackupDir string `json:"backupDir,omitempty"`
	// Factory restore only.
	FirmwareDir string `json:"firmwareDir,omitempty"`
	FullBackup  bool   `json:"fullBackup,omitempty"`
}

// Event reports progress to the UI. Types: stage, identified, progress, log, hint,
// cardid, complete.
type Event struct {
	Type    string `json:"type"`
	Stage   string `json:"stage,omitempty"`
	Percent int    `json:"percent,omitempty"`
	Message string `json:"message,omitempty"`
	Code    string `json:"code,omitempty"`
	Chip    string `json:"chip,omitempty"`
	MAC     string `json:"mac,omitempty"`
	Flash   string `json:"flash,omitempty"`
	// Factory restore and cardid events.
	Port      string `json:"port,omitempty"`
	Session   string `json:"session,omitempty"`
	File      string `json:"file,omitempty"`
	CardID    string `json:"cardid,omitempty"`
	Status    string `json:"status,omitempty"`
	Overwrite bool   `json:"overwrite,omitempty"`
	// Restarted is set on complete when the device was confirmed to have left download mode.
	Restarted bool `json:"restarted,omitempty"`
}

// CodedError carries a stable code the UI translates.
type CodedError struct {
	Code    string
	Message string
}

func (e *CodedError) Error() string { return e.Message }

// ErrNotRestarted means the device is still in download mode after the automatic reset.
var ErrNotRestarted = errors.New("the device did not restart by itself; unplug and replug USB")

func coded(code, format string, args ...any) *CodedError {
	return &CodedError{Code: code, Message: fmt.Sprintf(format, args...)}
}

// Device is one open connection to a chip. Tests replace it with a fake.
type Device interface {
	Chip() (string, error)
	MAC() (string, error)
	Security() (secureBoot, flashEncryption bool, err error)
	// Prepare runs the flasher stub, detects the flash size in bytes (0 if unknown)
	// and switches to baud when it is not 0.
	Prepare(baud int) (int, error)
	Read(offset, size int, progress func(done, total int)) ([]byte, error)
	// Write writes and MD5-checks each segment; flash mode and size bytes are kept.
	Write(segs []esp.Segment, progress func(done, total int)) error
	MD5(offset, size int) (string, error)
	// Reset restarts the chip into its application and closes the connection. It
	// returns ErrNotRestarted when the chip is still in download mode afterwards.
	Reset() error
	Close() error
}

// Connector opens a device. mode is "default-reset" or "no-reset".
type Connector func(port, mode string, attempts int, log func(string)) (Device, error)

// Runner runs device jobs in-process.
type Runner struct {
	// Connect defaults to opening a real serial port.
	Connect Connector
	// List defaults to the system's USB serial ports.
	List func() ([]Candidate, error)
	// WaitTimeout bounds the wait for a device in a restore or backup (default 150 s).
	WaitTimeout time.Duration
}

// Available is always true: flashing no longer needs a separate worker.
func (r Runner) Available() bool { return true }

func (r Runner) connector() Connector {
	if r.Connect != nil {
		return r.Connect
	}
	return ConnectSerial
}

func (r Runner) lister() func() ([]Candidate, error) {
	if r.List != nil {
		return r.List
	}
	return listCandidates
}

// Ports lists USB serial ports, Espressif devices first.
func (r Runner) Ports(ctx context.Context) ([]Port, error) {
	found, err := r.lister()()
	if err != nil {
		return nil, fmt.Errorf("listing serial ports failed: %w", err)
	}
	ports := []Port{}
	for _, c := range found {
		if c.VID == 0 && c.PID == 0 {
			continue
		}
		label := c.Description
		if label == "" {
			label = c.Device
		}
		ports = append(ports, Port{Path: c.Device, VID: c.VID, PID: c.PID, Serial: c.SerialNumber, Label: label, Recommended: c.VID == EspressifVID})
	}
	sortPorts(ports)
	return ports, nil
}

// Flash installs one play image at 0x0.
func (r Runner) Flash(ctx context.Context, req Request, on func(Event)) error {
	if !r.portPresent(req.Port) {
		return coded("no_device", "No device on %s. Plug in the AI Passport, refresh the list and choose it again.", req.Port)
	}
	return guard(func() error { return flashImage(ctx, r.connector(), req, on) })
}

// Restore runs a factory restore, or a read-only firmware backup when req.Command is
// "backup". Closing cancel stops it, but only before writing starts.
func (r Runner) Restore(ctx context.Context, req Request, on func(Event), cancel <-chan struct{}) error {
	mode := "restore"
	if req.Command == "backup" {
		mode = "backup"
	}
	job := &restoreJob{
		mode: mode, firmwareDir: req.FirmwareDir, backupRoot: req.BackupDir, fullBackup: req.FullBackup,
		emit: on, cancel: cancel, ctx: ctx, port: req.Port, connect: r.connector(), candidates: r.lister(),
		capture: quickBootloaderReset, hints: problemPortHints, timeout: connectTimeout, interval: scanInterval,
	}
	if r.WaitTimeout > 0 {
		job.timeout = r.WaitTimeout
	}
	return guard(func() error {
		res, err := job.run()
		if err != nil {
			return err
		}
		on(Event{Type: "complete", MAC: res.MAC, Session: res.Session, CardID: res.CardID, File: res.File, Restarted: res.Restarted})
		return nil
	})
}

// guard turns a panic in device code into an error instead of crashing the app.
func guard(fn func() error) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("internal error: %v\n%s", p, debug.Stack())
		}
	}()
	return fn()
}

// closeOnDone closes the device when ctx ends so a blocked read returns.
func closeOnDone(ctx context.Context, d Device) (stop func()) {
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = d.Close()
		case <-done:
		}
	}()
	return func() { close(done) }
}

func isESP32C3(chip string) bool {
	return strings.EqualFold(strings.ReplaceAll(chip, "-", ""), "esp32c3")
}

// percent turns byte progress into progress events, sending each value once.
func percent(on func(Event)) func(done, total int) {
	last := -1
	return func(done, total int) {
		p := 0
		if total > 0 {
			p = done * 100 / total
		}
		if p != last {
			last = p
			on(Event{Type: "progress", Percent: p})
		}
	}
}

// portPresent reports whether path is still a serial port. Unknown (listing failed or a
// Windows port that does not enumerate) counts as present so the connect can decide.
func (r Runner) portPresent(path string) bool {
	if path == "" {
		return false
	}
	found, err := r.lister()()
	if err != nil {
		return true
	}
	for _, c := range found {
		if strings.EqualFold(c.Device, path) {
			return true
		}
	}
	return runtime.GOOS == "windows" && len(problemPortHints()) > 0
}
