package chat

import (
	"context"
	"log"

	"github.com/mwiater/agon/internal/appconfig"
	"github.com/mwiater/agon/internal/metrics"
)

// Run starts the chat UI based on the current configuration.
func Run(
	cfg *appconfig.Config,
	startGUI func(context.Context, *appconfig.Config, context.CancelFunc),
	startPipelineGUI func(context.Context, *appconfig.Config, context.CancelFunc) error,
) {
	ctx, cancel := context.WithCancel(context.Background())

	metrics.GetInstance().SetMetricsEnabled(true)
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
