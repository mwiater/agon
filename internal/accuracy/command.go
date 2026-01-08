package accuracy

import (
	"log"

	"github.com/mwiater/agon/internal/appconfig"
)

// RunAccuracyCommand is the CLI entry point for accuracy.
func RunAccuracyCommand(cfg *appconfig.Config) error {
	log.Println("accuracy command called")
	if cfg == nil {
		log.Println("config is nil")
		return nil
	}
	log.Printf("accuracy mode: %v", cfg.AccuracyMode)
	return RunAccuracy(cfg)
}
