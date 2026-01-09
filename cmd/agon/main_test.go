package main

import (
	"testing"

	"github.com/mwiater/agon/internal/appconfig"
	"github.com/mwiater/agon/internal/metrics"
)

func TestMainWiring(t *testing.T) {
	origLoadConfig := loadConfig
	origInitLogging := initLogging
	origCloseLogging := closeLogging
	origGetMetrics := getMetrics
	origSetVersion := setVersionInfo
	origExecute := executeCmd
	t.Cleanup(func() {
		loadConfig = origLoadConfig
		initLogging = origInitLogging
		closeLogging = origCloseLogging
		getMetrics = origGetMetrics
		setVersionInfo = origSetVersion
		executeCmd = origExecute
	})

	calls := struct {
		load    bool
		initLog bool
		close   bool
		metrics bool
		version bool
		exec    bool
	}{}

	loadConfig = func(path string) (appconfig.Config, error) {
		calls.load = true
		if path != "" {
			t.Fatalf("expected empty path, got %q", path)
		}
		return appconfig.Config{LogFile: "test.log", Metrics: false}, nil
	}
	initLogging = func(path string) error {
		calls.initLog = true
		if path != "test.log" {
			t.Fatalf("expected log path test.log, got %q", path)
		}
		return nil
	}
	closeLogging = func() error {
		calls.close = true
		return nil
	}
	getMetrics = func() *metrics.Aggregator {
		calls.metrics = true
		return metrics.NewAggregator()
	}
	setVersionInfo = func(v, c, d string) {
		calls.version = true
		if v == "" || c == "" || d == "" {
			t.Fatalf("expected version info to be set")
		}
	}
	executeCmd = func() {
		calls.exec = true
	}

	main()

	if !calls.load || !calls.initLog || !calls.close || !calls.metrics || !calls.version || !calls.exec {
		t.Fatalf("expected all wiring calls, got %+v", calls)
	}
}
