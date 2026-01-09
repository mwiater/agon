package chat

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/mwiater/agon/internal/appconfig"
)

func TestRunNilConfigUsesGUI(t *testing.T) {
	calledGUI := 0
	calledPipeline := 0

	Run(nil,
		func(ctx context.Context, cfg *appconfig.Config, cancel context.CancelFunc) {
			if ctx == nil || cancel == nil {
				t.Fatalf("expected context and cancel")
			}
			if cfg != nil {
				t.Fatalf("expected nil config")
			}
			calledGUI++
		},
		func(ctx context.Context, cfg *appconfig.Config, cancel context.CancelFunc) error {
			calledPipeline++
			return nil
		},
	)

	if calledGUI != 1 {
		t.Fatalf("expected GUI to run once, got %d", calledGUI)
	}
	if calledPipeline != 0 {
		t.Fatalf("expected pipeline not called, got %d", calledPipeline)
	}
}

func TestRunPipelineMode(t *testing.T) {
	cfg := &appconfig.Config{PipelineMode: true}
	calledGUI := 0
	calledPipeline := 0

	Run(cfg,
		func(ctx context.Context, cfg *appconfig.Config, cancel context.CancelFunc) {
			calledGUI++
		},
		func(ctx context.Context, cfg *appconfig.Config, cancel context.CancelFunc) error {
			calledPipeline++
			return nil
		},
	)

	if calledPipeline != 1 {
		t.Fatalf("expected pipeline to run once, got %d", calledPipeline)
	}
	if calledGUI != 0 {
		t.Fatalf("expected GUI not called, got %d", calledGUI)
	}
}

func TestRunPipelineErrorFatal(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") == "1" {
		cfg := &appconfig.Config{PipelineMode: true}
		Run(cfg,
			func(ctx context.Context, cfg *appconfig.Config, cancel context.CancelFunc) {},
			func(ctx context.Context, cfg *appconfig.Config, cancel context.CancelFunc) error {
				return errors.New("boom")
			},
		)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestRunPipelineErrorFatal")
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected non-zero exit, got success: %s", output)
	}
	if !strings.Contains(string(output), "Error running pipeline program") {
		t.Fatalf("expected fatal log output, got: %s", output)
	}
}
