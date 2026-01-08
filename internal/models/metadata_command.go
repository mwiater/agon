package models

import (
	"fmt"
	"strings"
)

// FetchModelMetadataFromEndpoints is the CLI entry point for fetch modelmetadata.
func FetchModelMetadataFromEndpoints(urls []string, gpu string) error {
	if len(urls) == 0 {
		return fmt.Errorf("missing required --endpoints flag")
	}
	if strings.Contains(gpu, " ") || strings.Contains(gpu, "_") {
		return fmt.Errorf("--gpu value must not contain spaces or underscores")
	}

	modelNames := FetchEndpointModelNames(urls)
	FetchModelMetadata(modelNames, gpu)
	return nil
}
