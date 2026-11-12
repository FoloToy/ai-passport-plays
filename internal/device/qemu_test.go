package device

import (
	"os"
	"strings"
	"testing"

	"github.com/FoloToy/ai-passport-plays/internal/esp"
)

// TestQEMURestoreAndBackup runs a full factory restore and then a firmware backup on
// Espressif QEMU (see internal/esp TestQEMU). Set ESP_QEMU_ADDR to run it.
func TestQEMURestoreAndBackup(t *testing.T) {
	addr := os.Getenv("ESP_QEMU_ADDR")
	if addr == "" {
		t.Skip("ESP_QEMU_ADDR not set")
	}
	connect := func(_, _ string, attempts int, log func(string)) (Device, error) {
		p, err := esp.DialTCP(addr)
		if err != nil {
			return nil, err
		}
		l := esp.New(p)
		l.Log = log
		if err = l.Connect(nil, attempts); err != nil {
			_ = l.Close()
			return nil, err
		}
		return &serialDevice{l: l}, nil
	}
	r := Runner{Connect: connect}
	for _, mode := range []string{"restore", "backup"} {
		var logs []string
		var done Event
		err := r.Restore(t.Context(), Request{Command: mode, Port: "qemu", FirmwareDir: firmwareDir, BackupDir: t.TempDir(), FullBackup: true}, func(e Event) {
			switch e.Type {
			case "log":
				logs = append(logs, e.Message)
			case "complete":
				done = e
			}
		}, make(chan struct{}))
		t.Logf("%s log:\n%s", mode, strings.Join(logs, "\n"))
		if err != nil {
			t.Fatalf("%s: %v", mode, err)
		}
		t.Logf("%s complete: %+v", mode, done)
	}
}
