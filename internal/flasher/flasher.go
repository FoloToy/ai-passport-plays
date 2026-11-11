// Package flasher runs one device job at a time (installing a play or a factory restore)
// and exposes its progress.
package flasher

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/FoloToy/ai-passport-plays/internal/firmware"
	"github.com/FoloToy/ai-passport-plays/internal/worker"
)

type State struct {
	Active bool `json:"active"`
	// Kind is "flash" (install a play), "restore" (factory restore) or "backup" (read the whole flash).
	Kind string `json:"kind,omitempty"`
	Slug string `json:"slug,omitempty"`
	Port string `json:"port,omitempty"`
	// Flash: idle, checking, connecting, writing, verifying, resetting, done, failed.
	// Restore adds: preparing, waiting, backup, cardid, cancelled.
	Stage     string   `json:"stage"`
	Percent   int      `json:"percent"`
	Chip      string   `json:"chip,omitempty"`
	MAC       string   `json:"mac,omitempty"`
	Flash     string   `json:"flash,omitempty"`
	ErrorCode string   `json:"errorCode,omitempty"`
	Error     string   `json:"error,omitempty"`
	Logs      []string `json:"logs"`
	// Restore details.
	Hint       string `json:"hint,omitempty"`    // multiple, code43, replug
	Session    string `json:"session,omitempty"` // backup folder of this restore or backup
	File       string `json:"file,omitempty"`    // the saved flash image of a backup
	CardID     string `json:"cardid,omitempty"`  // keys, blank_ff, blank_00, unknown
	Cancelling bool   `json:"cancelling,omitempty"`
	// CardIDOverwritten is set when a play image replaced a non-blank identity area.
	CardIDOverwritten bool      `json:"cardidOverwritten,omitempty"`
	StartedAt         time.Time `json:"startedAt"`
	EndedAt           time.Time `json:"endedAt"`
}

type Job struct {
	Slug   string
	Port   string
	Image  string
	SHA256 string
	Size   int64
	Baud   int
	// BackupDir keeps a copy of the device identity area if the image overwrites it.
	BackupDir string
}

// RestoreJob is a factory restore. The worker finds the device by itself.
type RestoreJob struct {
	FirmwareDir string
	BackupDir   string
	FullBackup  bool
	// Backup only reads the device and saves its whole flash; nothing is written.
	Backup bool
}

type Runner interface {
	Flash(ctx context.Context, req worker.Request, on func(worker.Event)) error
	Restore(ctx context.Context, req worker.Request, on func(worker.Event), cancel <-chan struct{}) error
}

type Manager struct {
	runner   Runner
	mu       sync.Mutex
	state    State
	cancel   context.CancelFunc
	stop     chan struct{} // closed to ask a restore to stop safely
	stopOnce sync.Once
}

func New(r Runner) *Manager {
	return &Manager{runner: r, state: State{Stage: "idle", Logs: []string{}}}
}

func (m *Manager) Snapshot() State {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.state
	s.Logs = append([]string(nil), s.Logs...)
	return s
}

func (m *Manager) Active() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state.Active
}

// Reset clears a finished job so the dialog can start fresh.
func (m *Manager) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.state.Active {
		m.state = State{Stage: "idle", Logs: []string{}}
	}
}

func (m *Manager) Start(job Job) error {
	m.mu.Lock()
	if m.state.Active {
		m.mu.Unlock()
		return errors.New("busy")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	m.cancel = cancel
	m.state = State{Active: true, Kind: "flash", Slug: job.Slug, Port: job.Port, Stage: "checking", Logs: []string{}, StartedAt: time.Now()}
	m.mu.Unlock()
	go m.run(ctx, job)
	return nil
}

// StartRestore begins a factory restore, or a read-only firmware backup when job.Backup is set.
func (m *Manager) StartRestore(job RestoreJob) error {
	m.mu.Lock()
	if m.state.Active {
		m.mu.Unlock()
		return errors.New("busy")
	}
	// Up to 150 s waiting for the device, a full 8 MB backup, writing and verifying.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	m.cancel = cancel
	m.stop, m.stopOnce = make(chan struct{}), sync.Once{}
	kind, command := "restore", ""
	if job.Backup {
		kind, command = "backup", "backup"
	}
	m.state = State{Active: true, Kind: kind, Stage: "preparing", Logs: []string{}, StartedAt: time.Now()}
	stop := m.stop
	m.mu.Unlock()
	go func() {
		defer cancel()
		err := m.runner.Restore(ctx, worker.Request{Command: command, FirmwareDir: job.FirmwareDir, BackupDir: job.BackupDir, FullBackup: job.FullBackup}, m.event, stop)
		var ce *worker.CodedError
		if errors.As(err, &ce) && ce.Code == "cancelled" {
			m.mu.Lock()
			m.state.Active, m.state.Stage, m.state.Cancelling, m.state.EndedAt = false, "cancelled", false, time.Now()
			m.mu.Unlock()
			return
		}
		m.finish(err)
	}()
	return nil
}

// Cancel asks a running restore to stop. The worker ignores it once writing has begun,
// so the device is never left half-written. Installing a play cannot be cancelled.
func (m *Manager) Cancel() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.state.Active || (m.state.Kind != "restore" && m.state.Kind != "backup") {
		return false
	}
	if m.state.Kind == "restore" {
		switch m.state.Stage {
		case "writing", "verifying", "cardid", "resetting":
			return false
		}
	}
	m.state.Cancelling = true
	m.stopOnce.Do(func() { close(m.stop) })
	return true
}

func (m *Manager) run(ctx context.Context, job Job) {
	defer m.cancel()
	if err := firmware.Verify(job.Image, firmware.Expect{Size: job.Size, SHA256: job.SHA256}); err != nil {
		code := "image"
		if errors.Is(err, firmware.ErrChecksum) || errors.Is(err, firmware.ErrSize) {
			code = "checksum"
		} else if errors.Is(err, firmware.ErrWrongChip) {
			code = "image_chip"
		}
		m.fail(code, err)
		return
	}
	err := m.runner.Flash(ctx, worker.Request{Port: job.Port, Image: job.Image, SHA256: job.SHA256, Size: job.Size, Baud: job.Baud, BackupDir: job.BackupDir}, m.event)
	m.finish(err)
}

func (m *Manager) finish(err error) {
	if err != nil {
		code := "other"
		var ce *worker.CodedError
		if errors.As(err, &ce) && ce.Code != "" {
			code = ce.Code
		}
		m.fail(code, err)
		return
	}
	m.mu.Lock()
	m.state.Active, m.state.Stage, m.state.Percent, m.state.EndedAt = false, "done", 100, time.Now()
	m.mu.Unlock()
}

func (m *Manager) fail(code string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.Active, m.state.Stage, m.state.ErrorCode, m.state.Error, m.state.EndedAt = false, "failed", code, err.Error(), time.Now()
}

func (m *Manager) event(ev worker.Event) {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch ev.Type {
	case "stage":
		m.state.Stage = ev.Stage
		if ev.Stage == "writing" || ev.Stage == "verifying" || ev.Stage == "backup" {
			m.state.Percent = 0
		}
		if ev.Stage == "connecting" {
			m.state.Hint = ""
		}
	case "identified":
		m.state.Chip, m.state.MAC, m.state.Flash = ev.Chip, ev.MAC, ev.Flash
		m.state.Hint = ""
	case "hint":
		m.state.Hint = ev.Code
		if ev.Port != "" {
			m.state.Port = ev.Port
		}
	case "cardid":
		m.state.CardID = ev.Status
		m.state.CardIDOverwritten = ev.Overwrite && ev.Status == "keys"
	case "complete":
		if ev.Session != "" {
			m.state.Session = ev.Session
		}
		m.state.File = ev.File
		if ev.CardID != "" {
			m.state.CardID = ev.CardID
		}
	case "progress":
		m.state.Percent = ev.Percent
	case "log":
		if len(m.state.Logs) >= 400 {
			m.state.Logs = m.state.Logs[100:]
		}
		m.state.Logs = append(m.state.Logs, ev.Message)
	}
}
