package firmware

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func image(chip byte) []byte {
	b := make([]byte, 1024)
	b[0] = 0xE9
	b[12] = chip
	return b
}

func write(t *testing.T, b []byte) (string, string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "fw.bin")
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(b)
	return p, hex.EncodeToString(sum[:])
}

func TestVerifyAcceptsESP32C3(t *testing.T) {
	p, sum := write(t, image(0x05))
	if err := Verify(p, Expect{Size: 1024, SHA256: sum}); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyRejects(t *testing.T) {
	good, sum := write(t, image(0x05))
	s3, s3sum := write(t, image(0x09)) // ESP32-S3
	notESP, notSum := write(t, make([]byte, 64))
	cases := []struct {
		name string
		path string
		want Expect
		err  error
	}{
		{"wrong chip", s3, Expect{SHA256: s3sum}, ErrWrongChip},
		{"not esp", notESP, Expect{SHA256: notSum}, ErrNotESP},
		{"checksum", good, Expect{SHA256: "00" + sum[2:]}, ErrChecksum},
		{"size", good, Expect{Size: 99, SHA256: sum}, ErrSize},
	}
	for _, c := range cases {
		if err := Verify(c.path, c.want); !errors.Is(err, c.err) {
			t.Errorf("%s: got %v, want %v", c.name, err, c.err)
		}
	}
}

func TestVerifyRejectsOversize(t *testing.T) {
	b := make([]byte, MaxSize+1)
	b[0], b[12] = 0xE9, 0x05
	p, _ := write(t, b)
	if err := Verify(p, Expect{}); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("got %v", err)
	}
}
