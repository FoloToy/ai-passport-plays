// Package flasher runs one flash job at a time and exposes its progress.
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
	Active    bool      `json:"active"`
	Slug      string    `json:"slug,omitempty"`
	Port      string    `json:"port,omitempty"`
	Stage     string    `json:"stage"` // idle, checking, connecting, writing, verifying, resetting, done, failed
	Percent   int       `json:"percent"`
	Chip      string    `json:"chip,omitempty"`
	MAC       string    `json:"mac,omitempty"`
	Flash     string    `json:"flash,omitempty"`
	ErrorCode string    `json:"errorCode,omitempty"`
	Error     string    `json:"error,omitempty"`
	Logs      []string  `json:"logs"`
	StartedAt time.Time `json:"startedAt"`
	EndedAt   time.Time `json:"endedAt"`
}

type Job struct {
	Slug   string
	Port   string
	Image  string
	SHA256 string
	Size   int64
	Baud   int
}

type Runner interface {
	Flash(ctx context.Context, req worker.Request, on func(worker.Event)) error
}

type Manager struct {
	runner Runner
	mu     sync.Mutex
	state  State
	cancel context.CancelFunc
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
	m.state = State{Active: true, Slug: job.Slug, Port: job.Port, Stage: "checking", Logs: []string{}, StartedAt: time.Now()}
	m.mu.Unlock()
	go m.run(ctx, job)
	return nil
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
	err := m.runner.Flash(ctx, worker.Request{Port: job.Port, Image: job.Image, SHA256: job.SHA256, Size: job.Size, Baud: job.Baud}, m.event)
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
		if ev.Stage == "writing" || ev.Stage == "verifying" {
			m.state.Percent = 0
		}
	case "identified":
		m.state.Chip, m.state.MAC, m.state.Flash = ev.Chip, ev.MAC, ev.Flash
	case "progress":
		m.state.Percent = ev.Percent
	case "log":
		if len(m.state.Logs) >= 400 {
			m.state.Logs = m.state.Logs[100:]
		}
		m.state.Logs = append(m.state.Logs, ev.Message)
	}
}
