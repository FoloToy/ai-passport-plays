//go:build !windows

package device

// problemPortHints only applies to Windows USB Code 43.
func problemPortHints() []string { return nil }
