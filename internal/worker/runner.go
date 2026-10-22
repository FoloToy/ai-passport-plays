// Package worker runs the bundled esptool worker as a separate process.
package worker

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const Name = "ai-passport-plays-worker"

type Port struct {
	Path        string `json:"path"`
	VID         int    `json:"vid"`
	PID         int    `json:"pid"`
	Serial      string `json:"serial"`
	Label       string `json:"label"`
	Recommended bool   `json:"recommended"`
}

type Request struct {
	Command string `json:"command"`
	Port    string `json:"port"`
	Image   string `json:"image"`
	SHA256  string `json:"sha256"`
	Size    int64  `json:"size"`
	Baud    int    `json:"baud"`
}

type Event struct {
	Type    string `json:"type"`
	Stage   string `json:"stage,omitempty"`
	Percent int    `json:"percent,omitempty"`
	Message string `json:"message,omitempty"`
	Code    string `json:"code,omitempty"`
	Chip    string `json:"chip,omitempty"`
	MAC     string `json:"mac,omitempty"`
	Flash   string `json:"flash,omitempty"`
}

// CodedError keeps the worker's error code for translation in the UI.
type CodedError struct {
	Code    string
	Message string
}

func (e *CodedError) Error() string { return e.Message }

type Runner struct {
	Executable string
	Prefix     []string
}

// Discover finds the worker inside the app bundle (macOS) or next to the executable (Windows).
func Discover() (Runner, error) {
	exe, err := os.Executable()
	if err != nil {
		return Runner{}, err
	}
	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}
	dir := filepath.Dir(exe)
	for _, p := range []string{
		filepath.Join(dir, "..", "Resources", "worker", Name+suffix),
		filepath.Join(dir, "worker", Name+suffix),
	} {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return Runner{Executable: p}, nil
		}
	}
	return Runner{}, errors.New("bundled flashing worker not found")
}

func (r Runner) Available() bool { return r.Executable != "" }

func (r Runner) command(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, r.Executable, append(append([]string{}, r.Prefix...), args...)...)
	configure(cmd)
	cmd.WaitDelay = 2 * time.Second
	return cmd
}

func (r Runner) Ports(ctx context.Context) ([]Port, error) {
	if !r.Available() {
		return nil, errors.New("flashing worker unavailable")
	}
	cmd := r.command(ctx, "ports")
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("listing serial ports failed: %v %s", err, strings.TrimSpace(stderr.String()))
	}
	ports := []Port{}
	if err := json.Unmarshal(out.Bytes(), &ports); err != nil {
		return nil, err
	}
	return ports, nil
}

// Flash streams worker events to on until the worker exits.
func (r Runner) Flash(ctx context.Context, req Request, on func(Event)) error {
	if !r.Available() {
		return errors.New("flashing worker unavailable")
	}
	req.Command = "flash"
	cmd := r.command(ctx)
	in, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err = cmd.Start(); err != nil {
		return err
	}
	if err = json.NewEncoder(in).Encode(req); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return err
	}
	_ = in.Close()
	scan := bufio.NewScanner(out)
	scan.Buffer(make([]byte, 4096), 1<<20)
	var workerErr *CodedError
	completed := false
	for scan.Scan() {
		var ev Event
		if json.Unmarshal(scan.Bytes(), &ev) != nil {
			continue
		}
		switch ev.Type {
		case "error":
			workerErr = &CodedError{Code: ev.Code, Message: ev.Message}
		case "complete":
			completed = true
		}
		on(ev)
	}
	waitErr := cmd.Wait()
	if workerErr != nil {
		return workerErr
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if waitErr != nil {
		return fmt.Errorf("worker exited: %v %s", waitErr, strings.TrimSpace(stderr.String()))
	}
	if !completed {
		return errors.New("worker exited without completing")
	}
	return nil
}
