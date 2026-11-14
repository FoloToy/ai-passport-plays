package main

import (
	"log"
	"path/filepath"

	"github.com/FoloToy/ai-passport-desktop/assets"
	"github.com/FoloToy/ai-passport-desktop/internal/desktop"
	"github.com/FoloToy/ai-passport-desktop/pkg/flasher"
	"github.com/FoloToy/ai-passport-desktop/pkg/syncer"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// runUI shows the UI in a native WebView window (macOS, Windows 10 and later).
func runUI(server *desktop.Server, flash *flasher.Manager, sync *syncer.Manager) {
	app := application.New(application.Options{
		Name:        appTitle,
		Description: "Offline AI Passport play library and flasher",
		Icon:        assets.Icon,
		Assets:      application.AssetOptions{Handler: server},
		Mac:         application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: true},
		ShouldQuit:  func() bool { return !flash.Active() },
		OnShutdown:  sync.Cancel,
	})
	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title: appTitle, Width: 1280, Height: 860, MinWidth: 980, MinHeight: 680, URL: "/",
		BackgroundColour: application.NewRGB(12, 12, 13),
	})
	server.ChooseDir = func(title, current string) (string, error) {
		d := app.Dialog.OpenFile().SetTitle(title).
			CanChooseFiles(false).CanChooseDirectories(true).CanCreateDirectories(true).AttachToWindow(window)
		if current != "" {
			d = d.SetDirectory(filepath.Dir(current))
		}
		return d.PromptForSingleSelection()
	}
	server.OpenURL = app.Browser.OpenURL
	window.OnWindowEvent(events.Common.WindowClosing, func(e *application.WindowEvent) {
		if flash.Active() {
			e.Cancel()
			app.Dialog.Info().SetTitle("正在操作设备 / Device busy").SetMessage("请等待刷机或恢复出厂完成后再关闭窗口，否则设备可能无法启动。\nWait for flashing or the factory restore to finish before closing.").Show()
		}
	})
	app.Menu.Set(application.DefaultApplicationMenu())
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
