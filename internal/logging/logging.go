// Package logging provides Inflate's rotating log + crash log.
// Provider request/response bodies must never be passed in.
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

const maxLogBytes int64 = 10 * 1024 * 1024

// Init opens (and rotates if needed) the log file, returning a *slog.Logger.
// dir is the directory where inflate.log + crash.log live.
func Init(dir string) (*slog.Logger, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "inflate.log")
	if info, err := os.Stat(path); err == nil && info.Size() > maxLogBytes {
		_ = os.Rename(path, path+".1")
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		// disk full or otherwise unwritable → log to stderr only
		return slog.New(slog.NewTextHandler(os.Stderr, nil)), nil
	}
	return slog.New(slog.NewTextHandler(io.MultiWriter(f, os.Stderr), &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})), nil
}

// WriteCrash writes a panic stack to crash.log. Best-effort.
func WriteCrash(dir, body string) {
	_ = os.MkdirAll(dir, 0o755)
	f, err := os.OpenFile(filepath.Join(dir, "crash.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintln(f, body)
}
