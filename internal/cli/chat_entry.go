package agon

import (
	"context"
	"log"

	"github.com/mwiater/agon/internal/metrics"
)

func runChat() {
	ctx, cancel := context.WithCancel(context.Background())

	cfg := GetConfig()
	metrics.GetInstance().SetMetricsEnabled(true) // Enable metrics for chat mode
	if cfg == nil {
		startGUI(ctx, cfg, cancel)
		return
	}

	if cfg.PipelineMode {
		if err := startPipelineGUI(ctx, cfg, cancel); err != nil {
			log.Fatalf("Error running pipeline program: %v", err)
		}
		return
	}

	startGUI(ctx, cfg, cancel)
}
