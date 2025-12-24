package rag

import (
	"fmt"
	"strings"

	"github.com/mwiater/agon/internal/appconfig"
)

func embeddingHost(cfg *appconfig.Config) (appconfig.Host, error) {
	if strings.TrimSpace(cfg.RagEmbeddingHost) == "" {
		return appconfig.Host{}, fmt.Errorf("ragEmbeddingHost is required when ragMode is enabled")
	}
	for _, host := range cfg.Hosts {
		if host.Name == cfg.RagEmbeddingHost {
			return host, nil
		}
	}
	return appconfig.Host{}, fmt.Errorf("ragEmbeddingHost %q not found in config hosts", cfg.RagEmbeddingHost)
}
