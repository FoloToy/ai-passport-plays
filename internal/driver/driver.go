// Package driver finds an AI Passport whose USB serial interface has no driver and
// installs one. Windows 10 and later bind usbser.sys to the ESP32-C3 USB-JTAG/Serial
// port by themselves; Windows 7 and 8 do not, so the device shows a yellow
// exclamation mark and never gets a COM port.
package driver

import _ "embed"

// Status values reported to the UI.
const (
	OK      = "ok"      // no AI Passport without a driver was found
	Missing = "missing" // an AI Passport is plugged in but its serial port has no driver
)

// INF binds Windows' own usbser.sys to USB\VID_303A&PID_1001&MI_00. It carries no
// catalog, so Windows asks the user to confirm the publisher once.
//
//go:embed ai-passport-usb-serial.inf
var INF []byte

// InfName is the file name the INF is written under before installing.
const InfName = "ai-passport-usb-serial.inf"
