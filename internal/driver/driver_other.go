//go:build !windows

package driver

import "errors"

// Check always reports OK outside Windows.
func Check() string { return OK }

// Install is only needed on Windows 7 and 8.
func Install(string) error { return errors.New("driver install is only needed on Windows 7 and 8") }
