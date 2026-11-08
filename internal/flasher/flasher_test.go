package flasher

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/FoloToy/ai-passport-plays/internal/worker"
)

type fakeRunner struct {
	called  bool
	err     error
	restore func(on func(worker.Event), cancel <-chan struct{}) error
	req     worker.Request
}

func (f *fakeRunner) Restore(_ context.Context, req worker.Request, on func(worker.Event), cancel <-chan struct{}) error {
	f.called, f.req = true, req
	return f.restore(on, cancel)
}

func (f *fakeRunner) Flash(_ context.Context, req worker.Request, on func(worker.Event)) error {
	f.called = true
	on(worker.Event{Type: "stage", Stage: "connecting"})
	on(worker.Event{Type: "identified", Chip: "ESP32-C3", MAC: "aa:bb", Flash: "8MB"})
	on(worker.Event{Type: "stage", Stage: "writing"})
	on(worker.Event{Type: "progress", Percent: 50})
	return f.err
}

func image(t *testing.T) (string, string, int64) {
	b := make([]byte, 512)
	b[0], b[12] = 0xE9, 0x05
	p := filepath.Join(t.TempDir(), "firmware.bin")
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(b)
	return p, hex.EncodeToString(sum[:]), int64(len(b))
}

func wait(t *testing.T, m *Manager) State {
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if s := m.Snapshot(); !s.Active {
			return s
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("flash did not finish")
	return State{}
}

func TestFlashSuccess(t *testing.T) {
	r := &fakeRunner{}
	m := New(r)
	p, sum, size := image(t)
	if err := m.Start(Job{Slug: "x", Port: "/dev/cu.test", Image: p, SHA256: sum, Size: size}); err != nil {
		t.Fatal(err)
	}
	s := wait(t, m)
	if s.Stage != "done" || s.Chip != "ESP32-C3" || s.Percent != 100 {
		t.Fatalf("unexpected state %+v", s)
	}
}

func TestChecksumFailureNeverTouchesDevice(t *testing.T) {
	r := &fakeRunner{}
	m := New(r)
	p, _, size := image(t)
	if err := m.Start(Job{Slug: "x", Port: "/dev/cu.test", Image: p, SHA256: "00", Size: size}); err != nil {
		t.Fatal(err)
	}
	s := wait(t, m)
	if s.Stage != "failed" || s.ErrorCode != "checksum" || r.called {
		t.Fatalf("expected checksum failure before flashing: %+v called=%v", s, r.called)
	}
}

func TestWorkerErrorCodeIsKept(t *testing.T) {
	r := &fakeRunner{err: &worker.CodedError{Code: "wrong_chip", Message: "Detected ESP32-S3"}}
	m := New(r)
	p, sum, size := image(t)
	_ = m.Start(Job{Slug: "x", Port: "/dev/cu.test", Image: p, SHA256: sum, Size: size})
	if s := wait(t, m); s.ErrorCode != "wrong_chip" {
		t.Fatalf("got %+v", s)
	}
	if err := m.Start(Job{}); err != nil {
		t.Fatal("a finished job must not block the next one")
	}
}

func TestRestoreReportsSessionAndCardID(t *testing.T) {
	r := &fakeRunner{restore: func(on func(worker.Event), _ <-chan struct{}) error {
		on(worker.Event{Type: "stage", Stage: "waiting"})
		on(worker.Event{Type: "hint", Code: "replug"})
		on(worker.Event{Type: "stage", Stage: "connecting"})
		on(worker.Event{Type: "identified", Chip: "ESP32-C3", MAC: "aa:bb", Flash: "8MB"})
		on(worker.Event{Type: "complete", MAC: "aa:bb", Session: "/backups/x", CardID: "keys"})
		return nil
	}}
	m := New(r)
	if err := m.StartRestore(RestoreJob{FirmwareDir: "/fw", BackupDir: "/backups", FullBackup: true}); err != nil {
		t.Fatal(err)
	}
	s := wait(t, m)
	if s.Kind != "restore" || s.Stage != "done" || s.Session != "/backups/x" || s.CardID != "keys" || s.Hint != "" {
		t.Fatalf("unexpected state %+v", s)
	}
	if !r.req.FullBackup || r.req.FirmwareDir != "/fw" || r.req.BackupDir != "/backups" {
		t.Fatalf("request not passed through: %+v", r.req)
	}
}

func TestRestoreCancelOnlyBeforeWriting(t *testing.T) {
	writing := make(chan struct{})
	release := make(chan struct{})
	r := &fakeRunner{restore: func(on func(worker.Event), cancel <-chan struct{}) error {
		on(worker.Event{Type: "stage", Stage: "waiting"})
		<-cancel
		return &worker.CodedError{Code: "cancelled", Message: "stopped"}
	}}
	m := New(r)
	if err := m.StartRestore(RestoreJob{}); err != nil {
		t.Fatal(err)
	}
	for m.Snapshot().Stage != "waiting" {
		time.Sleep(5 * time.Millisecond)
	}
	if !m.Cancel() {
		t.Fatal("cancel while waiting should be accepted")
	}
	m.Cancel() // a second request must not panic on the closed channel
	if s := wait(t, m); s.Stage != "cancelled" || s.ErrorCode != "" {
		t.Fatalf("unexpected state %+v", s)
	}

	r.restore = func(on func(worker.Event), cancel <-chan struct{}) error {
		on(worker.Event{Type: "stage", Stage: "writing"})
		close(writing)
		<-release
		return nil
	}
	if err := m.StartRestore(RestoreJob{}); err != nil {
		t.Fatal(err)
	}
	<-writing
	if m.Cancel() {
		t.Fatal("cancel must be refused once writing has started")
	}
	if err := m.Start(Job{}); err == nil {
		t.Fatal("a second device job must be refused while a restore runs")
	}
	close(release)
	if s := wait(t, m); s.Stage != "done" {
		t.Fatalf("unexpected state %+v", s)
	}
}
