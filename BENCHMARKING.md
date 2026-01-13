# Benchmarking

## 1) Generating modelmetadata

Use `agon fetch modelmetadata` to collect model metadata from configured hosts. This writes JSON files to `agonData/modelMetadata/` for downstream accuracy analysis.

**Example**
```bash
agon fetch modelmetadata --endpoints http://localhost:8080,http://localhost:8081 --gpu radeon-rx-570
```

## 2) Generating Benchmarks

Benchmark mode runs a common prompt against models in parallel for n iterations. Your configuration file **must only have one model per host**. There is no UI in this mode; it repeats requests to build average response timing.

Start from the provided example file and edit hosts/models as needed:

```bash
cp configs/config.example.BenchmarkMode.json configs/config.benchmark.json
```

Open `configs/config.benchmark.json` and update host URLs and model IDs so each host has a single model.

If you want a standalone benchmark server, use `servers/benchmark` (llama.cpp only). Configure `servers/benchmark/agon-benchmark.yml` with a `models_path` pointing to your GGUF directory, then run the server and POST to `/benchmark` with the model filename (relative to `models_path`) or an absolute path.

Benchmark results are written to `agonData/modelBenchmarks/`.

Benchmark mode uses the following definitions:

```
  "benchmarkMode": true,
  "benchmarkCount": 10,
```

**Examples**
```bash
agon benchmark models --config configs/config.benchmark.json
```

```bash
agon benchmark model --model llama-3-2-1b-instruct-q8_0.gguf --gpu radeon-rx-570 --benchmark-endpoint http://localhost:9999/benchmark
```
