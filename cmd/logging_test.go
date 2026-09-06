package cmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

func TestLogDirUsesXDGStateHome(t *testing.T) {
	stateHome := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateHome)
	if got, want := logDir(), filepath.Join(stateHome, "gg", "logs"); got != want {
		t.Fatalf("logDir() = %q, want %q", got, want)
	}
}

func TestDailyLogWriterRotatesByLocalDate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "logs")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "2026-09-06.log"), []byte("existing\n"), 0644); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.September, 6, 23, 59, 0, 0, time.Local)
	w, err := newDailyLogWriter(dir, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close() })

	if _, err := w.Write([]byte("first\n")); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Minute)
	if _, err := w.Write([]byte("second\n")); err != nil {
		t.Fatal(err)
	}

	assertFileContent(t, filepath.Join(dir, "2026-09-06.log"), "existing\nfirst\n")
	assertFileContent(t, filepath.Join(dir, "2026-09-07.log"), "second\n")
	info, err := os.Stat(filepath.Join(dir, "2026-09-07.log"))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); runtime.GOOS != "windows" && got != 0600 {
		t.Fatalf("log permissions = %o, want 600", got)
	}
}

func TestDailyLogWriterRejectsUnusableDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(path, []byte("occupied"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := newDailyLogWriter(filepath.Join(path, "logs"), time.Now); err == nil {
		t.Fatal("newDailyLogWriter succeeded with an unusable directory")
	}
}

func TestErrorRoutingFormatterSeparatesLevels(t *testing.T) {
	var console bytes.Buffer
	var logs bytes.Buffer
	logger := logrus.New()
	logger.SetLevel(logrus.TraceLevel)
	logger.SetOutput(&console)
	logger.SetFormatter(&logrus.TextFormatter{DisableColors: true, DisableTimestamp: true})
	configureLogrusErrorRouting(logger, &logs, &logrus.TextFormatter{DisableColors: true, DisableTimestamp: true})

	logger.Info("visible info")
	logger.Warn("hidden warning")
	logger.Error("hidden error")

	if got := console.String(); !strings.Contains(got, "visible info") || strings.Contains(got, "hidden warning") || strings.Contains(got, "hidden error") {
		t.Fatalf("unexpected console output: %q", got)
	}
	if got := logs.String(); strings.Contains(got, "visible info") || !strings.Contains(got, "hidden warning") || !strings.Contains(got, "hidden error") {
		t.Fatalf("unexpected file output: %q", got)
	}
}

func TestErrorRoutingFormatterFallsBackToConsole(t *testing.T) {
	var console bytes.Buffer
	logger := logrus.New()
	logger.SetOutput(&console)
	logger.SetFormatter(&logrus.TextFormatter{DisableColors: true, DisableTimestamp: true})
	configureLogrusErrorRouting(logger, failingWriter{}, &logrus.TextFormatter{DisableColors: true, DisableTimestamp: true})

	logger.Warn("still visible")
	if got := console.String(); !strings.Contains(got, "still visible") {
		t.Fatalf("warning was lost after log write failure: %q", got)
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("%s contains %q, want %q", path, got, want)
	}
}
