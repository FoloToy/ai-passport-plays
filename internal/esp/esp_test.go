package esp

import (
	"bytes"
	"compress/zlib"
	"crypto/md5"
	"encoding/hex"
	"os"
	"strings"
	"testing"
	"time"
)

// bufPort is an in-memory Port: Read serves rx, Write appends to tx.
type bufPort struct {
	rx, tx bytes.Buffer
	dtr    bool
}

func (b *bufPort) Read(p []byte) (int, error)         { n, _ := b.rx.Read(p); return n, nil }
func (b *bufPort) Write(p []byte) (int, error)        { return b.tx.Write(p) }
func (b *bufPort) SetReadTimeout(time.Duration) error { return nil }
func (b *bufPort) SetDTR(v bool) error                { b.dtr = v; return nil }
func (b *bufPort) SetRTS(bool) error                  { return nil }
func (b *bufPort) DTR() bool                          { return b.dtr }
func (b *bufPort) SetBaudRate(int) error              { return nil }
func (b *bufPort) BaudRate() int                      { return 115200 }
func (b *bufPort) ResetInputBuffer() error            { return nil }
func (b *bufPort) Close() error                       { return nil }

func TestSLIPRoundTrip(t *testing.T) {
	p := &bufPort{}
	l := New(p)
	payload := []byte{0x01, 0xC0, 0xDB, 0x02, 0xC0}
	if err := l.writePacket(payload); err != nil {
		t.Fatal(err)
	}
	want := []byte{0xC0, 0x01, 0xDB, 0xDC, 0xDB, 0xDD, 0x02, 0xDB, 0xDC, 0xC0}
	if !bytes.Equal(p.tx.Bytes(), want) {
		t.Fatalf("encoded %x, want %x", p.tx.Bytes(), want)
	}
	// Boot noise before the frame must be skipped.
	p.rx.WriteString("ESP-ROM:esp32c3\r\n")
	p.rx.Write(want)
	got, err := l.readPacket(50 * time.Millisecond)
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("decoded %x %v, want %x", got, err, payload)
	}
	if _, err = l.readPacket(10 * time.Millisecond); err != ErrTimeout {
		t.Fatalf("want timeout, got %v", err)
	}
}

func TestCheckStatus(t *testing.T) {
	p := &bufPort{}
	l := New(p)
	// READ_REG response: val=0x12345678, status ok (ROM sends 4 status bytes).
	resp := []byte{1, opReadReg, 4, 0, 0x78, 0x56, 0x34, 0x12, 0, 0, 0, 0}
	enc := New(&bufPort{})
	_ = enc.writePacket(resp)
	p.rx.Write(enc.port.(*bufPort).tx.Bytes())
	v, err := l.ReadReg(0x60008844)
	if err != nil || v != 0x12345678 {
		t.Fatalf("got %x %v", v, err)
	}
	// Failure status.
	enc = New(&bufPort{})
	_ = enc.writePacket([]byte{1, opReadReg, 4, 0, 0, 0, 0, 0, 1, 5, 0, 0})
	p.rx.Write(enc.port.(*bufPort).tx.Bytes())
	if _, err = l.ReadReg(0); err == nil || !strings.Contains(err.Error(), "status 01 05") {
		t.Fatalf("want status error, got %v", err)
	}
}

func TestUncompressedPerBlock(t *testing.T) {
	data := bytes.Repeat([]byte{0xFF}, 3<<20)
	copy(data[1<<20:], bytes.Repeat([]byte("noise-"), 100000))
	var c bytes.Buffer
	zw, _ := zlib.NewWriterLevel(&c, zlib.BestCompression)
	zw.Write(data)
	zw.Close()
	got, err := uncompressedPerBlock(c.Bytes(), 64)
	if err != nil {
		t.Fatal(err)
	}
	if got[len(got)-1] != len(data) {
		t.Fatalf("last block reaches %d, want %d", got[len(got)-1], len(data))
	}
	for i := 1; i < len(got); i++ {
		if got[i] < got[i-1] {
			t.Fatal("not monotonic")
		}
	}
}

func TestStubEmbedded(t *testing.T) {
	n, err := StubSize()
	if err != nil || n < 4096 {
		t.Fatalf("stub size %d %v", n, err)
	}
}

// TestQEMU runs against Espressif QEMU started with
// -serial tcp:127.0.0.1:5555,server,nowait and strap_mode=0x02 (download mode).
// Set ESP_QEMU_ADDR=127.0.0.1:5555 to run it. It rewrites the bootloader and
// partition table and checks that cardid is unchanged.
func TestQEMU(t *testing.T) {
	addr := os.Getenv("ESP_QEMU_ADDR")
	if addr == "" {
		t.Skip("ESP_QEMU_ADDR not set")
	}
	port, err := DialTCP(addr)
	if err != nil {
		t.Fatal(err)
	}
	l := New(port)
	l.Log = func(s string) { t.Log(s) }
	defer l.Close()
	if err = l.Connect(nil, 3); err != nil {
		t.Fatal(err)
	}
	chip, err := l.Chip()
	if err != nil || chip != "ESP32-C3" {
		t.Fatalf("chip %q %v", chip, err)
	}
	mac, err := l.MAC()
	t.Logf("MAC %s %v", mac, err)
	sb, fe, err := l.Security()
	if err != nil || sb || fe {
		t.Fatalf("security %v %v %v", sb, fe, err)
	}
	if err = l.RunStub(); err != nil {
		t.Fatal(err)
	}
	size, id, err := l.FlashSize()
	if err != nil || size != 8<<20 {
		t.Fatalf("flash size %d id %06x %v", size, id, err)
	}
	if err = l.AttachFlash(size); err != nil {
		t.Fatal(err)
	}
	if err = l.ChangeBaud(460800); err != nil {
		t.Fatal(err)
	}
	cardid, err := l.ReadFlash(0x356000, 0x4000, nil)
	if err != nil {
		t.Fatal(err)
	}
	boot, _ := os.ReadFile("../factory/firmware/bootloader.bin")
	part, _ := os.ReadFile("../factory/firmware/partition-table.bin")
	tail := bytes.Repeat([]byte{0xFF}, 0x10000) // compressible, crosses 4 MB
	tail[0] = 0x42
	start := time.Now()
	if err = l.WriteFlash([]Segment{{0, boot}, {0x8000, part}, {0x400000, tail}}, nil); err != nil {
		t.Fatal(err)
	}
	t.Logf("write took %s", time.Since(start))
	after, err := l.ReadFlash(0x356000, 0x4000, nil)
	if err != nil || !bytes.Equal(after, cardid) {
		t.Fatalf("cardid changed (%v)", err)
	}
	sum, err := l.FlashMD5(0, len(boot))
	want := md5.Sum(boot)
	if err != nil || sum != hex.EncodeToString(want[:]) {
		t.Fatalf("md5 %s %v", sum, err)
	}
}
