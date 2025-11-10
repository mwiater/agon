// internal/metrics/provider.go
package metrics

import (
	"context"
	"time"

	"github.com/mwiater/agon/internal/appconfig"
	"github.com/mwiater/agon/internal/logging"
	"github.com/mwiater/agon/internal/providers"
)

// Provider is a decorator that wraps a ChatProvider to record metrics.
type Provider struct {
	wrapped          providers.ChatProvider
	aggregator       *Aggregator
	startTime        time.Time
	firstChunkTime   time.Time
}

// NewProvider creates a new metrics-enabled provider that wraps an existing ChatProvider.
func NewProvider(wrapped providers.ChatProvider, aggregator *Aggregator) *Provider {
	logging.LogEvent("[METRICS] Wrapping provider with metrics provider")
	return &Provider{wrapped: wrapped, aggregator: aggregator}
}

// Stream intercepts the call to the wrapped provider's Stream method to record performance metrics.
func (p *Provider) Stream(ctx context.Context, req providers.StreamRequest, callbacks providers.StreamCallbacks) error {
	logging.LogEvent("[METRICS] Stream called on metrics provider for model %s", req.Model)
	p.startTime = time.Now()
	firstChunkReceived := false

	onChunk := func(chunk providers.ChatMessage) error {
		if !firstChunkReceived {
			p.firstChunkTime = time.Now()
			firstChunkReceived = true
		}
		if callbacks.OnChunk != nil {
			return callbacks.OnChunk(chunk)
		}
		return nil
	}

	onComplete := func(meta providers.StreamMetadata) error {
		logging.LogEvent("[METRICS] onComplete called for model %s", meta.Model)
		if p.aggregator != nil {
			ttft := int64(0)
			if firstChunkReceived {
				ttft = p.firstChunkTime.Sub(p.startTime).Milliseconds()
			}
			p.aggregator.Record(meta, ttft)
		}

		if callbacks.OnComplete != nil {
			return callbacks.OnComplete(meta)
		}
		return nil
	}

	newCallbacks := providers.StreamCallbacks{
		OnChunk:    onChunk,
		OnComplete: onComplete,
	}

	return p.wrapped.Stream(ctx, req, newCallbacks)
}

// LoadedModels passes the call through to the wrapped provider.
func (p *Provider) LoadedModels(ctx context.Context, host appconfig.Host) ([]string, error) {
	return p.wrapped.LoadedModels(ctx, host)
}

// EnsureModelReady passes the call through to the wrapped provider.
func (p *Provider) EnsureModelReady(ctx context.Context, host appconfig.Host, model string) error {
	return p.wrapped.EnsureModelReady(ctx, host, model)
}

// Close passes the call through to the wrapped provider.
func (p *Provider) Close() error {
	return p.wrapped.Close()
}