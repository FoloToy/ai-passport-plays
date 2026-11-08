package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"time"

	"github.com/FoloToy/ai-passport-plays/assets"
	"github.com/FoloToy/ai-passport-plays/internal/desktop"
	"github.com/FoloToy/ai-passport-plays/internal/flasher"
	"github.com/FoloToy/ai-passport-plays/internal/instance"
	"github.com/FoloToy/ai-passport-plays/internal/settings"
	"github.com/FoloToy/ai-passport-plays/internal/syncer"
	"github.com/FoloToy/ai-passport-plays/internal/worker"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

var version = "0.2.0-dev"

const appTitle = "AI Passport 玩法社区"

func main() {
	headless := flag.Bool("headless", false, "Serve the UI on a private loopback HTTP listener (development)")
	address := flag.String("listen", "127.0.0.1:0", "Loopback listener for -headless")
	workerPath := flag.String("worker", "", "Worker executable or Python interpreter (development)")
	workerScript := flag.String("worker-script", "", "Worker script run by -worker (development)")
	stateDir := flag.String("state-dir", "", "Override the private settings directory")
	origin := flag.String("origin", syncer.DefaultOrigin, "Community site origin")
	flag.Parse()

	dir := *stateDir
	if dir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			log.Fatal(err)
		}
		dir = filepath.Join(base, "FoloToy", "AI Passport Plays")
	}
	store, err := settings.Open(dir)
	if err != nil {
		log.Fatal(err)
	}
	unlock, err := instance.Acquire(dir)
	if err != nil {
		log.Fatal(err)
	}
	defer unlock()

	runner, werr := worker.Discover()
	if *workerPath != "" {
		runner, werr = worker.Runner{Executable: *workerPath}, nil
		if *workerScript != "" {
			runner.Prefix = []string{*workerScript}
		}
	}
	if werr != nil {
		log.Print(werr)
	}

	sync := syncer.New(syncer.NewClient(*origin, version))
	flash := flasher.New(runner)
	server := desktop.New(version, store, sync, flash, runner)
	server.Reveal = reveal
	server.StateDir = dir
	server.CheckUpdatesInBackground()

	if *headless {
		serveHeadless(server, *address, sync)
		return
	}

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

func reveal(path string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", path).Start()
	case "windows":
		return exec.Command("explorer", path).Start()
	default:
		return exec.Command("xdg-open", path).Start()
	}
}

func serveHeadless(server *desktop.Server, address string, sync *syncer.Manager) {
	host, _, err := net.SplitHostPort(address)
	if err != nil || host != "127.0.0.1" {
		log.Fatal("development listener must use 127.0.0.1")
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		log.Fatal(err)
	}
	server.OpenURL = func(url string) error { log.Print("open ", url); return nil }
	httpServer := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Host != listener.Addr().String() {
				http.Error(w, "Invalid Host", http.StatusForbidden)
				return
			}
			server.ServeHTTP(w, r)
		}),
		ReadHeaderTimeout: 5 * time.Second,
	}
	fmt.Println(appTitle + " " + version + " http://" + listener.Addr().String())
	go func() {
		if err := httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Print(err)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	<-stop
	sync.Cancel()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(ctx)
}
