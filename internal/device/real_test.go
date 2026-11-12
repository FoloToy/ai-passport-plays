package device

import (
	"os"
	"testing"
	"time"

	"github.com/FoloToy/ai-passport-plays/internal/esp"
)

// probeROM reports whether the ROM bootloader answers SYNC on path without resetting it.
func probeROM(t *testing.T, path string) bool {
	p, err := esp.OpenSerial(path, 115200)
	if err != nil {
		t.Logf("open %s: %v", path, err)
		return false
	}
	l := esp.New(p)
	defer l.Close()
	return l.Connect(nil, 2) == nil
}

// TestRealDeviceReset connects to a real AI Passport (ESP_REAL_PORT), reads its identity,
// hard-resets it and checks it left download mode. It never writes flash.
func TestRealDeviceReset(t *testing.T) {
	port := os.Getenv("ESP_REAL_PORT")
	if port == "" {
		t.Skip("ESP_REAL_PORT not set")
	}
	d, err := ConnectSerial(port, "default-reset", 4, func(s string) { t.Log(s) })
	if err != nil {
		t.Fatal(err)
	}
	chip, _ := d.Chip()
	mac, _ := d.MAC()
	sb, fe, _ := d.Security()
	t.Logf("chip %s mac %s secure_boot %v encryption %v", chip, mac, sb, fe)
	size, err := d.Prepare(460800)
	t.Logf("flash %s %v", esp.SizeName(size), err)
	start := time.Now()
	if err = d.Reset(); err != nil {
		t.Fatal(err)
	}
	_ = d.Close()
	t.Logf("reset returned after %s", time.Since(start))
	for i := 0; i < 50; i++ {
		if _, err := os.Stat(port); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	time.Sleep(1500 * time.Millisecond)
	if probeROM(t, port) {
		t.Fatal("still in the ROM bootloader after reset")
	}
	t.Log("device left download mode (ROM no longer answers)")
}

// TestRealDeviceProbeSeesBootloader checks probeROM's positive case, then resets.
func TestRealDeviceProbeSeesBootloader(t *testing.T) {
	port := os.Getenv("ESP_REAL_PORT")
	if port == "" {
		t.Skip("ESP_REAL_PORT not set")
	}
	d, err := ConnectSerial(port, "default-reset", 4, nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = d.Close() // leave it in download mode
	time.Sleep(300 * time.Millisecond)
	in := probeROM(t, port)
	d, err = ConnectSerial(port, "no-reset", 2, nil)
	if err == nil {
		_ = d.Reset()
		_ = d.Close()
	}
	if !in {
		t.Fatal("probe did not see the bootloader")
	}
}

// TestRealDeviceWatchdogReset checks the fallback reset on its own.
func TestRealDeviceWatchdogReset(t *testing.T) {
	port := os.Getenv("ESP_REAL_PORT")
	if port == "" {
		t.Skip("ESP_REAL_PORT not set")
	}
	d, err := ConnectSerial(port, "default-reset", 4, nil)
	if err != nil {
		t.Fatal(err)
	}
	sd := d.(*serialDevice)
	if err = sd.l.WatchdogReset(); err != nil {
		t.Fatal(err)
	}
	_ = sd.Close()
	if l := sd.romStillThere(); l != nil {
		_ = l.Close()
		t.Fatal("still in the ROM bootloader after a watchdog reset")
	}
}

// TestRealDeviceBackup runs the read-only firmware backup job end to end, with automatic
// capture, into ESP_REAL_BACKUP_DIR.
func TestRealDeviceBackup(t *testing.T) {
	dir := os.Getenv("ESP_REAL_BACKUP_DIR")
	if dir == "" {
		t.Skip("ESP_REAL_BACKUP_DIR not set")
	}
	var done Event
	start := time.Now()
	err := Runner{}.Restore(t.Context(), Request{Command: "backup", BackupDir: dir}, func(e Event) {
		switch e.Type {
		case "log", "hint":
			t.Logf("%s %s%s", e.Type, e.Message, e.Code)
		case "stage":
			t.Logf("stage %s", e.Stage)
		case "complete":
			done = e
		}
	}, make(chan struct{}))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("took %s, restarted=%v, cardid=%s", time.Since(start).Round(time.Second), done.Restarted, done.CardID)
	if !done.Restarted {
		t.Fatal("device did not restart")
	}
}
