// Package firmware checks that a file is a complete AI Passport (ESP32-C3) image
// meant to be written at offset 0x0.
package firmware

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	// ChipESP32C3 is the chip_id stored in the ESP image extended header.
	ChipESP32C3 = 0x0005
	// MaxSize is the AI Passport flash capacity (8 MB).
	MaxSize = 8 << 20
	// Format is the only firmware format the community publishes for AI Passport.
	Format = "esp-merged-0x0"
)

var (
	ErrEmpty     = errors.New("firmware file is empty")
	ErrTooLarge  = errors.New("firmware is larger than the 8 MB AI Passport flash")
	ErrNotESP    = errors.New("not an ESP firmware image")
	ErrWrongChip = errors.New("firmware is not built for ESP32-C3 (AI Passport)")
	ErrChecksum  = errors.New("firmware SHA-256 mismatch")
	ErrSize      = errors.New("firmware size mismatch")
)

// CheckHeader validates the first bytes of a merged image: magic 0xE9 and chip id ESP32-C3.
func CheckHeader(head []byte) error {
	if len(head) == 0 {
		return ErrEmpty
	}
	if len(head) < 16 || head[0] != 0xE9 {
		return ErrNotESP
	}
	if chip := binary.LittleEndian.Uint16(head[12:14]); chip != ChipESP32C3 {
		return fmt.Errorf("%w (chip id 0x%04x)", ErrWrongChip, chip)
	}
	return nil
}

// Expect describes what the catalog says a firmware file should be.
type Expect struct {
	Size   int64
	SHA256 string
}

// Verify reads the file once, checking size, header and SHA-256.
func Verify(path string, want Expect) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	if st.Size() == 0 {
		return ErrEmpty
	}
	if st.Size() > MaxSize {
		return ErrTooLarge
	}
	if want.Size > 0 && st.Size() != want.Size {
		return fmt.Errorf("%w: expected %d bytes, got %d", ErrSize, want.Size, st.Size())
	}
	head := make([]byte, 16)
	if _, err := io.ReadFull(f, head); err != nil {
		return ErrNotESP
	}
	if err := CheckHeader(head); err != nil {
		return err
	}
	if want.SHA256 == "" {
		return nil
	}
	h := sha256.New()
	h.Write(head)
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), want.SHA256) {
		return ErrChecksum
	}
	return nil
}
