package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
)

func New(level, format string) (*slog.Logger, error) {
	handler, err := newHandler(os.Stdout, level, format)
	if err != nil {
		return nil, err
	}

	return slog.New(handler), nil
}

func newHandler(w io.Writer, level, format string) (slog.Handler, error) {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		return nil, fmt.Errorf("parse log level %q: %w", level, err)
	}

	opts := &slog.HandlerOptions{Level: lvl}

	switch format {
	case "text":
		return slog.NewTextHandler(w, opts), nil
	case "json":
		return slog.NewJSONHandler(w, opts), nil
	default:
		return nil, fmt.Errorf("unknown log format %q", format)
	}
}
