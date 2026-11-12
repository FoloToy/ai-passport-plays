package esp

import "time"

// Pin levels follow esptool: asserting a modem line (true) pulls the pin low.
const (
	pinLow  = true
	pinHigh = false
)

// Reset puts the chip into the ROM download mode before a connect attempt.
type Reset func(Port)

// setRTS re-applies DTR after RTS: Windows usbser.sys only sends the line state
// when both are written.
func setRTS(p Port, v bool) {
	_ = p.SetRTS(v)
	_ = p.SetDTR(p.DTR())
}

// USBJTAGReset is esptool's USB-JTAG/Serial bootloader reset. esptool waits 100 ms per
// step; the fast capture uses 5 ms for firmware that only keeps USB up briefly.
func USBJTAGReset(step time.Duration) Reset {
	return func(p Port) {
		setRTS(p, pinHigh)
		_ = p.SetDTR(pinHigh)
		time.Sleep(step)
		_ = p.SetDTR(pinLow)
		setRTS(p, pinHigh)
		time.Sleep(step)
		setRTS(p, pinLow)
		_ = p.SetDTR(pinHigh)
		setRTS(p, pinLow)
		time.Sleep(step)
		_ = p.SetDTR(pinHigh)
		setRTS(p, pinHigh)
	}
}

// ClassicReset is esptool's reset for USB-UART bridges wired to EN and IO9.
// esptool alternates delay 50 ms and 550 ms between attempts.
func ClassicReset(delay time.Duration) Reset {
	return func(p Port) {
		_ = p.SetDTR(false)
		setRTS(p, true)
		time.Sleep(100 * time.Millisecond)
		_ = p.SetDTR(true)
		setRTS(p, false)
		time.Sleep(delay)
		_ = p.SetDTR(false)
	}
}

// DefaultResets is esptool's default-reset sequence for the port type.
func DefaultResets(usb bool) []Reset {
	if usb {
		return []Reset{USBJTAGReset(100 * time.Millisecond)}
	}
	return []Reset{ClassicReset(50 * time.Millisecond), ClassicReset(550 * time.Millisecond)}
}

// HardReset pulses EN to run the application. usb adds the delays esptool uses for
// chips on their own USB peripheral, which re-enumerate during reset.
func HardReset(p Port, usb bool) {
	hold, after := 100*time.Millisecond, time.Duration(0)
	if usb {
		hold, after = 200*time.Millisecond, 200*time.Millisecond
	}
	setRTS(p, pinLow)
	time.Sleep(hold)
	setRTS(p, pinHigh)
	time.Sleep(after)
}
