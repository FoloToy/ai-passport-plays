package worker

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// TestHelperWorker acts as a fake worker when the test binary runs itself.
func TestHelperWorker(t *testing.T) {
	if os.Getenv("PLAYS_FAKE_WORKER") != "1" {
		return
	}
	in := bufio.NewScanner(os.Stdin)
	in.Scan()
	var req Request
	_ = json.Unmarshal(in.Bytes(), &req)
	fmt.Printf(`{"type":"stage","stage":"waiting"}` + "\n")
	if req.Command != "restore" || !req.FullBackup {
		fmt.Printf(`{"type":"error","code":"protocol","message":"bad request"}` + "\n")
		os.Exit(1)
	}
	for in.Scan() {
		if strings.Contains(in.Text(), `"cancel"`) {
			fmt.Printf(`{"type":"error","code":"cancelled","message":"stopped"}` + "\n")
			os.Exit(1)
		}
	}
	os.Exit(2)
}

func TestRestoreDeliversCancel(t *testing.T) {
	t.Setenv("PLAYS_FAKE_WORKER", "1")
	r := Runner{Executable: os.Args[0], Prefix: []string{"-test.run=^TestHelperWorker$"}}
	cancel := make(chan struct{})
	ctx, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()
	err := r.Restore(ctx, Request{FullBackup: true}, func(ev Event) {
		if ev.Type == "stage" && ev.Stage == "waiting" {
			close(cancel)
		}
	}, cancel)
	var ce *CodedError
	if !errors.As(err, &ce) || ce.Code != "cancelled" {
		t.Fatalf("want cancelled, got %v", err)
	}
}
