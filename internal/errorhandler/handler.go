// Package errorhandler provides the application's shared error and panic boundary.
package errorhandler

import (
	"fmt"
	"log/slog"
	"runtime/debug"
)

// Handler reports errors with an operation name and recovers panics at process
// boundaries such as Discord event callbacks.
type Handler struct {
	logger *slog.Logger
}

// New creates a Handler. A nil logger uses slog.Default().
func New(logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}

	return &Handler{logger: logger}
}

// Error records an error and returns it unchanged, which makes it convenient
// to use in return paths while keeping logging consistent.
func (h *Handler) Error(operation string, err error) error {
	if err == nil {
		return nil
	}

	h.logger.Error("operation failed", "operation", operation, "error", err)
	return err
}

// Run executes fn and converts a panic into a logged error. A panic in one
// event callback must not terminate the Discord process.
func (h *Handler) Run(operation string, fn func()) {
	defer func() {
		if recovered := recover(); recovered != nil {
			h.logger.Error(
				"panic recovered",
				"operation", operation,
				"panic", recovered,
				"stack", string(debug.Stack()),
			)
		}
	}()

	fn()
}

// Errorf is useful for failures that do not already have an error value.
func (h *Handler) Errorf(operation, format string, args ...any) {
	h.Error(operation, fmt.Errorf(format, args...))
}
