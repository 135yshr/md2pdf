package converter

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestRunMmdc_CancelStopsTheCommand is the regression guard for the reason the
// context is threaded at all: without it an interrupted run left mmdc, and the
// browser mmdc starts, running to completion.
//
// The stub forks rather than execing, so the sleep it starts outlives the
// shell that CommandContext kills and inherits its output pipes. That is what
// mmdc does too — it starts a browser — and without a WaitDelay the read of
// those pipes holds the conversion open for the grandchild's full lifetime.
func TestRunMmdc_CancelStopsTheCommand(t *testing.T) {
	const stubSleep = 30 * time.Second

	dir := t.TempDir()
	stub := filepath.Join(dir, "mmdc")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\nsleep 30\n"), 0o755); err != nil {
		t.Fatalf("write stub mmdc: %v", err)
	}

	c := newTestConverter(t, &Config{MmdcPath: stub})

	ctx, cancel := context.WithCancel(t.Context())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	err := c.runMmdc(ctx, filepath.Join(dir, "d.mmd"), filepath.Join(dir, "d.svg"),
		"graph TD\n  A --> B\n", "")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("runMmdc on a canceled context = nil, want the command to have been stopped")
	}
	if elapsed >= stubSleep {
		t.Errorf("runMmdc took %v: the cancellation never reached the command", elapsed)
	}
}
