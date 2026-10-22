package settings

import (
	"os/exec"
	"strings"
)

// SystemLanguage returns "zh" or "en" from the user's preferred macOS languages.
func SystemLanguage() string {
	out, err := exec.Command("defaults", "read", "-g", "AppleLanguages").Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.Trim(strings.TrimSpace(line), `",()`)
		if line != "" {
			return languageFrom(line)
		}
	}
	return ""
}
