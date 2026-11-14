package factory

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEmbeddedFirmwareMatchesTable(t *testing.T) {
	if err := Verify(); err != nil {
		t.Fatal(err)
	}
}

func TestSegmentsNeverTouchCardID(t *testing.T) {
	for _, s := range Segments {
		if s.Offset < CardIDOffset+CardIDSize && s.End() > CardIDOffset {
			t.Fatalf("%s overlaps cardid", s.File)
		}
	}
	bad := append([]Segment{}, Segments...)
	bad = append(bad, Segment{Offset: 0x350000, File: "merged.bin", Size: 0x10000})
	if CheckLayout(bad) == nil {
		t.Fatal("a segment overlapping cardid must be rejected")
	}
}

// The bootloader header must already say DIO / 80 MHz / 8 MB: the worker writes it
// unchanged ("keep"), which equals the original tool's dio/80m/detect on an 8 MB chip.
func TestBootloaderHeaderIsDIO80m8MB(t *testing.T) {
	b, err := files.ReadFile("firmware/bootloader.bin")
	if err != nil {
		t.Fatal(err)
	}
	if b[0] != 0xE9 || b[2] != 0x02 || b[3] != 0x3F || b[12] != 0x05 {
		t.Fatalf("unexpected bootloader header % x", b[:16])
	}
}

func TestExtractReusesAndRepairs(t *testing.T) {
	dir := t.TempDir()
	out, err := Extract(dir)
	if err != nil {
		t.Fatal(err)
	}
	victim := filepath.Join(out, "imgava.bin")
	if err := os.WriteFile(victim, []byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Extract(dir); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(victim)
	if int64(len(b)) != Segments[3].Size {
		t.Fatalf("tampered segment was not repaired (%d bytes)", len(b))
	}
}
