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

func Init(logPath string) error {
    mu.Lock()
    defer mu.Unlock()

	if logFile != nil {
		_ = logFile.Close()
		logFile = nil
	}

	var writers []io.Writer
	writers = append(writers, os.Stdout)

    if logPath != "" {
		if dir := filepath.Dir(logPath); dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
		}
		file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		logFile = file
		writers = append(writers, logFile)
    }

	log.SetOutput(io.MultiWriter(writers...))
	return nil
}

func Close() error {
	mu.Lock()
	defer mu.Unlock()
	if logFile == nil {
		return nil
	}
	log.SetOutput(os.Stderr)
	err := logFile.Close()
	logFile = nil
	return err
}

func LogEvent(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	log.Println(msg)
}

func LogRequest(direction, host, model, tool string, payload any) {
	msg := buildRequestMessage(direction, host, model, tool, payload)
	log.Println(msg)
}

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
