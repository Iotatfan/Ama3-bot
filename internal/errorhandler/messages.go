package errorhandler

import (
	"context"
	"errors"
	"net"
	"net/http"

	"github.com/openai/openai-go/v3"
)

const (
	openAI400Message     = "The request could not be interpreted reliably. I will not continue on uncertain input."
	openAI401Message     = "That operation is outside my current permissions."
	openAI404Message     = "I searched for it. It is not there, or no longer exists."
	openAI429Message     = "Too many requests in too short a time. The system requires a brief recovery interval."
	openAI500Message     = "**An internal fault interrupted my assessment.**\nThe failure originated within my systems, not your request. Try again shortly."
	openAITimeoutMessage = "I lost contact with the required system before the operation completed. The connection may recover if you try again."
	openAIUnknownMessage = "I encountered a fault I cannot classify. Continuing without understanding the failure would be irresponsible."
)

// OpenAIErrorMessage converts an OpenAI or transport error into a safe,
// user-facing message without exposing provider details.
func OpenAIErrorMessage(err error) string {
	if err == nil {
		return ""
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return openAITimeoutMessage
	}

	var networkErr net.Error
	if errors.As(err, &networkErr) && networkErr.Timeout() {
		return openAITimeoutMessage
	}

	var apiErr *openai.Error
	if !errors.As(err, &apiErr) {
		return openAIUnknownMessage
	}

	switch apiErr.StatusCode {
	case http.StatusBadRequest:
		return openAI400Message
	case http.StatusUnauthorized, http.StatusForbidden:
		return openAI401Message
	case http.StatusNotFound:
		return openAI404Message
	case http.StatusTooManyRequests:
		return openAI429Message
	case http.StatusInternalServerError:
		return openAI500Message
	default:
		return openAIUnknownMessage
	}
}
