// cmd/agon/main.go
package main

import (
	"io"
	"log"

	"github.com/mwiater/agon/internal/appconfig"
	cmd "github.com/mwiater/agon/internal/commands"
	"github.com/mwiater/agon/internal/logging"
	"github.com/mwiater/agon/internal/metrics"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

var (
	loadConfig     = appconfig.Load
	initLogging    = logging.Init
	closeLogging   = logging.Close
	getMetrics     = metrics.GetInstance
	setVersionInfo = cmd.SetVersionInfo
	executeCmd     = cmd.Execute
)

// main is the entry point for the agon CLI application.
func main() {
	log.SetOutput(io.Discard) // Discard all log output by default

	cfg, err := loadConfig("")
	if err != nil {
		log.Printf("could not load config: %v", err)
	}

	// Initialize logging based on config
	if err := initLogging(cfg.LogFilePath()); err != nil {
		log.Fatalf("failed to initialize logging: %v", err)
	}
	defer closeLogging()

	// Always get the instance to ensure it's initialized
	aggregator := getMetrics()

	if cfg.Metrics {
		aggregator.SetMetricsEnabled(true)
	}
	defer metrics.Close()

	setVersionInfo(version, commit, date)
	executeCmd()
}
