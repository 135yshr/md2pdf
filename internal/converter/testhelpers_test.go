package converter

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"
)

// skipOnWindows skips tests that stand in for an external binary with a shell
// script.
func skipOnWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("test uses /bin/sh; skipping on Windows")
	}
}

// writeShellScript writes an executable stand-in for an external binary that
// exits with exitCode, optionally after printing stderrText.
func writeShellScript(t *testing.T, path string, exitCode int, stderrText string) {
	t.Helper()
	var b strings.Builder
	b.WriteString("#!/bin/sh\n")
	if stderrText != "" {
		b.WriteString("cat 1>&2 <<'EOF'\n")
		b.WriteString(stderrText)
		b.WriteString("\nEOF\n")
	}
	fmt.Fprintf(&b, "exit %d\n", exitCode)
	if err := os.WriteFile(path, []byte(b.String()), 0o755); err != nil { //nolint:gosec // an executable stub is the point
		t.Fatalf("write shell script %q: %v", path, err)
	}
}
