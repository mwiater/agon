package mcplog

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/mwiater/agon/internal/appconfig"
)

var mu sync.Mutex

// Write appends a formatted message to mcp-debug.log when debug mode is enabled.
func Write(cfg *appconfig.Config, format string, args ...any) {
	if cfg == nil || !cfg.Debug {
		return
	}

	msg := fmt.Sprintf(format, args...)

	mu.Lock()
	defer mu.Unlock()

	file, err := os.OpenFile("mcp-debug.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer file.Close()

	timestamp := time.Now().Format(time.RFC3339)
	fmt.Fprintf(file, "[%s] %s\n", timestamp, msg)
}
