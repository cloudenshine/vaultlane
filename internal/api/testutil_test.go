package api

import (
	"io"
	"log/slog"
)

// slogNew 测试用 logger（丢弃所有输出）
func slogNew() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}
