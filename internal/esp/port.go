// Package esp talks to the ESP32-C3 ROM bootloader and flasher stub over a serial port.
//
// It is a Go port of the parts of esptool (https://github.com/espressif/esptool) that
// AI Passport needs: connect, identify, run the flasher stub, compressed write,
// read, MD5 and reset. The flasher stub itself (stub/esp32c3.json) comes from
// esp-flasher-stub and is dual licensed Apache-2.0 / MIT.
package esp

import (
	"errors"
	"net"
	"os"
	"time"

	"go.bug.st/serial"
)

// Port is the transport the loader needs. A real serial port and a TCP socket
// (for the QEMU test harness) both implement it.
type Port interface {
	Read(p []byte) (int, error)
	Write(p []byte) (int, error)
	// SetReadTimeout makes Read return (0, nil) when no data arrives in time.
	SetReadTimeout(time.Duration) error
	SetDTR(bool) error
	SetRTS(bool) error
	DTR() bool
	SetBaudRate(int) error
	BaudRate() int
	ResetInputBuffer() error
	Close() error
}

type serialPort struct {
	serial.Port
	dtr  bool
	baud int
}

// OpenSerial opens a serial port with DTR and RTS released, so opening it does not
// reset the chip.
func OpenSerial(path string, baud int) (Port, error) {
	p, err := serial.Open(path, &serial.Mode{
		BaudRate:          baud,
		DataBits:          8,
		Parity:            serial.NoParity,
		StopBits:          serial.OneStopBit,
		InitialStatusBits: &serial.ModemOutputBits{DTR: false, RTS: false},
	})
	if err != nil {
		return nil, err
	}
	return &serialPort{Port: p, baud: baud}, nil
}

func (s *serialPort) SetDTR(v bool) error {
	s.dtr = v
	return s.Port.SetDTR(v)
}

func (s *serialPort) DTR() bool     { return s.dtr }
func (s *serialPort) BaudRate() int { return s.baud }

func (s *serialPort) SetBaudRate(baud int) error {
	if err := s.Port.SetMode(&serial.Mode{BaudRate: baud, DataBits: 8, Parity: serial.NoParity, StopBits: serial.OneStopBit}); err != nil {
		return err
	}
	s.baud = baud
	return nil
}

// tcpPort is used with QEMU's "-serial tcp:..." option. Modem lines and baud rate
// are ignored, so it only works with ResetNone.
type tcpPort struct {
	conn    net.Conn
	timeout time.Duration
	dtr     bool
	baud    int
}

// DialTCP connects to an emulated serial port such as QEMU's.
func DialTCP(addr string) (Port, error) {
	c, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		return nil, err
	}
	return &tcpPort{conn: c, timeout: time.Second, baud: 115200}, nil
}

func (t *tcpPort) Read(p []byte) (int, error) {
	_ = t.conn.SetReadDeadline(time.Now().Add(t.timeout))
	n, err := t.conn.Read(p)
	if err != nil && errors.Is(err, os.ErrDeadlineExceeded) {
		return n, nil
	}
	return n, err
}

func (t *tcpPort) Write(p []byte) (int, error) {
	_ = t.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return t.conn.Write(p)
}

func (t *tcpPort) SetReadTimeout(d time.Duration) error { t.timeout = d; return nil }
func (t *tcpPort) SetDTR(v bool) error                  { t.dtr = v; return nil }
func (t *tcpPort) SetRTS(bool) error                    { return nil }
func (t *tcpPort) DTR() bool                            { return t.dtr }
func (t *tcpPort) SetBaudRate(b int) error              { t.baud = b; return nil }
func (t *tcpPort) BaudRate() int                        { return t.baud }
func (t *tcpPort) Close() error                         { return t.conn.Close() }

func (t *tcpPort) ResetInputBuffer() error {
	buf := make([]byte, 4096)
	for {
		_ = t.conn.SetReadDeadline(time.Now().Add(time.Millisecond))
		n, err := t.conn.Read(buf)
		if n == 0 || err != nil {
			return nil
		}
	}
}
