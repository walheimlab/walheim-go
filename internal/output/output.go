package output

import (
	"encoding/json"
	"fmt"
	"os"
)

// ErrorPayload is emitted to stdout when --output json is set and a command fails.
// In human mode, only the Message is printed to stderr.
type ErrorPayload struct {
	Error       string            `json:"error"`       // snake_case error code
	Message     string            `json:"message"`
	Input       map[string]string `json:"input,omitempty"`
	Suggestion  string            `json:"suggestion,omitempty"`
	Retryable   bool              `json:"retryable"`
}

// Errorf prints a structured error. In JSON mode, writes to stdout; otherwise stderr.
func Errorf(jsonMode bool, code, message, suggestion string, input map[string]string, retryable bool) {
	if jsonMode {
		payload := ErrorPayload{
			Error:      code,
			Message:    message,
			Input:      input,
			Suggestion: suggestion,
			Retryable:  retryable,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(payload)
	} else {
		fmt.Fprintf(os.Stderr, "Error: %s\n", message)
		if suggestion != "" {
			fmt.Fprintf(os.Stderr, "Hint:  %s\n", suggestion)
		}
	}
}

// Warnf always writes to stderr regardless of output mode.
func Warnf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "Warning: "+format+"\n", args...)
}
