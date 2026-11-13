package main

import (
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
	"syscall"
	"time"

	"github.com/FoloToy/ai-passport-plays/internal/desktop"
	"github.com/FoloToy/ai-passport-plays/internal/device"
	"github.com/FoloToy/ai-passport-plays/internal/esp"
	"github.com/FoloToy/ai-passport-plays/internal/factory"
	"github.com/FoloToy/ai-passport-plays/internal/flasher"
	"github.com/FoloToy/ai-passport-plays/internal/instance"
	"github.com/FoloToy/ai-passport-plays/internal/settings"
	"github.com/FoloToy/ai-passport-plays/internal/syncer"
)

var version = "0.4.1-dev"

const appTitle = "AI Passport 玩法社区"

func main() {
	headless := flag.Bool("headless", false, "Serve the UI on a private loopback HTTP listener (development)")
	address := flag.String("listen", "127.0.0.1:0", "Loopback listener for -headless")
	selfTest := flag.Bool("self-test", false, "Check the bundled flasher stub and factory firmware, print JSON and exit")
	stateDir := flag.String("state-dir", "", "Override the private settings directory")
	origin := flag.String("origin", syncer.DefaultOrigin, "Community site origin")
	flag.Parse()
	if *selfTest {
		runSelfTest()
		return
	}

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
		alreadyRunning(dir, err)
		return
	}
	defer unlock()

	runner := device.Runner{}
	sync := syncer.New(syncer.NewClient(*origin, version))
	flash := flasher.New(runner)
	server := desktop.New(version, store, sync, flash, runner)
	server.Reveal = reveal
	server.StateDir = dir
	server.CheckUpdatesInBackground()

	if *headless {
		serveLoopback(server, *address, sync, func(url string) { fmt.Println(appTitle + " " + version + " " + url) })
		<-interrupted()
		return
	}

	runUI(server, flash, sync)
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

// serveLoopback serves the UI on a private 127.0.0.1 listener (development, and the
// Windows 7 edition, which shows it in Chrome). ready gets the page URL. The server
// stops when the process exits.
func serveLoopback(server *desktop.Server, address string, sync *syncer.Manager, ready func(url string)) {
	host, _, err := net.SplitHostPort(address)
	if err != nil || host != "127.0.0.1" {
		log.Fatal("the UI listener must use 127.0.0.1")
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		log.Fatal(err)
	}
	if server.OpenURL == nil {
		server.OpenURL = func(url string) error { log.Print("open ", url); return nil }
	}
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
	go func() {
		if err := httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Print(err)
		}
	}()
	ready("http://" + listener.Addr().String() + "/")
}

// interrupted is closed by Ctrl+C or, on Windows, by closing the console window.
func interrupted() <-chan os.Signal {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	return stop
}

// runSelfTest is used by the packaging scripts to check the built app.
func runSelfTest() {
	stub, err := esp.StubSize()
	if err == nil {
		err = factory.Verify()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "self-test failed:", err)
		os.Exit(1)
	}
	fmt.Printf(`{"passed":true,"version":%q,"edition":%q,"chip":"ESP32-C3","stub_bytes":%d,"factory":%q}`+"\n", version, edition, stub, factory.Version)
}
