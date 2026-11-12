//go:build win7

package main

// The Windows 7 / 8 / 8.1 edition. WebView2 and current Go do not run there, so this
// build is compiled with go-legacy-win7 and shows the same UI in Chrome (109 is the
// last version for Windows 7) on a private 127.0.0.1 address. The console window
// stays open while the app runs; closing it quits.

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/FoloToy/ai-passport-plays/internal/desktop"
	"github.com/FoloToy/ai-passport-plays/internal/driver"
	"github.com/FoloToy/ai-passport-plays/internal/flasher"
	"github.com/FoloToy/ai-passport-plays/internal/syncer"
)

const edition = "win7"

const urlFile = "ui-url.txt"

// alreadyRunning reopens the running instance's page instead of starting a second one.
func alreadyRunning(dir string, err error) {
	if b, rerr := os.ReadFile(filepath.Join(dir, urlFile)); rerr == nil && strings.HasPrefix(string(b), "http://127.0.0.1:") {
		openBrowser(strings.TrimSpace(string(b)))
		return
	}
	fmt.Println(err)
	pause()
}

func runUI(server *desktop.Server, flash *flasher.Manager, sync *syncer.Manager) {
	server.ChooseDir = chooseDir
	server.OpenURL = func(url string) error { openBrowser(url); return nil }
	server.DriverStatus = driver.Check
	server.InstallDriver = func() error { return driver.Install(filepath.Join(server.StateDir, "driver")) }
	stateFile := filepath.Join(server.StateDir, urlFile)
	serveLoopback(server, "127.0.0.1:0", sync, func(url string) {
		_ = os.WriteFile(stateFile, []byte(url), 0o600)
		fmt.Println(appTitle + " " + version + "（Windows 7 版 / Windows 7 edition）")
		fmt.Println()
		fmt.Println("界面已在浏览器中打开：" + url)
		fmt.Println("The app is open in your browser: " + url)
		fmt.Println()
		fmt.Println("请保持这个窗口打开；关闭它就会退出程序。刷机或恢复出厂时不要关闭。")
		fmt.Println("Keep this window open; closing it quits the app. Never close it while flashing.")
		if !openBrowser(url) {
			fmt.Println()
			fmt.Println("没有找到 Chrome。请安装 Chrome 109，再把上面的地址复制到 Chrome 里打开。")
			fmt.Println("Chrome was not found. Install Chrome 109, then open the address above in it.")
		}
	})
	<-interrupted()
	_ = os.Remove(stateFile)
	sync.Cancel()
}

// browsers lists Chrome, then Edge (both 109 on Windows 7). Internet Explorer cannot
// run the UI, so the system default browser is only the last resort.
func browsers() []string {
	var out []string
	for _, base := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)"), os.Getenv("ProgramW6432"), os.Getenv("LOCALAPPDATA")} {
		if base == "" {
			continue
		}
		out = append(out, filepath.Join(base, "Google", "Chrome", "Application", "chrome.exe"))
	}
	for _, base := range []string{os.Getenv("ProgramFiles(x86)"), os.Getenv("ProgramFiles")} {
		if base != "" {
			out = append(out, filepath.Join(base, "Microsoft", "Edge", "Application", "msedge.exe"))
		}
	}
	return out
}

// openBrowser shows url in an app-style Chrome or Edge window. It reports false when
// neither is installed; the default browser is tried anyway.
func openBrowser(url string) bool {
	for _, b := range browsers() {
		if st, err := os.Stat(b); err == nil && !st.IsDir() {
			if exec.Command(b, "--app="+url).Start() == nil {
				return true
			}
		}
	}
	_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	return false
}

// chooseDir shows the Windows folder picker through PowerShell 2.0, which Windows 7
// ships. Paths travel as base64 UTF-8 so Chinese folder names survive the console.
func chooseDir(title, current string) (string, error) {
	enc := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
	script := `$u=[Text.Encoding]::UTF8
Add-Type -AssemblyName System.Windows.Forms
$d=New-Object System.Windows.Forms.FolderBrowserDialog
$d.Description=$u.GetString([Convert]::FromBase64String('` + enc(title) + `'))
$d.SelectedPath=$u.GetString([Convert]::FromBase64String('` + enc(current) + `'))
$d.ShowNewFolderButton=$true
$f=New-Object System.Windows.Forms.Form -Property @{TopMost=$true}
if($d.ShowDialog($f) -eq [System.Windows.Forms.DialogResult]::OK){[Console]::Out.Write([Convert]::ToBase64String($u.GetBytes($d.SelectedPath)))}`
	cmd := exec.Command("powershell.exe", "-NoProfile", "-STA", "-ExecutionPolicy", "Bypass", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("folder picker: %w", err)
	}
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(out)))
	if err != nil {
		return "", fmt.Errorf("folder picker: %w", err)
	}
	return string(b), nil
}

func pause() {
	fmt.Println("按回车键关闭 / Press Enter to close")
	_, _ = fmt.Scanln()
}
