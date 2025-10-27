package logging

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/mwiater/agon/internal/mcplog"
)

func LogEvent(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	log.Println(msg)
	mcplog.Write(msg)
}

func LogRequest(direction, host, model, tool string, payload any) {
	msg := buildRequestMessage(direction, host, model, tool, payload)
	log.Println(msg)
	mcplog.Write(msg)
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
