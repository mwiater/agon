# Benchmarking

## 1) Generating modelmetadata

Use `agon fetch modelmetadata` to collect model metadata from configured hosts. This writes JSON files to `agonData/modelMetadata/` for downstream accuracy analysis.

**Example**
```bash
agon fetch modelmetadata --endpoints http://localhost:8080,http://localhost:8081 --gpu radeon-rx-570
```

## 2) Running Benchmarks

Use the standalone benchmark server in `servers/benchmark` (llama.cpp only). Configure `servers/benchmark/agon-benchmark.yml` with a `models_path` pointing to your GGUF directory, then run the server and POST to `/benchmark` with the model filename (relative to `models_path`) or an absolute path.

Benchmark results are written to `agonData/modelBenchmarks/`.

**Example**
```bash
go run cmd/agon/main.go benchmark model \
  --model OpenELM-1_1B.gguf \
  --gpu pentium-n3710-1-60ghz \
  --benchmark-endpoint http://192.168.0.91:9999/benchmark
```
