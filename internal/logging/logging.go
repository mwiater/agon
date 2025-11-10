// internal/logging/logging.go
package logging

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var (
	mu      sync.Mutex
	logFile *os.File
)

// Init initializes the logging system, setting the output to a file if a path is provided.
func Init(logPath string) error {
	mu.Lock()
	defer mu.Unlock()

	// Close any existing log file
	if logFile != nil {
		_ = logFile.Close()
		logFile = nil
	}

	// If no path is provided, set the logger to discard all output
	if logPath == "" {
		log.SetOutput(io.Discard)
		return nil
	}

	// --- A path was provided, so configure the file logger ---

	// Create directory if it doesn't exist
	if dir := filepath.Dir(logPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	// Open the log file
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}

	logFile = file

	// Set the log output to *only* this file
	log.SetOutput(logFile)
	return nil
}

// Close closes the log file if it's open.
func Close() error {
	mu.Lock()
	defer mu.Unlock()
	if logFile == nil {
		return nil
	}
	log.SetOutput(io.Discard)
	err := logFile.Close()
	logFile = nil
	return err
}

// LogEvent logs a general event message.
func LogEvent(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	log.Println(msg)
}

// LogRequest logs a request/response message with structured data.
func LogRequest(direction, host, model, tool string, payload any) {
	msg := buildRequestMessage(direction, host, model, tool, payload)
	log.Println(msg)
}

// buildRequestMessage constructs a structured log message for a request.
func buildRequestMessage(direction, host, model, tool string, payload any) string {
	dir := strings.TrimSpace(direction)
	if dir != "" {
		dir = strings.ToUpper(dir)
	}
	hostValue := strings.TrimSpace(host)
	if hostValue == "" {
		hostValue = "unknown"
	}
	modelValue := strings.TrimSpace(model)
	if modelValue == "" {
		modelValue = "unknown"
	}
	parts := []string{fmt.Sprintf("[%s]", dir)}
	parts = append(parts, fmt.Sprintf("host=%s", hostValue))
	parts = append(parts, fmt.Sprintf("model=%s", modelValue))
	if tool = strings.TrimSpace(tool); tool != "" {
		parts = append(parts, fmt.Sprintf("tool=%s", tool))
	}
	parts = append(parts, fmt.Sprintf("payload=%s", formatPayload(payload)))
	return strings.Join(parts, " ")
}

// formatPayload formats a payload of any type into a string for logging.
func formatPayload(payload any) string {
	switch v := payload.(type) {
	case nil:
		return "null"
	case string:
		if strings.TrimSpace(v) == "" {
			return `""`
		}
		return v
	case []byte:
		if len(v) == 0 {
			return "[]"
		}
		return string(v)
	case fmt.Stringer:
		return v.String()
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(data)
	}
}
