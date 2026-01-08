package benchmark

import (
	"log"

	"github.com/mwiater/agon/internal/appconfig"
	"github.com/mwiater/agon/internal/metrics"
)

// RunBenchmarkModels is the CLI entry point for benchmark models.
func RunBenchmarkModels(cfg *appconfig.Config) error {
	metrics.GetInstance().SetMetricsEnabled(true)
	if cfg == nil {
		log.Println("config is nil")
		return nil
	}
	log.Printf("benchmark mode: %v", cfg.BenchmarkMode)
	return BenchmarkModels(cfg)
}
