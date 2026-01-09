// internal/metrics/analyzeMetrics.go
package metrics

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// AccuracyRecord mirrors a single JSONL entry from agonData/modelAccuracy.
type AccuracyRecord struct {
	Timestamp           time.Time       `json:"timestamp"`
	Host                string          `json:"host"`
	Model               string          `json:"model"`
	PromptID            int             `json:"promptId"`
	Prompt              string          `json:"prompt"`
	ExpectedAnswer      json.RawMessage `json:"expectedAnswer"`
	Response            string          `json:"response"`
	LogProbs            *LogProbs       `json:"logprobs,omitempty"`
	Correct             bool            `json:"correct"`
	MarginOfError       float64         `json:"marginOfError"`
	Difficulty          int             `json:"difficulty"`
	TimeToFirstToken    int64           `json:"time_to_first_token"`
	TokensPerSecond     float64         `json:"tokens_per_second"`
	InputTokens         int             `json:"input_tokens"`
	OutputTokens        int             `json:"output_tokens"`
	TotalDurationMs     int64           `json:"total_duration_ms"`
	TotalTokens         int             `json:"total_tokens,omitempty"`
	CacheN              int             `json:"cache_n,omitempty"`
	PromptN             int             `json:"prompt_n,omitempty"`
	PromptMs            float64         `json:"prompt_ms,omitempty"`
	PromptPerTokenMs    float64         `json:"prompt_per_token_ms,omitempty"`
	PromptPerSecond     float64         `json:"prompt_per_second,omitempty"`
	PredictedN          int             `json:"predicted_n,omitempty"`
	PredictedMs         float64         `json:"predicted_ms,omitempty"`
	PredictedPerTokenMs float64         `json:"predicted_per_token_ms,omitempty"`
	PredictedPerSecond  float64         `json:"predicted_per_second,omitempty"`
	DeadlineExceeded    bool            `json:"deadlineExceeded"`
	DeadlineTimeout     int64           `json:"deadlineTimeout"`
	ParameterTemplate   string          `json:"parameterTemplate,omitempty"`
}

// LogProbs captures token-level likelihood details from accuracy records.
type LogProbs struct {
	Content []LogProbToken `json:"content"`
}

// LogProbToken captures a single token's logprob payload.
type LogProbToken struct {
	ID          int          `json:"id"`
	Token       string       `json:"token"`
	Bytes       []int        `json:"bytes"`
	LogProb     float64      `json:"logprob"`
	TopLogProbs []TopLogProb `json:"top_logprobs"`
}

// TopLogProb captures alternative token probabilities.
type TopLogProb struct {
	ID      int     `json:"id"`
	Token   string  `json:"token"`
	Bytes   []int   `json:"bytes"`
	LogProb float64 `json:"logprob"`
}

// BenchmarkRun mirrors a single benchmark entry from agonData/modelBenchmarks.
type BenchmarkRun struct {
	BuildCommit         string    `json:"build_commit"`
	BuildNumber         int       `json:"build_number"`
	CPUInfo             string    `json:"cpu_info"`
	GPUInfo             string    `json:"gpu_info"`
	Backends            string    `json:"backends"`
	ModelFilename       string    `json:"model_filename"`
	ModelType           string    `json:"model_type"`
	ModelSize           int64     `json:"model_size"`
	ModelNParams        int64     `json:"model_n_params"`
	NBatch              int       `json:"n_batch"`
	NUBatch             int       `json:"n_ubatch"`
	NThreads            int       `json:"n_threads"`
	CPUMask             string    `json:"cpu_mask"`
	CPUStrict           bool      `json:"cpu_strict"`
	Poll                int       `json:"poll"`
	TypeK               string    `json:"type_k"`
	TypeV               string    `json:"type_v"`
	NGPULayers          int       `json:"n_gpu_layers"`
	NCPUMoe             int       `json:"n_cpu_moe"`
	SplitMode           string    `json:"split_mode"`
	MainGPU             int       `json:"main_gpu"`
	NoKVOffload         bool      `json:"no_kv_offload"`
	FlashAttn           bool      `json:"flash_attn"`
	Devices             string    `json:"devices"`
	TensorSplit         string    `json:"tensor_split"`
	TensorBuftOverrides string    `json:"tensor_buft_overrides"`
	UseMmap             bool      `json:"use_mmap"`
	Embeddings          bool      `json:"embeddings"`
	NoOpOffload         int       `json:"no_op_offload"`
	NoHost              bool      `json:"no_host"`
	NPrompt             int       `json:"n_prompt"`
	NGen                int       `json:"n_gen"`
	NDepth              int       `json:"n_depth"`
	TestTime            time.Time `json:"test_time"`
	AvgNs               int64     `json:"avg_ns"`
	StddevNs            int64     `json:"stddev_ns"`
	AvgTs               float64   `json:"avg_ts"`
	StddevTs            float64   `json:"stddev_ts"`
	SamplesNs           []int64   `json:"samples_ns"`
	SamplesTs           []float64 `json:"samples_ts"`
}

// ModelMetadataDocument mirrors a single metadata entry from agonData/modelMetadata.
type ModelMetadataDocument struct {
	Type     string        `json:"type"`
	Name     string        `json:"name"`
	Endpoint string        `json:"endpoint"`
	GPU      string        `json:"gpu"`
	Metadata ModelMetadata `json:"metadata"`
}

// ModelMetadata captures the nested metadata payload used by the endpoint.
type ModelMetadata struct {
	BOSToken                  string                     `json:"bos_token"`
	BuildInfo                 string                     `json:"build_info"`
	ChatTemplate              string                     `json:"chat_template"`
	DefaultGenerationSettings DefaultGenerationSettings  `json:"default_generation_settings"`
	EndpointMetrics           bool                       `json:"endpoint_metrics"`
	EndpointProps             bool                       `json:"endpoint_props"`
	EndpointSlots             bool                       `json:"endpoint_slots"`
	EOSToken                  string                     `json:"eos_token"`
	IsSleeping                bool                       `json:"is_sleeping"`
	Modalities                Modalities                 `json:"modalities"`
	ModelAlias                string                     `json:"model_alias"`
	ModelPath                 string                     `json:"model_path"`
	TotalSlots                int                        `json:"total_slots"`
	WebUI                     bool                       `json:"webui"`
	WebUISettings             map[string]json.RawMessage `json:"webui_settings"`
}

// Modalities describes model capability flags.
type Modalities struct {
	Audio  bool `json:"audio"`
	Vision bool `json:"vision"`
}

// DefaultGenerationSettings mirrors default runtime generation settings.
type DefaultGenerationSettings struct {
	NCtx   int                        `json:"n_ctx"`
	Params map[string]json.RawMessage `json:"params"`
}

// CombinedMetrics ties the three data sources together for report generation.
type CombinedMetrics struct {
	Models []ModelMetricsBundle `json:"models"`
}

// ModelMetricsBundle groups accuracy, benchmark, and metadata inputs for a model+GPU.
type ModelMetricsBundle struct {
	GPU          string                 `json:"gpu"`
	Model        string                 `json:"model"`
	Accuracy     []AccuracyRecord       `json:"accuracy,omitempty"`
	Benchmarks   []BenchmarkRun         `json:"benchmarks,omitempty"`
	Metadata     *ModelMetadataDocument `json:"metadata,omitempty"`
	ModelMetrics *ModelMetrics          `json:"-"`
	Aggregates   DerivedAggregates      `json:"aggregates"`
}

// DerivedAggregates summarizes accuracy, latency, throughput, and comparison insights.
type DerivedAggregates struct {
	Accuracy      AccuracyAggregate     `json:"accuracy"`
	Latency       LatencyAggregate      `json:"latency"`
	Throughput    ThroughputAggregate   `json:"throughput"`
	TokenUsage    TokenUsageAggregate   `json:"token_usage"`
	Reliability   ReliabilityAggregate  `json:"reliability"`
	Distributions DistributionAggregate `json:"distributions"`
	Stability     StabilityAggregate    `json:"stability"`
	Efficiency    EfficiencyAggregate   `json:"efficiency"`
	Correlations  CorrelationAggregate  `json:"correlations"`
	Benchmark     BenchmarkAggregate    `json:"benchmark"`
	Metadata      MetadataSummary       `json:"metadata"`
	Comparisons   ComparisonAggregate   `json:"comparisons"`
}

// AccuracyAggregate captures correctness-focused rollups.
type AccuracyAggregate struct {
	Total                int                       `json:"total"`
	Correct              int                       `json:"correct"`
	Accuracy             float64                   `json:"accuracy"`
	ErrorRate            float64                   `json:"error_rate"`
	AvgDifficulty        float64                   `json:"avg_difficulty"`
	AvgMarginOfError     float64                   `json:"avg_margin_of_error"`
	MedianMarginOfError  float64                   `json:"median_margin_of_error"`
	MarginP90            float64                   `json:"margin_p90"`
	MarginP95            float64                   `json:"margin_p95"`
	MarginP99            float64                   `json:"margin_p99"`
	ByDifficulty         map[int]AccuracyBucket    `json:"by_difficulty,omitempty"`
	ByMarginBucket       map[string]AccuracyBucket `json:"by_margin_bucket,omitempty"`
	ByPromptLengthBucket map[string]AccuracyBucket `json:"by_prompt_length_bucket,omitempty"`
}

// AccuracyBucket stores aggregated accuracy for a bucket.
type AccuracyBucket struct {
	Total    int     `json:"total"`
	Correct  int     `json:"correct"`
	Accuracy float64 `json:"accuracy"`
}

// LatencyAggregate captures latency rollups based on time to first token and total duration.
type LatencyAggregate struct {
	AvgTTFTMs     float64 `json:"avg_ttft_ms"`
	MinTTFTMs     float64 `json:"min_ttft_ms"`
	MaxTTFTMs     float64 `json:"max_ttft_ms"`
	MedianTTFTMs  float64 `json:"median_ttft_ms"`
	P90TTFTMs     float64 `json:"p90_ttft_ms"`
	P95TTFTMs     float64 `json:"p95_ttft_ms"`
	P99TTFTMs     float64 `json:"p99_ttft_ms"`
	AvgTotalMs    float64 `json:"avg_total_ms"`
	MedianTotalMs float64 `json:"median_total_ms"`
	P90TotalMs    float64 `json:"p90_total_ms"`
	P95TotalMs    float64 `json:"p95_total_ms"`
	P99TotalMs    float64 `json:"p99_total_ms"`
	SampleCount   int     `json:"sample_count"`
}

// ThroughputAggregate captures throughput rollups based on tokens per second.
type ThroughputAggregate struct {
	AvgTokensPerSecond    float64 `json:"avg_tokens_per_second"`
	MinTokensPerSecond    float64 `json:"min_tokens_per_second"`
	MaxTokensPerSecond    float64 `json:"max_tokens_per_second"`
	MedianTokensPerSecond float64 `json:"median_tokens_per_second"`
	P90TokensPerSecond    float64 `json:"p90_tokens_per_second"`
	P95TokensPerSecond    float64 `json:"p95_tokens_per_second"`
	P99TokensPerSecond    float64 `json:"p99_tokens_per_second"`
	SampleCount           int     `json:"sample_count"`
	AvgLogProb            float64 `json:"avg_logprob"`
	AvgEffectiveTPS       float64 `json:"avg_effective_tps"`
	MedianEffectiveTPS    float64 `json:"median_effective_tps"`
	P90EffectiveTPS       float64 `json:"p90_effective_tps"`
	P95EffectiveTPS       float64 `json:"p95_effective_tps"`
	P99EffectiveTPS       float64 `json:"p99_effective_tps"`
	EffectiveSampleCount  int     `json:"effective_sample_count"`
}

// TokenUsageAggregate captures prompt/response length statistics.
type TokenUsageAggregate struct {
	AvgInputTokens           float64 `json:"avg_input_tokens"`
	AvgOutputTokens          float64 `json:"avg_output_tokens"`
	MedianInputTokens        float64 `json:"median_input_tokens"`
	MedianOutputTokens       float64 `json:"median_output_tokens"`
	P90InputTokens           float64 `json:"p90_input_tokens"`
	P90OutputTokens          float64 `json:"p90_output_tokens"`
	InputTokenRatePerSecond  float64 `json:"input_token_rate_per_second"`
	OutputTokenRatePerSecond float64 `json:"output_token_rate_per_second"`
	SampleCount              int     `json:"sample_count"`
}

// ReliabilityAggregate captures deadline and timeout reliability signals.
type ReliabilityAggregate struct {
	DeadlineExceededCount int     `json:"deadline_exceeded_count"`
	DeadlineExceededRate  float64 `json:"deadline_exceeded_rate"`
	TimeoutCount          int     `json:"timeout_count"`
	TimeoutRate           float64 `json:"timeout_rate"`
}

// DistributionAggregate captures distributional details across metrics.
type DistributionAggregate struct {
	TTFTMs          DistributionStats `json:"ttft_ms"`
	TotalMs         DistributionStats `json:"total_ms"`
	TokensPerSecond DistributionStats `json:"tokens_per_second"`
	EffectiveTPS    DistributionStats `json:"effective_tps"`
	InputTokens     DistributionStats `json:"input_tokens"`
	OutputTokens    DistributionStats `json:"output_tokens"`
	MarginOfError   DistributionStats `json:"margin_of_error"`
	LogProb         DistributionStats `json:"logprob"`
}

// DistributionStats captures count, mean, stddev, and quantiles.
type DistributionStats struct {
	Count  int     `json:"count"`
	Mean   float64 `json:"mean"`
	StdDev float64 `json:"stddev"`
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
	P50    float64 `json:"p50"`
	P90    float64 `json:"p90"`
	P95    float64 `json:"p95"`
	P99    float64 `json:"p99"`
}

// StabilityAggregate captures run-to-run consistency.
type StabilityAggregate struct {
	TokensPerSecondStdDev float64 `json:"tokens_per_second_stddev"`
	TTFTStdDevMs          float64 `json:"ttft_stddev_ms"`
	TotalStdDevMs         float64 `json:"total_stddev_ms"`
	TokensPerSecondCV     float64 `json:"tokens_per_second_cv"`
	TTFTCV                float64 `json:"ttft_cv"`
	TotalCV               float64 `json:"total_cv"`
}

// EfficiencyAggregate captures normalized performance and accuracy tradeoffs.
type EfficiencyAggregate struct {
	AccuracyPerSecond       float64 `json:"accuracy_per_second"`
	AccuracyPerToken        float64 `json:"accuracy_per_token"`
	TokensPerSecondPerParam float64 `json:"tokens_per_second_per_param"`
	LatencyPerParam         float64 `json:"latency_per_param"`
}

// CorrelationAggregate captures relationships between metrics.
type CorrelationAggregate struct {
	AccuracyVsTTFT        float64 `json:"accuracy_vs_ttft"`
	AccuracyVsThroughput  float64 `json:"accuracy_vs_throughput"`
	AccuracyVsTotalMs     float64 `json:"accuracy_vs_total_ms"`
	MarginVsDifficulty    float64 `json:"margin_vs_difficulty"`
	TTFTVsInputTokens     float64 `json:"ttft_vs_input_tokens"`
	TotalMsVsOutputTokens float64 `json:"total_ms_vs_output_tokens"`
}

// BenchmarkAggregate captures benchmark-only rollups.
type BenchmarkAggregate struct {
	PromptTokensPerSecond     float64 `json:"prompt_tokens_per_second"`
	GenerationTokensPerSecond float64 `json:"generation_tokens_per_second"`
	AvgBenchmarkNs            float64 `json:"avg_benchmark_ns"`
	StdDevBenchmarkNs         float64 `json:"stddev_benchmark_ns"`
	RunCount                  int     `json:"run_count"`
}

// MetadataSummary provides summary fields for report filtering and display.
type MetadataSummary struct {
	ModelType      string `json:"model_type"`
	ModelAlias     string `json:"model_alias"`
	ModelPath      string `json:"model_path"`
	Backend        string `json:"backend"`
	ContextSize    int    `json:"context_size"`
	SupportsAudio  bool   `json:"supports_audio"`
	SupportsVision bool   `json:"supports_vision"`
}

// ComparisonAggregate captures relative rankings against peers.
type ComparisonAggregate struct {
	RelativeAccuracy   float64 `json:"relative_accuracy"`
	RelativeLatency    float64 `json:"relative_latency"`
	RelativeThroughput float64 `json:"relative_throughput"`
	RelativeEfficiency float64 `json:"relative_efficiency"`
	ParetoFront        bool    `json:"pareto_front"`
}

// LoadCombinedMetrics reads accuracy, benchmark, and metadata directories into a combined set.
func LoadCombinedMetrics(accuracyDir, benchmarksDir, metadataDir, modelMetricsPath string) (CombinedMetrics, error) {
	models := make(map[string]*ModelMetricsBundle)

	if accuracyDir != "" {
		if err := loadAccuracyDir(accuracyDir, models); err != nil {
			return CombinedMetrics{}, err
		}
	}
	if benchmarksDir != "" {
		if err := loadBenchmarksDir(benchmarksDir, models); err != nil {
			return CombinedMetrics{}, err
		}
	}
	if metadataDir != "" {
		if err := loadMetadataDir(metadataDir, models); err != nil {
			return CombinedMetrics{}, err
		}
	}

	bundles := make([]ModelMetricsBundle, 0, len(models))
	for _, bundle := range models {
		computeAggregates(bundle)
		bundles = append(bundles, *bundle)
	}

	sort.Slice(bundles, func(i, j int) bool {
		if bundles[i].GPU == bundles[j].GPU {
			return bundles[i].Model < bundles[j].Model
		}
		return bundles[i].GPU < bundles[j].GPU
	})

	combined := CombinedMetrics{Models: bundles}
	applyComparisons(combined.Models)
	return combined, nil
}

func applyModelMetricsFile(path string, models map[string]*ModelMetricsBundle) error {
	metrics, err := loadModelMetricsFile(path)
	if err != nil {
		return err
	}
	if len(metrics) == 0 {
		return nil
	}

	index := make(map[string]map[*ModelMetricsBundle]struct{})
	for _, bundle := range models {
		if bundle.Metadata == nil {
			continue
		}
		names := []string{bundle.Model, bundle.Metadata.Name, bundle.Metadata.Metadata.ModelAlias}
		for _, name := range names {
			key := normalizeModelName(name)
			if key == "" {
				continue
			}
			set := index[key]
			if set == nil {
				set = make(map[*ModelMetricsBundle]struct{})
				index[key] = set
			}
			set[bundle] = struct{}{}
		}
	}

	for i := range metrics {
		metric := &metrics[i]
		key := normalizeModelName(metric.ModelName)
		if key == "" {
			continue
		}
		set := index[key]
		if len(set) != 1 {
			continue
		}
		for bundle := range set {
			bundle.ModelMetrics = metric
		}
	}

	return nil
}

func loadModelMetricsFile(path string) ([]ModelMetrics, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return nil, nil
	}
	data, err := os.ReadFile(trimmed)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read model metrics file %s: %w", trimmed, err)
	}
	var metrics []ModelMetrics
	if err := json.Unmarshal(data, &metrics); err != nil {
		return nil, fmt.Errorf("parse model metrics file %s: %w", trimmed, err)
	}
	return metrics, nil
}

func loadAccuracyDir(dir string, models map[string]*ModelMetricsBundle) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read accuracy dir %s: %w", dir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if !strings.HasSuffix(strings.ToLower(entry.Name()), ".jsonl") {
			continue
		}
		gpu, model, ok := parseGPUModel(entry.Name())
		if !ok {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		records, err := readAccuracyFile(path)
		if err != nil {
			return err
		}
		bundle := getBundle(models, gpu, model)
		bundle.Accuracy = append(bundle.Accuracy, records...)
	}
	return nil
}

func loadBenchmarksDir(dir string, models map[string]*ModelMetricsBundle) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read benchmarks dir %s: %w", dir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			continue
		}
		gpu, model, ok := parseGPUModel(entry.Name())
		if !ok {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		runs, err := readBenchmarkFile(path)
		if err != nil {
			return err
		}
		bundle := getBundle(models, gpu, model)
		bundle.Benchmarks = append(bundle.Benchmarks, runs...)
	}
	return nil
}

func loadMetadataDir(dir string, models map[string]*ModelMetricsBundle) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read metadata dir %s: %w", dir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			continue
		}
		gpu, model, ok := parseGPUModel(entry.Name())
		if !ok {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		doc, err := readMetadataFile(path)
		if err != nil {
			return err
		}
		bundle := getBundle(models, gpu, model)
		bundle.Metadata = &doc
	}
	return nil
}

func getBundle(models map[string]*ModelMetricsBundle, gpu, model string) *ModelMetricsBundle {
	normalizedModel := normalizeModelName(model)
	key := gpu + "|" + normalizedModel
	bundle, ok := models[key]
	if ok {
		if bundle.Model == "" {
			bundle.Model = model
		}
		return bundle
	}
	bundle = &ModelMetricsBundle{
		GPU:   gpu,
		Model: model,
	}
	models[key] = bundle
	return bundle
}

func parseGPUModel(name string) (string, string, bool) {
	base := strings.TrimSuffix(name, filepath.Ext(name))
	parts := strings.SplitN(base, "_", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func normalizeModelName(name string) string {
	normalized := strings.ToLower(name)
	normalized = strings.TrimSuffix(normalized, ".gguf")
	normalized = strings.TrimSuffix(normalized, "-gguf")
	normalized = strings.ReplaceAll(normalized, "_", "-")
	normalized = strings.ReplaceAll(normalized, ".", "-")
	normalized = strings.ReplaceAll(normalized, " ", "-")
	builder := strings.Builder{}
	builder.Grow(len(normalized))
	lastDash := false
	for _, r := range normalized {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
			lastDash = false
			continue
		}
		if r == '-' {
			if !lastDash {
				builder.WriteRune('-')
				lastDash = true
			}
			continue
		}
	}
	normalized = builder.String()
	normalized = strings.Trim(normalized, "-")
	return normalized
}

func readAccuracyFile(path string) ([]AccuracyRecord, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read accuracy file %s: %w", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024*1024), 50*1024*1024)
	var records []AccuracyRecord
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record AccuracyRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return nil, fmt.Errorf("parse accuracy record %s: %w", path, err)
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan accuracy file %s: %w", path, err)
	}
	return records, nil
}

func readBenchmarkFile(path string) ([]BenchmarkRun, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read benchmark file %s: %w", path, err)
	}
	var runs []BenchmarkRun
	if err := json.Unmarshal(data, &runs); err != nil {
		return nil, fmt.Errorf("parse benchmark file %s: %w", path, err)
	}
	return runs, nil
}

func readMetadataFile(path string) (ModelMetadataDocument, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ModelMetadataDocument{}, fmt.Errorf("read metadata file %s: %w", path, err)
	}
	var doc ModelMetadataDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return ModelMetadataDocument{}, fmt.Errorf("parse metadata file %s: %w", path, err)
	}
	return doc, nil
}

func computeAggregates(bundle *ModelMetricsBundle) {
	computeAccuracyAggregates(bundle)
	computeLatencyAggregates(bundle)
	computeThroughputAggregates(bundle)
	computeTokenUsageAggregates(bundle)
	computeReliabilityAggregates(bundle)
	computeDistributionAggregates(bundle)
	computeStabilityAggregates(bundle)
	computeEfficiencyAggregates(bundle)
	computeCorrelationAggregates(bundle)
	computeBenchmarkAggregates(bundle)
	computeMetadataSummary(bundle)
}

func applyModelMetricsFallbacks(bundle *ModelMetricsBundle) {
	if bundle == nil || bundle.ModelMetrics == nil {
		return
	}
	if len(bundle.Accuracy) == 0 {
		return
	}
	stats := bundle.ModelMetrics.OverallStats
	for i := range bundle.Accuracy {
		record := &bundle.Accuracy[i]
		if record.TokensPerSecond == 0 && stats.TokensPerSecond.Mean > 0 {
			record.TokensPerSecond = stats.TokensPerSecond.Mean
		}
		if record.TimeToFirstToken == 0 && stats.TTFTMillis.Mean > 0 {
			record.TimeToFirstToken = int64(math.Round(stats.TTFTMillis.Mean))
		}
		if record.TotalDurationMs == 0 && stats.TotalDurationMillis.Mean > 0 {
			record.TotalDurationMs = int64(math.Round(stats.TotalDurationMillis.Mean))
		}
		if record.InputTokens == 0 && stats.InputTokens.Mean > 0 {
			record.InputTokens = int(math.Round(stats.InputTokens.Mean))
		}
		if record.OutputTokens == 0 && stats.OutputTokens.Mean > 0 {
			record.OutputTokens = int(math.Round(stats.OutputTokens.Mean))
		}
	}
}

func computeAccuracyAggregates(bundle *ModelMetricsBundle) {
	records := bundle.Accuracy
	if len(records) == 0 {
		return
	}
	var correct int
	var difficultySum float64
	var marginSum float64
	marginValues := make([]float64, 0, len(records))
	byDifficulty := make(map[int]AccuracyBucket)
	byMargin := make(map[string]AccuracyBucket)
	byPrompt := make(map[string]AccuracyBucket)

	for _, record := range records {
		if record.Correct {
			correct++
		}
		difficultySum += float64(record.Difficulty)
		marginSum += record.MarginOfError
		marginValues = append(marginValues, record.MarginOfError)

		diffBucket := byDifficulty[record.Difficulty]
		diffBucket.Total++
		if record.Correct {
			diffBucket.Correct++
		}
		byDifficulty[record.Difficulty] = diffBucket

		marginBucket := bucketMargin(record.MarginOfError)
		marginEntry := byMargin[marginBucket]
		marginEntry.Total++
		if record.Correct {
			marginEntry.Correct++
		}
		byMargin[marginBucket] = marginEntry

		promptBucket := getTokenBucket(record.InputTokens)
		promptEntry := byPrompt[promptBucket]
		promptEntry.Total++
		if record.Correct {
			promptEntry.Correct++
		}
		byPrompt[promptBucket] = promptEntry
	}

	total := len(records)
	accuracy := ratio(float64(correct), float64(total))
	for key, bucket := range byDifficulty {
		bucket.Accuracy = ratio(float64(bucket.Correct), float64(bucket.Total))
		byDifficulty[key] = bucket
	}
	for key, bucket := range byMargin {
		bucket.Accuracy = ratio(float64(bucket.Correct), float64(bucket.Total))
		byMargin[key] = bucket
	}
	for key, bucket := range byPrompt {
		bucket.Accuracy = ratio(float64(bucket.Correct), float64(bucket.Total))
		byPrompt[key] = bucket
	}

	median, p90, p95, p99 := quantiles(marginValues)
	bundle.Aggregates.Accuracy = AccuracyAggregate{
		Total:                total,
		Correct:              correct,
		Accuracy:             accuracy,
		ErrorRate:            1 - accuracy,
		AvgDifficulty:        difficultySum / float64(total),
		AvgMarginOfError:     marginSum / float64(total),
		MedianMarginOfError:  median,
		MarginP90:            p90,
		MarginP95:            p95,
		MarginP99:            p99,
		ByDifficulty:         byDifficulty,
		ByMarginBucket:       byMargin,
		ByPromptLengthBucket: byPrompt,
	}
}

func computeLatencyAggregates(bundle *ModelMetricsBundle) {
	records := bundle.Accuracy
	if len(records) == 0 {
		return
	}
	ttft := make([]float64, 0, len(records))
	totalMs := make([]float64, 0, len(records))
	for _, record := range records {
		ttft = append(ttft, float64(record.TimeToFirstToken))
		totalMs = append(totalMs, float64(record.TotalDurationMs))
	}

	bundle.Aggregates.Latency = LatencyAggregate{
		AvgTTFTMs:     mean(ttft),
		MinTTFTMs:     minValue(ttft),
		MaxTTFTMs:     maxValue(ttft),
		MedianTTFTMs:  percentile(ttft, 50),
		P90TTFTMs:     percentile(ttft, 90),
		P95TTFTMs:     percentile(ttft, 95),
		P99TTFTMs:     percentile(ttft, 99),
		AvgTotalMs:    mean(totalMs),
		MedianTotalMs: percentile(totalMs, 50),
		P90TotalMs:    percentile(totalMs, 90),
		P95TotalMs:    percentile(totalMs, 95),
		P99TotalMs:    percentile(totalMs, 99),
		SampleCount:   len(records),
	}
}

func computeThroughputAggregates(bundle *ModelMetricsBundle) {
	records := bundle.Accuracy
	if len(records) == 0 {
		return
	}
	values := make([]float64, 0, len(records))
	effective := make([]float64, 0, len(records))
	logprobs := make([]float64, 0, len(records))
	for _, record := range records {
		values = append(values, record.TokensPerSecond)
		if avgLogProb, ok := averageLogProb(record); ok {
			logprobs = append(logprobs, avgLogProb)
			effective = append(effective, record.TokensPerSecond*math.Exp(avgLogProb))
		}
	}
	avgLogProb := mean(logprobs)
	medianEffective := percentile(effective, 50)
	p90Effective := percentile(effective, 90)
	p95Effective := percentile(effective, 95)
	p99Effective := percentile(effective, 99)
	bundle.Aggregates.Throughput = ThroughputAggregate{
		AvgTokensPerSecond:    mean(values),
		MinTokensPerSecond:    minValue(values),
		MaxTokensPerSecond:    maxValue(values),
		MedianTokensPerSecond: percentile(values, 50),
		P90TokensPerSecond:    percentile(values, 90),
		P95TokensPerSecond:    percentile(values, 95),
		P99TokensPerSecond:    percentile(values, 99),
		SampleCount:           len(values),
		AvgLogProb:            avgLogProb,
		AvgEffectiveTPS:       mean(effective),
		MedianEffectiveTPS:    medianEffective,
		P90EffectiveTPS:       p90Effective,
		P95EffectiveTPS:       p95Effective,
		P99EffectiveTPS:       p99Effective,
		EffectiveSampleCount:  len(effective),
	}
}

func computeTokenUsageAggregates(bundle *ModelMetricsBundle) {
	records := bundle.Accuracy
	if len(records) == 0 {
		return
	}
	input := make([]float64, 0, len(records))
	output := make([]float64, 0, len(records))
	var totalDurationSeconds float64
	for _, record := range records {
		input = append(input, float64(record.InputTokens))
		output = append(output, float64(record.OutputTokens))
		if record.TotalDurationMs > 0 {
			totalDurationSeconds += float64(record.TotalDurationMs) / 1000
		}
	}
	bundle.Aggregates.TokenUsage = TokenUsageAggregate{
		AvgInputTokens:           mean(input),
		AvgOutputTokens:          mean(output),
		MedianInputTokens:        percentile(input, 50),
		MedianOutputTokens:       percentile(output, 50),
		P90InputTokens:           percentile(input, 90),
		P90OutputTokens:          percentile(output, 90),
		InputTokenRatePerSecond:  rate(mean(input), totalDurationSeconds, len(records)),
		OutputTokenRatePerSecond: rate(mean(output), totalDurationSeconds, len(records)),
		SampleCount:              len(records),
	}
}

func computeReliabilityAggregates(bundle *ModelMetricsBundle) {
	records := bundle.Accuracy
	if len(records) == 0 {
		return
	}
	var deadlineExceeded int
	var timeoutCount int
	for _, record := range records {
		if record.DeadlineExceeded {
			deadlineExceeded++
		}
		if record.DeadlineExceeded || record.DeadlineTimeout > 0 {
			timeoutCount++
		}
	}
	total := float64(len(records))
	bundle.Aggregates.Reliability = ReliabilityAggregate{
		DeadlineExceededCount: deadlineExceeded,
		DeadlineExceededRate:  ratio(float64(deadlineExceeded), total),
		TimeoutCount:          timeoutCount,
		TimeoutRate:           ratio(float64(timeoutCount), total),
	}
}

func computeDistributionAggregates(bundle *ModelMetricsBundle) {
	records := bundle.Accuracy
	if len(records) == 0 {
		return
	}
	ttft := make([]float64, 0, len(records))
	totalMs := make([]float64, 0, len(records))
	tps := make([]float64, 0, len(records))
	etps := make([]float64, 0, len(records))
	input := make([]float64, 0, len(records))
	output := make([]float64, 0, len(records))
	margin := make([]float64, 0, len(records))
	logprobs := make([]float64, 0, len(records))
	for _, record := range records {
		ttft = append(ttft, float64(record.TimeToFirstToken))
		totalMs = append(totalMs, float64(record.TotalDurationMs))
		tps = append(tps, record.TokensPerSecond)
		input = append(input, float64(record.InputTokens))
		output = append(output, float64(record.OutputTokens))
		margin = append(margin, record.MarginOfError)
		if avgLogProb, ok := averageLogProb(record); ok {
			logprobs = append(logprobs, avgLogProb)
			etps = append(etps, record.TokensPerSecond*math.Exp(avgLogProb))
		}
	}
	bundle.Aggregates.Distributions = DistributionAggregate{
		TTFTMs:          distributionStats(ttft),
		TotalMs:         distributionStats(totalMs),
		TokensPerSecond: distributionStats(tps),
		EffectiveTPS:    distributionStats(etps),
		InputTokens:     distributionStats(input),
		OutputTokens:    distributionStats(output),
		MarginOfError:   distributionStats(margin),
		LogProb:         distributionStats(logprobs),
	}
}

func computeStabilityAggregates(bundle *ModelMetricsBundle) {
	dist := bundle.Aggregates.Distributions
	bundle.Aggregates.Stability = StabilityAggregate{
		TokensPerSecondStdDev: dist.TokensPerSecond.StdDev,
		TTFTStdDevMs:          dist.TTFTMs.StdDev,
		TotalStdDevMs:         dist.TotalMs.StdDev,
		TokensPerSecondCV:     ratio(dist.TokensPerSecond.StdDev, dist.TokensPerSecond.Mean),
		TTFTCV:                ratio(dist.TTFTMs.StdDev, dist.TTFTMs.Mean),
		TotalCV:               ratio(dist.TotalMs.StdDev, dist.TotalMs.Mean),
	}
}

func computeEfficiencyAggregates(bundle *ModelMetricsBundle) {
	accuracy := bundle.Aggregates.Accuracy.Accuracy
	avgTotalMs := bundle.Aggregates.Latency.AvgTotalMs
	avgOutputTokens := bundle.Aggregates.TokenUsage.AvgOutputTokens
	paramCount := benchmarkParamCount(bundle.Benchmarks)
	efficiency := EfficiencyAggregate{
		AccuracyPerSecond:       ratio(accuracy, avgTotalMs/1000),
		AccuracyPerToken:        ratio(accuracy, avgOutputTokens),
		TokensPerSecondPerParam: ratio(bundle.Aggregates.Throughput.AvgTokensPerSecond, float64(paramCount)),
		LatencyPerParam:         ratio(avgTotalMs, float64(paramCount)),
	}
	bundle.Aggregates.Efficiency = efficiency
}

func computeCorrelationAggregates(bundle *ModelMetricsBundle) {
	records := bundle.Accuracy
	if len(records) < 2 {
		return
	}
	accuracyValues := make([]float64, 0, len(records))
	ttftValues := make([]float64, 0, len(records))
	throughputValues := make([]float64, 0, len(records))
	totalMsValues := make([]float64, 0, len(records))
	marginValues := make([]float64, 0, len(records))
	difficultyValues := make([]float64, 0, len(records))
	inputTokens := make([]float64, 0, len(records))
	outputTokens := make([]float64, 0, len(records))

	for _, record := range records {
		if record.Correct {
			accuracyValues = append(accuracyValues, 1)
		} else {
			accuracyValues = append(accuracyValues, 0)
		}
		ttftValues = append(ttftValues, float64(record.TimeToFirstToken))
		throughputValues = append(throughputValues, record.TokensPerSecond)
		totalMsValues = append(totalMsValues, float64(record.TotalDurationMs))
		marginValues = append(marginValues, record.MarginOfError)
		difficultyValues = append(difficultyValues, float64(record.Difficulty))
		inputTokens = append(inputTokens, float64(record.InputTokens))
		outputTokens = append(outputTokens, float64(record.OutputTokens))
	}

	bundle.Aggregates.Correlations = CorrelationAggregate{
		AccuracyVsTTFT:        correlation(accuracyValues, ttftValues),
		AccuracyVsThroughput:  correlation(accuracyValues, throughputValues),
		AccuracyVsTotalMs:     correlation(accuracyValues, totalMsValues),
		MarginVsDifficulty:    correlation(marginValues, difficultyValues),
		TTFTVsInputTokens:     correlation(ttftValues, inputTokens),
		TotalMsVsOutputTokens: correlation(totalMsValues, outputTokens),
	}
}

func computeBenchmarkAggregates(bundle *ModelMetricsBundle) {
	if len(bundle.Benchmarks) == 0 {
		return
	}
	var promptSum float64
	var promptCount int
	var genSum float64
	var genCount int
	var avgNsSum float64
	var stdNsSum float64
	for _, run := range bundle.Benchmarks {
		avgNsSum += float64(run.AvgNs)
		stdNsSum += float64(run.StddevNs)
		if run.NPrompt > 0 && run.NGen == 0 {
			promptSum += run.AvgTs
			promptCount++
		}
		if run.NGen > 0 {
			genSum += run.AvgTs
			genCount++
		}
	}
	runCount := len(bundle.Benchmarks)
	bundle.Aggregates.Benchmark = BenchmarkAggregate{
		PromptTokensPerSecond:     ratio(promptSum, float64(promptCount)),
		GenerationTokensPerSecond: ratio(genSum, float64(genCount)),
		AvgBenchmarkNs:            ratio(avgNsSum, float64(runCount)),
		StdDevBenchmarkNs:         ratio(stdNsSum, float64(runCount)),
		RunCount:                  runCount,
	}
}

func computeMetadataSummary(bundle *ModelMetricsBundle) {
	var modelType string
	var backend string
	for _, run := range bundle.Benchmarks {
		if modelType == "" && run.ModelType != "" {
			modelType = run.ModelType
		}
		if backend == "" && run.Backends != "" {
			backend = run.Backends
		}
	}

	metadata := MetadataSummary{
		ModelType: modelType,
		Backend:   backend,
	}

	if bundle.Metadata != nil {
		metadata.ModelAlias = bundle.Metadata.Metadata.ModelAlias
		metadata.ModelPath = bundle.Metadata.Metadata.ModelPath
		metadata.ContextSize = bundle.Metadata.Metadata.DefaultGenerationSettings.NCtx
		metadata.SupportsAudio = bundle.Metadata.Metadata.Modalities.Audio
		metadata.SupportsVision = bundle.Metadata.Metadata.Modalities.Vision
		if metadata.Backend == "" {
			metadata.Backend = bundle.Metadata.Type
		}
	}

	bundle.Aggregates.Metadata = metadata
}

func applyComparisons(models []ModelMetricsBundle) {
	var bestAccuracy float64
	var bestThroughput float64
	bestLatency := math.MaxFloat64
	var bestEfficiency float64
	for _, bundle := range models {
		if bundle.Aggregates.Accuracy.Accuracy > bestAccuracy {
			bestAccuracy = bundle.Aggregates.Accuracy.Accuracy
		}
		if bundle.Aggregates.Throughput.AvgTokensPerSecond > bestThroughput {
			bestThroughput = bundle.Aggregates.Throughput.AvgTokensPerSecond
		}
		latency := bundle.Aggregates.Latency.AvgTotalMs
		if latency > 0 && latency < bestLatency {
			bestLatency = latency
		}
		if bundle.Aggregates.Efficiency.AccuracyPerSecond > bestEfficiency {
			bestEfficiency = bundle.Aggregates.Efficiency.AccuracyPerSecond
		}
	}
	if bestLatency == math.MaxFloat64 {
		bestLatency = 0
	}

	for i := range models {
		bundle := &models[i]
		bundle.Aggregates.Comparisons = ComparisonAggregate{
			RelativeAccuracy:   ratio(bundle.Aggregates.Accuracy.Accuracy, bestAccuracy),
			RelativeLatency:    ratio(bestLatency, bundle.Aggregates.Latency.AvgTotalMs),
			RelativeThroughput: ratio(bundle.Aggregates.Throughput.AvgTokensPerSecond, bestThroughput),
			RelativeEfficiency: ratio(bundle.Aggregates.Efficiency.AccuracyPerSecond, bestEfficiency),
			ParetoFront:        isParetoFront(models, i),
		}
	}
}

func isParetoFront(models []ModelMetricsBundle, idx int) bool {
	target := models[idx]
	for i := range models {
		if i == idx {
			continue
		}
		peer := models[i]
		if dominates(peer, target) {
			return false
		}
	}
	return true
}

func dominates(a, b ModelMetricsBundle) bool {
	aAcc := a.Aggregates.Accuracy.Accuracy
	bAcc := b.Aggregates.Accuracy.Accuracy
	aThroughput := a.Aggregates.Throughput.AvgTokensPerSecond
	bThroughput := b.Aggregates.Throughput.AvgTokensPerSecond
	aLatency := a.Aggregates.Latency.AvgTotalMs
	bLatency := b.Aggregates.Latency.AvgTotalMs

	betterOrEqual := aAcc >= bAcc && aThroughput >= bThroughput
	latencyBetter := aLatency > 0 && bLatency > 0 && aLatency <= bLatency
	if !betterOrEqual || !latencyBetter {
		return false
	}
	strictBetter := aAcc > bAcc || aThroughput > bThroughput || aLatency < bLatency
	return strictBetter
}

func distributionStats(values []float64) DistributionStats {
	if len(values) == 0 {
		return DistributionStats{}
	}
	meanVal := mean(values)
	stddev := stddev(values, meanVal)
	return DistributionStats{
		Count:  len(values),
		Mean:   meanVal,
		StdDev: stddev,
		Min:    minValue(values),
		Max:    maxValue(values),
		P50:    percentile(values, 50),
		P90:    percentile(values, 90),
		P95:    percentile(values, 95),
		P99:    percentile(values, 99),
	}
}

func quantiles(values []float64) (float64, float64, float64, float64) {
	return percentile(values, 50), percentile(values, 90), percentile(values, 95), percentile(values, 99)
}

func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)
	if len(sorted) == 1 {
		return sorted[0]
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 100 {
		return sorted[len(sorted)-1]
	}
	pos := (p / 100) * float64(len(sorted)-1)
	lower := int(math.Floor(pos))
	upper := int(math.Ceil(pos))
	if lower == upper {
		return sorted[lower]
	}
	weight := pos - float64(lower)
	return sorted[lower] + weight*(sorted[upper]-sorted[lower])
}

func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	var sum float64
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

func stddev(values []float64, meanVal float64) float64 {
	if len(values) < 2 {
		return 0
	}
	var sum float64
	for _, v := range values {
		diff := v - meanVal
		sum += diff * diff
	}
	return math.Sqrt(sum / float64(len(values)-1))
}

func minValue(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	minVal := values[0]
	for _, v := range values[1:] {
		if v < minVal {
			minVal = v
		}
	}
	return minVal
}

func maxValue(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	maxVal := values[0]
	for _, v := range values[1:] {
		if v > maxVal {
			maxVal = v
		}
	}
	return maxVal
}

func ratio(numerator, denominator float64) float64 {
	if denominator == 0 {
		return 0
	}
	return numerator / denominator
}

func rate(avgTokens, totalSeconds float64, count int) float64 {
	if totalSeconds <= 0 || count == 0 {
		return 0
	}
	return avgTokens * float64(count) / totalSeconds
}

func correlation(x, y []float64) float64 {
	if len(x) != len(y) || len(x) < 2 {
		return 0
	}
	meanX := mean(x)
	meanY := mean(y)
	var num float64
	var sumX float64
	var sumY float64
	for i := range x {
		dx := x[i] - meanX
		dy := y[i] - meanY
		num += dx * dy
		sumX += dx * dx
		sumY += dy * dy
	}
	den := math.Sqrt(sumX * sumY)
	if den == 0 {
		return 0
	}
	return num / den
}

func bucketMargin(value float64) string {
	switch {
	case value <= 0:
		return "0"
	case value <= 0.5:
		return "0-0.5"
	case value <= 1:
		return "0.5-1"
	case value <= 2:
		return "1-2"
	case value <= 5:
		return "2-5"
	default:
		return "5+"
	}
}

func getTokenBucket(tokens int) string {
	switch {
	case tokens <= 256:
		return "0-256"
	case tokens <= 1024:
		return "257-1024"
	case tokens <= 4096:
		return "1025-4096"
	case tokens <= 8192:
		return "4097-8192"
	default:
		return "8192+"
	}
}

func benchmarkParamCount(runs []BenchmarkRun) int64 {
	for _, run := range runs {
		if run.ModelNParams > 0 {
			return run.ModelNParams
		}
	}
	return 0
}

func averageLogProb(record AccuracyRecord) (float64, bool) {
	if record.LogProbs == nil || len(record.LogProbs.Content) == 0 {
		return 0, false
	}
	var sum float64
	for _, token := range record.LogProbs.Content {
		sum += token.LogProb
	}
	return sum / float64(len(record.LogProbs.Content)), true
}
