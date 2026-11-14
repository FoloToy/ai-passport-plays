// Package factory holds the AI Passport factory firmware and the rules for restoring it.
//
// The firmware is written as four separate segments, never as one merged image, so the
// per-device identity area (cardid: SN and keys) between them is never touched.
// Do not change offsets, sizes or hashes without re-checking the partition table.
package factory

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const (
	// Version is the factory firmware release bundled with the app.
	Version = "AI-Passport-Factory-Default-v1.1.2"
	// FlashSize is the only flash size the factory image supports (8 MB).
	FlashSize = 0x800000
	// CardIDOffset and CardIDSize locate the per-device identity partition.
	CardIDOffset = 0x356000
	CardIDSize   = 0x4000
)

// Segment is one file written at a fixed flash offset.
type Segment struct {
	Offset int64  `json:"offset"`
	File   string `json:"file"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

func (s Segment) End() int64 { return s.Offset + s.Size }

// Segments come from the factory release's flasher_args.json.
var Segments = []Segment{
	{0x000000, "bootloader.bin", 22496, "fcbfb4e8045425054639ef344d858d96b36d6540c8f2a9d26cb00c78f7af03f7"},
	{0x008000, "partition-table.bin", 3072, "590cd71c08f65e3af9cd7bc74047b6ba3eeda319ca4f1be6ebbf99cb7facea9c"},
	{0x010000, "trae_card.bin", 2665312, "94410524091d19c963ee102a2319ab9dccf424f391d25777dc9ec2cded87495f"},
	{0x3FA000, "imgava.bin", 1048576, "88f223c10302372bdfcbdf0135270958014232fa4c00854ccc11f9407b4d1481"},
}

//go:embed firmware/*.bin
var files embed.FS

var ErrCorrupt = errors.New("factory firmware failed its integrity check")

// CheckLayout fails if any segment would overlap cardid or run past the flash.
func CheckLayout(segs []Segment) error {
	end := int64(CardIDOffset + CardIDSize)
	for _, s := range segs {
		if s.Offset < end && s.End() > CardIDOffset {
			return fmt.Errorf("%s overlaps the cardid identity area", s.File)
		}
		if s.Offset < 0 || s.End() > FlashSize {
			return fmt.Errorf("%s does not fit in 8 MB flash", s.File)
		}
	}
	return nil
}

// Verify checks the embedded files against the segment table.
func Verify() error {
	if err := CheckLayout(Segments); err != nil {
		return err
	}
	for _, s := range Segments {
		b, err := fs.ReadFile(files, "firmware/"+s.File)
		if err != nil {
			return fmt.Errorf("%w: %s missing", ErrCorrupt, s.File)
		}
		if err := check(s, b); err != nil {
			return err
		}
	}
	return nil
}

func check(s Segment, b []byte) error {
	if int64(len(b)) != s.Size {
		return fmt.Errorf("%w: %s is %d bytes, want %d", ErrCorrupt, s.File, len(b), s.Size)
	}
	sum := sha256.Sum256(b)
	if hex.EncodeToString(sum[:]) != s.SHA256 {
		return fmt.Errorf("%w: %s SHA-256 mismatch", ErrCorrupt, s.File)
	}
	return nil
}

// Extract writes the verified segments to dir/<Version> and returns that directory.
// Files already present with the right content are reused.
func Extract(dir string) (string, error) {
	if err := Verify(); err != nil {
		return "", err
	}
	out := filepath.Join(dir, Version)
	if err := os.MkdirAll(out, 0o755); err != nil {
		return "", err
	}
	for _, s := range Segments {
		dest := filepath.Join(out, s.File)
		if b, err := os.ReadFile(dest); err == nil && check(s, b) == nil {
			continue
		}
		b, _ := fs.ReadFile(files, "firmware/"+s.File)
		tmp := dest + ".tmp"
		if err := os.WriteFile(tmp, b, 0o644); err != nil {
			return "", err
		}
		if err := os.Rename(tmp, dest); err != nil {
			_ = os.Remove(tmp)
			return "", err
		}
	}
	return out, nil
}
