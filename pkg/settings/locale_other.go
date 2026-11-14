//go:build !darwin && !windows

package settings

import "os"

// SystemLanguage returns "zh" or "en" from the LANG environment variable.
func SystemLanguage() string { return languageFrom(os.Getenv("LANG")) }
