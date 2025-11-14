// cmd/agon/main.go
package main

import (
	"io"
	"log"

	"github.com/mwiater/agon/internal/appconfig"
	cmd "github.com/mwiater/agon/internal/cli"
	"github.com/mwiater/agon/internal/logging"
	"github.com/mwiater/agon/internal/metrics"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// main is the entry point for the agon CLI application.
func main() {
	log.SetOutput(io.Discard) // Discard all log output by default

	cfg, err := appconfig.Load("")
	if err != nil {
		log.Printf("could not load config: %v", err)
	}

	// Initialize logging based on config
	if err := logging.Init(cfg.LogFilePath()); err != nil {
		log.Fatalf("failed to initialize logging: %v", err)
	}
	defer logging.Close()

	// Always get the instance to ensure it's initialized
	aggregator := metrics.GetInstance()

	if cfg.Metrics {
		aggregator.SetMetricsEnabled(true)
	}
	defer metrics.Close()

	cmd.SetVersionInfo(version, commit, date)
	cmd.Execute()
}
