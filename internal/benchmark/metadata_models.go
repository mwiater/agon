package benchmark

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mwiater/agon/internal/models"
)

const modelMetadataDir = "agonData/modelMetadata"

// RunBenchmarkModelsFromMetadata runs benchmarks for metadata entries matching the GPU.
func RunBenchmarkModelsFromMetadata(gpu, benchmarkServer string) error {
	gpu = strings.TrimSpace(gpu)
	if gpu == "" {
		return fmt.Errorf("missing required --gpu flag")
	}
	if strings.Contains(gpu, " ") || strings.Contains(gpu, "_") {
		return fmt.Errorf("--gpu value must not contain spaces or underscores")
	}

	benchmarkServer = normalizeEndpoint(benchmarkServer)
	if benchmarkServer == "" {
		return fmt.Errorf("missing required --benchmark-server flag")
	}

	entries, err := os.ReadDir(modelMetadataDir)
	if err != nil {
		return fmt.Errorf("read metadata dir %s: %w", modelMetadataDir, err)
	}

	var matches []string
	seen := make(map[string]struct{})

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".json") {
			continue
		}

		path := filepath.Join(modelMetadataDir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read metadata file %s: %w", path, err)
		}

		var meta models.ModelMeta
		if err := json.Unmarshal(data, &meta); err != nil {
			return fmt.Errorf("parse metadata file %s: %w", path, err)
		}

		metaGPU := strings.TrimSpace(meta.GPU)
		if metaGPU != gpu {
			continue
		}
		key := strings.Join([]string{metaGPU, meta.Name}, "\x00")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		matches = append(matches, meta.Name)
	}

	if len(matches) == 0 {
		return fmt.Errorf("no model metadata found for gpu %s", gpu)
	}

	for _, name := range matches {
		if err := RunBenchmarkModel(name, gpu, benchmarkServer); err != nil {
			return err
		}
	}

	return nil
}

func normalizeEndpoint(raw string) string {
	return strings.TrimRight(strings.TrimSpace(raw), "/")
}
