package slog

import (
	"log/slog"
	"os"

	"github.com/joeycumines/logiface"
)

func ExampleNewLogger() {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	logger := logiface.New[*Event](NewLogger(handler))

	logger.Info().Str("component", "example").Log("logiface-slog adapter working")
	logger.Debug().Str("debug", "data").Log("this should not appear (level filtered)")
}
