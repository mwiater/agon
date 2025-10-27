package mcplog

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

var (
	mu      sync.Mutex
	logFile *os.File
)

func ensureFile() (*os.File, error) {
	if logFile != nil {
		return logFile, nil
	}
	file, err := os.OpenFile("agon-mcp-server.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	logFile = file
	return logFile, nil
}

func Close() error {
	mu.Lock()
	defer mu.Unlock()
	if logFile == nil {
		return nil
	}
	err := logFile.Close()
	logFile = nil
	return err
}

func Write(message string) {
	if strings.TrimSpace(message) == "" {
		return
	}
	writeLine(message)
}

func Writef(format string, args ...any) {
	writeLine(fmt.Sprintf(format, args...))
}

func writeLine(line string) {
	mu.Lock()
	defer mu.Unlock()

	file, err := ensureFile()
	if err != nil {
		return
	}
	timestamp := time.Now().Format(time.RFC3339)
	fmt.Fprintf(file, "[%s] %s\n", timestamp, line)
}
