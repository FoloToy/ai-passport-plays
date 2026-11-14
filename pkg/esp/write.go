package esp

import (
	"bytes"
	"compress/zlib"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

// Segment is data to write at a flash offset.
type Segment struct {
	Offset int
	Data   []byte
}

// WriteFlash writes the segments with compressed transfers, checks each one against
// the chip's MD5, and leaves the stub running. Flash mode, frequency and size bytes in
// the image header are kept as they are. progress (may be nil) gets the bytes written
// across all segments.
func (l *Loader) WriteFlash(segs []Segment, progress func(done, total int)) error {
	if !l.stub {
		return errors.New("writing flash needs the flasher stub")
	}
	total := 0
	for _, s := range segs {
		total += len(s.Data)
		if l.flashSize > 0 && s.Offset+len(s.Data) > l.flashSize {
			return fmt.Errorf("data at 0x%x does not fit in the %s flash", s.Offset, SizeName(l.flashSize))
		}
	}
	done := 0
	for _, s := range segs {
		if err := l.writeSegment(s, func(n int) {
			if progress != nil {
				progress(done+n, total)
			}
		}); err != nil {
			return err
		}
		done += len(s.Data)
	}
	// Leave compressed mode but keep the stub running (1 = do not reboot).
	if _, _, err := l.check("leave compressed flash mode", opFlashDeflEnd, le(1), 0, 0, defaultTimeout); err != nil {
		return err
	}
	return nil
}

func (l *Loader) writeSegment(s Segment, progress func(int)) error {
	var comp bytes.Buffer
	zw, _ := zlib.NewWriterLevel(&comp, zlib.BestCompression)
	_, _ = zw.Write(s.Data)
	_ = zw.Close()
	c := comp.Bytes()
	blocks := (len(c) + stubWriteBlock - 1) / stubWriteBlock
	written, err := uncompressedPerBlock(c, stubWriteBlock)
	if err != nil {
		return err
	}
	l.logf("Writing %d bytes at 0x%08x (%d compressed)", len(s.Data), s.Offset, len(c))
	if _, _, err = l.check("enter compressed flash mode", opFlashDeflBegin,
		le(uint32(len(s.Data)), uint32(blocks), stubWriteBlock, uint32(s.Offset), 0), 0, 0, defaultTimeout); err != nil {
		return err
	}
	for i := range blocks {
		chunk := c[i*stubWriteBlock : min((i+1)*stubWriteBlock, len(c))]
		payload := append(le(uint32(len(chunk)), uint32(i), 0, 0), chunk...)
		// The stub erases as it writes, so allow for the data this block expands to.
		timeout := timeoutPerMB(writePerMB, written[i]-prev(written, i)+sectorSize)
		for attempt := 1; ; attempt++ {
			_, _, err = l.check(fmt.Sprintf("write compressed block %d", i), opFlashDeflData, payload, checksum(chunk), 0, timeout)
			if err == nil {
				break
			}
			if attempt == writeAttempts {
				return err
			}
			l.logf("Retrying block %d: %v", i, err)
		}
		progress(written[i])
	}
	want := md5.Sum(s.Data)
	got, err := l.FlashMD5(s.Offset, len(s.Data))
	if err != nil {
		return err
	}
	if got != hex.EncodeToString(want[:]) {
		return fmt.Errorf("verify at 0x%x failed: flash MD5 %s, expected %x", s.Offset, got, want)
	}
	l.logf("Hash of data at 0x%08x verified", s.Offset)
	return nil
}

func prev(v []int, i int) int {
	if i == 0 {
		return 0
	}
	return v[i-1]
}

// byteCounter lets flate read byte by byte so the input position is exact.
type byteCounter struct {
	data []byte
	pos  int
}

func (b *byteCounter) Read(p []byte) (int, error) {
	if b.pos >= len(b.data) {
		return 0, io.EOF
	}
	n := copy(p, b.data[b.pos:])
	b.pos += n
	return n, nil
}

func (b *byteCounter) ReadByte() (byte, error) {
	if b.pos >= len(b.data) {
		return 0, io.EOF
	}
	b.pos++
	return b.data[b.pos-1], nil
}

// uncompressedPerBlock returns, for each compressed block, how many uncompressed bytes
// the stream has produced once that block has been consumed. It decides write timeouts.
func uncompressedPerBlock(comp []byte, block int) ([]int, error) {
	blocks := (len(comp) + block - 1) / block
	out := make([]int, blocks)
	src := &byteCounter{data: comp}
	zr, err := zlib.NewReader(src)
	if err != nil {
		return nil, err
	}
	buf := make([]byte, 4096)
	total, i := 0, 0
	for {
		n, err := zr.Read(buf)
		total += n
		for i < blocks && src.pos > (i+1)*block {
			out[i] = total
			i++
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	for ; i < blocks; i++ {
		out[i] = total
	}
	return out, nil
}
