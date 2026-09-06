package cmd

import (
	"fmt"
	"io"
	stdlog "log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

const logDateLayout = "2006-01-02"

type dailyLogWriter struct {
	mu       sync.Mutex
	dir      string
	now      func() time.Time
	date     string
	file     *os.File
}

func stateHome() string {
	if stateHome := os.Getenv("XDG_STATE_HOME"); stateHome != "" {
		return stateHome
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".local", "state")
	}
	return ""
}

func logDir() string {
	stateDir := stateHome()
	if stateDir == "" {
		return ""
	}
	return filepath.Join(stateDir, "gg", "logs")
}

func newDailyLogWriter(dir string, now func() time.Time) (*dailyLogWriter, error) {
	if dir == "" {
		return nil, fmt.Errorf("cannot determine the user state directory")
	}
	if now == nil {
		now = time.Now
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("create log directory %q: %w", dir, err)
	}
	w := &dailyLogWriter{dir: dir, now: now}
	w.mu.Lock()
	err := w.rotateLocked()
	w.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return w, nil
}

func (w *dailyLogWriter) rotateLocked() error {
	date := w.now().Format(logDateLayout)
	if w.file != nil && w.date == date {
		return nil
	}
	path := filepath.Join(w.dir, date+".log")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("open log file %q: %w", path, err)
	}
	if err := file.Chmod(0600); err != nil {
		_ = file.Close()
		return fmt.Errorf("set permissions on log file %q: %w", path, err)
	}
	oldFile := w.file
	w.file = file
	w.date = date
	if oldFile != nil {
		_ = oldFile.Close()
	}
	return nil
}

func (w *dailyLogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.rotateLocked(); err != nil {
		return 0, err
	}
	return w.file.Write(p)
}

func (w *dailyLogWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}

type errorRoutingFormatter struct {
	console logrus.Formatter
	file    logrus.Formatter
	output  io.Writer
}

func (f *errorRoutingFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	if entry.Level > logrus.WarnLevel {
		return f.console.Format(entry)
	}
	data, err := f.file.Format(entry)
	if err != nil {
		return nil, err
	}
	if _, err := f.output.Write(data); err != nil {
		// Preserve visibility if the daily log cannot be written after startup.
		return f.console.Format(entry)
	}
	return nil, nil
}

type fallbackWriter struct {
	primary  io.Writer
	fallback io.Writer
}

func (w fallbackWriter) Write(p []byte) (int, error) {
	n, err := w.primary.Write(p)
	if err == nil {
		return n, nil
	}
	return w.fallback.Write(p)
}

func redirectCLIErrorLogs(logger *logrus.Logger) error {
	w, err := newDailyLogWriter(logDir(), time.Now)
	if err != nil {
		return err
	}
	fileFormatter := &logrus.TextFormatter{
		DisableColors:   true,
		FullTimestamp:   true,
		TimestampFormat: time.RFC3339,
	}
	configureLogrusErrorRouting(logger, w, fileFormatter)
	if standard := logrus.StandardLogger(); standard != logger {
		configureLogrusErrorRouting(standard, w, fileFormatter)
	}
	stdlog.SetOutput(fallbackWriter{primary: w, fallback: os.Stderr})
	return nil
}

func configureLogrusErrorRouting(logger *logrus.Logger, output io.Writer, fileFormatter logrus.Formatter) {
	logger.SetFormatter(&errorRoutingFormatter{
		console: logger.Formatter,
		file:    fileFormatter,
		output:  output,
	})
}
