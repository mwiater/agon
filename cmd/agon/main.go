// cmd/agon/main.go
package main

import (
	"log"

	"github.com/mwiater/agon/internal/appconfig"
	cmd "github.com/mwiater/agon/internal/cli"
	"github.com/mwiater/agon/internal/metrics"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// main is the entry point for the agon CLI application.
func main() {
	cfg, err := appconfig.Load("")
	if err != nil {
		log.Printf("could not load config: %v", err)
	}

	if cfg.Metrics {
		metrics.GetInstance()
		defer metrics.Close()
	}

	cmd.SetVersionInfo(version, commit, date)
	cmd.Execute()
}