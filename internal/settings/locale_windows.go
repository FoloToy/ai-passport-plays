package settings

import "syscall"

var getUserDefaultUILanguage = syscall.NewLazyDLL("kernel32.dll").NewProc("GetUserDefaultUILanguage")

// SystemLanguage returns "zh" or "en" from the Windows UI language.
func SystemLanguage() string {
	id, _, _ := getUserDefaultUILanguage.Call()
	if id&0x3ff == 0x04 { // LANG_CHINESE
		return "zh"
	}
	return "en"
}
