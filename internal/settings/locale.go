package settings

import "strings"

// languageFrom maps a locale such as "zh-Hans-CN" or "en_US" to "zh" or "en".
func languageFrom(locale string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(locale)), "zh") {
		return "zh"
	}
	if locale == "" {
		return ""
	}
	return "en"
}
