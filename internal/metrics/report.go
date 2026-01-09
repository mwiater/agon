// internal/metrics/report.go
package metrics

import (
	"bytes"
	"encoding/json"
	"html/template"
)

type CombinedReportData struct {
	Title       string
	MetricsJSON template.JS
}

type ReportMetrics struct {
	Models []ReportModelMetricsBundle `json:"models"`
}

type ReportModelMetricsBundle struct {
	GPU        string                 `json:"gpu"`
	Model      string                 `json:"model"`
	Accuracy   []ReportAccuracyRecord `json:"accuracy,omitempty"`
	Aggregates DerivedAggregates      `json:"aggregates"`
}

type ReportAccuracyRecord struct {
	InputTokens      int     `json:"input_tokens"`
	TokensPerSecond  float64 `json:"tokens_per_second"`
	TimeToFirstToken int64   `json:"time_to_first_token"`
	TotalDurationMs  int64   `json:"total_duration_ms"`
}

// GenerateCombinedReport renders a standalone HTML dashboard powered by CombinedMetrics.
func GenerateCombinedReport(combined CombinedMetrics) (string, error) {
	condensed := condenseMetrics(combined)
	payload, err := json.Marshal(condensed)
	if err != nil {
		return "", err
	}

	viewModel := CombinedReportData{
		Title:       "agon: LLM Metrics Report",
		MetricsJSON: template.JS(payload),
	}

	var buf bytes.Buffer
	if err := combinedReportTemplate.Execute(&buf, viewModel); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func condenseMetrics(combined CombinedMetrics) ReportMetrics {
	models := make([]ReportModelMetricsBundle, 0, len(combined.Models))
	for _, bundle := range combined.Models {
		accuracy := make([]ReportAccuracyRecord, 0, len(bundle.Accuracy))
		for _, record := range bundle.Accuracy {
			accuracy = append(accuracy, ReportAccuracyRecord{
				InputTokens:      record.InputTokens,
				TokensPerSecond:  record.TokensPerSecond,
				TimeToFirstToken: record.TimeToFirstToken,
				TotalDurationMs:  record.TotalDurationMs,
			})
		}
		models = append(models, ReportModelMetricsBundle{
			GPU:        bundle.GPU,
			Model:      bundle.Model,
			Accuracy:   accuracy,
			Aggregates: bundle.Aggregates,
		})
	}
	return ReportMetrics{Models: models}
}

var combinedReportTemplate = template.Must(template.New("metrics-report").Parse(combinedReportTemplateHTML))

const combinedReportTemplateHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{ .Title }}</title>
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/bootstrap@5.3.3/dist/css/bootstrap.min.css">
  <script src="https://kit.fontawesome.com/517f4f7a2b.js" crossorigin="anonymous"></script>
  <link href="https://fonts.googleapis.com/icon?family=Material+Icons+Two+Tone" rel="stylesheet">
  <style>
    :root {
      --primary: #334155;
      --secondary: #64748B;
      --accent: #3B82F6;
      --light: #F1F5F9;
      --background: #FFFFFF;
      --text: #0F172A;
      --success: #10B981;
      --warning: #F59E0B;
      --border: #E2E8F0;
    }
    [data-theme="dark"] {
      --primary: #0F172A;
      --secondary: #94A3B8;
      --accent: #60A5FA;
      --light: #0B1220;
      --background: #0F172A;
      --text: #E2E8F0;
      --success: #34D399;
      --warning: #FBBF24;
      --border: rgba(148, 163, 184, 0.25);
    }
    body {
      background-color: var(--light);
      color: var(--text);
    }
    .navbar-dark {
      background-color: var(--primary) !important;
    }
    .bg-dark {
      background-color: var(--primary) !important;
    }
    .navbar-dark .navbar-brand,
    .navbar-dark .text-light {
      color: var(--light) !important;
    }
    .card {
      border: 1px solid var(--border);
      background-color: var(--background);
    }
    .table thead th { cursor: pointer; }
    .table thead th,
    .table thead td {
      background-color: var(--light);
      color: var(--text);
      border-color: var(--border);
    }
    .table-striped>tbody>tr:nth-of-type(odd)>* {
      --bs-table-accent-bg: var(--light);
    }
    .table-bordered>:not(caption)>* {
      border-color: var(--border);
    }
    .sort-icon { font-size: 0.8rem; margin-left: 0.25rem; }
    .accordion-button .badge { margin-left: 0.5rem; }
    .accordion-button {
      background-color: var(--light);
      color: var(--text);
    }
    .accordion-button:not(.collapsed) {
      background-color: var(--accent);
      color: var(--background);
    }
    .list-group-item {
      display: flex;
      align-items: center;
      justify-content: space-between;
      background-color: var(--background);
      border-color: var(--border);
      color: var(--text);
    }
    .notes-list li { margin-bottom: 0.25rem; }
    .table#modelsTable>tbody>tr>td.top-performer {
      background-color: #DBEAFE;
      font-weight: 600;
      color: var(--text);
    }
    .chart-card {
      background: var(--background);
      border-radius: 16px;
      padding: 1.5rem;
      box-shadow: 0 1px 3px rgba(15, 23, 42, 0.1);
      border: 1px solid var(--border);
    }
    .chart-title {
      font-size: 1.5rem;
      font-weight: 700;
      color: var(--text);
      margin-bottom: 0.25rem;
    }
    .chart-subtitle {
      color: var(--secondary);
      margin-bottom: 1.5rem;
    }
    .chart-canvas {
      position: relative;
      height: 420px;
    }
    .legend-container {
      display: flex;
      gap: 1.5rem;
      justify-content: center;
      flex-wrap: wrap;
      margin-top: 1.25rem;
      padding-top: 1.25rem;
      border-top: 2px solid var(--border);
    }
    .legend-item {
      display: flex;
      align-items: center;
      gap: 0.5rem;
    }
    .legend-color {
      width: 14px;
      height: 14px;
      border-radius: 50%;
    }
    .legend-text {
      font-size: 0.9rem;
      color: var(--secondary);
    }
    .filter-row {
      display: flex;
      align-items: center;
      gap: 0.75rem;
      flex-wrap: wrap;
      margin-bottom: 1rem;
    }
    .filter-label {
      font-weight: 600;
      color: var(--text);
    }
    .badge.bg-primary {
      background-color: var(--accent) !important;
    }
    .badge.bg-success {
      background-color: var(--success) !important;
    }
    .badge.bg-warning {
      background-color: var(--warning) !important;
      color: var(--background) !important;
    }
    .badge.bg-danger {
      background-color: #DC2626 !important;
    }
    .badge.bg-secondary {
      background-color: var(--secondary) !important;
    }
    .theme-toggle {
      border: 1px solid var(--border);
      color: var(--light);
    }
    [data-theme="dark"] .theme-toggle {
      color: var(--text);
      background-color: rgba(148, 163, 184, 0.15);
    }
    [data-theme="dark"] .table#modelsTable>tbody>tr>td.top-performer {
      background-color: rgba(96, 165, 250, 0.25);
    }
    [data-theme="dark"] .chart-card {
      box-shadow: 0 10px 28px rgba(2, 6, 23, 0.6);
    }
    [data-theme="dark"] .accordion-button:not(.collapsed) {
      background-color: var(--accent);
      color: #0B1220;
    }
    [data-theme="dark"] .badge.bg-warning {
      color: #0B1220 !important;
    }
  </style>
</head>
<body>
  <nav class="navbar navbar-dark bg-dark">
    <div class="container-fluid">
      <span class="navbar-brand mb-0 h1">{{ .Title }}</span>
      <div class="d-flex align-items-center gap-3">
        <button class="btn btn-sm theme-toggle" id="themeToggle" type="button" aria-label="Toggle dark mode">
          <i class="fa-duotone fa-regular fa-moon" aria-hidden="true"></i>
        </button>
        <span class="text-light">Generated: <span id="generatedAt">-</span></span>
      </div>
    </div>
  </nav>
  <main class="container-fluid my-4">
    <div id="summaryRows"></div>

    <section class="mt-4">
      <div class="card shadow-sm">
        <div class="card-body">
          <div class="filter-row mb-0">
            <span class="filter-label">GPU filter:</span>
            <select class="form-select form-select-sm w-auto" id="gpuFilter"></select>
          </div>
        </div>
      </div>
    </section>

    <section class="mt-4">
      <div class="card shadow-sm">
        <div class="card-header bg-white">
          <h5 class="mb-0">Model Comparison</h5>
        </div>
        <div class="card-body">
          <div class="table-responsive">
            <table class="table table-striped table-hover table-bordered table-sm" id="modelsTable">
              <thead class="table-light">
                <tr>
                  <th class="sortable" data-type="text">Model <i class="fa-duotone fa-regular fa-arrows-up-down sort-icon" aria-hidden="true"></i></th>
                  <th class="sortable" data-type="text">GPU <i class="fa-duotone fa-regular fa-arrows-up-down sort-icon" aria-hidden="true"></i></th>
                  <th class="sortable" data-type="number">Accuracy (%) <i class="fa-duotone fa-regular fa-arrows-up-down sort-icon" aria-hidden="true"></i></th>
                  <th class="sortable" data-type="number">Questions <i class="fa-duotone fa-regular fa-arrows-up-down sort-icon" aria-hidden="true"></i></th>
                  <th class="sortable" data-type="number">Deadline Rate (%) <i class="fa-duotone fa-regular fa-arrows-up-down sort-icon" aria-hidden="true"></i></th>
                  <th class="sortable" data-type="number">Avg TTFT (ms) <i class="fa-duotone fa-regular fa-arrows-up-down sort-icon" aria-hidden="true"></i></th>
                  <th class="sortable" data-type="number">Avg TPS <i class="fa-duotone fa-regular fa-arrows-up-down sort-icon" aria-hidden="true"></i></th>
                  <th class="sortable" data-type="number">Eff TPS <i class="fa-duotone fa-regular fa-arrows-up-down sort-icon" aria-hidden="true"></i></th>
                  <th class="sortable" data-type="number">Avg Total (ms) <i class="fa-duotone fa-regular fa-arrows-up-down sort-icon" aria-hidden="true"></i></th>
                  <th class="sortable" data-type="number">Avg Output Tokens <i class="fa-duotone fa-regular fa-arrows-up-down sort-icon" aria-hidden="true"></i></th>
                  <th class="sortable" data-type="number">Efficiency (acc/sec) <i class="fa-duotone fa-regular fa-arrows-up-down sort-icon" aria-hidden="true"></i></th>
                  <th class="sortable" data-type="number">Stability (CV) <i class="fa-duotone fa-regular fa-arrows-up-down sort-icon" aria-hidden="true"></i></th>
                  <th class="sortable" data-type="text">Pareto <i class="fa-duotone fa-regular fa-arrows-up-down sort-icon" aria-hidden="true"></i></th>
                </tr>
              </thead>
              <tbody></tbody>
            </table>
          </div>
        </div>
      </div>
    </section>

    <section class="mt-4">
      <div class="card shadow-sm chart-card">
        <div class="card-body">
          <div class="chart-title">Accuracy vs Throughput</div>
          <div class="chart-subtitle">Higher accuracy and higher tokens/sec is better.</div>
          <div class="chart-canvas">
            <canvas id="accuracyThroughputChart" aria-label="Accuracy vs throughput chart" role="img"></canvas>
          </div>
          <div id="accuracyThroughputEmpty" class="text-muted small mt-2"></div>
          <div class="legend-container">
            <div class="legend-item">
              <div class="legend-color" style="background: #334155;"></div>
              <span class="legend-text"><strong>Excellent</strong> (70%+ accuracy)</span>
            </div>
            <div class="legend-item">
              <div class="legend-color" style="background: #64748B;"></div>
              <span class="legend-text"><strong>Good</strong> (50-70% accuracy)</span>
            </div>
            <div class="legend-item">
              <div class="legend-color" style="background: #94A3B8;"></div>
              <span class="legend-text"><strong>Fair</strong> (35-50% accuracy)</span>
            </div>
            <div class="legend-item">
              <div class="legend-color" style="background: #CBD5E1;"></div>
              <span class="legend-text"><strong>Poor</strong> (&lt;35% accuracy)</span>
            </div>
          </div>
        </div>
      </div>
    </section>
    <section class="mt-4">
      <div class="row g-3">
        <div class="col-xl-6">
          <div class="card shadow-sm chart-card">
            <div class="card-body">
              <div class="chart-title">Efficiency Leaderboard</div>
              <div class="chart-subtitle">Accuracy per second (higher is better).</div>
              <div class="chart-canvas">
                <canvas id="efficiencyChart" aria-label="Efficiency chart" role="img"></canvas>
              </div>
              <div id="efficiencyEmpty" class="text-muted small mt-2"></div>
            </div>
          </div>
        </div>
        <div class="col-xl-6">
          <div class="card shadow-sm chart-card">
            <div class="card-body">
              <div class="chart-title">Latency Percentiles</div>
              <div class="chart-subtitle">TTFT percentiles by model and GPU.</div>
              <div class="chart-canvas">
                <canvas id="latencyPercentileChart" aria-label="Latency percentile chart" role="img"></canvas>
              </div>
              <div id="latencyPercentileEmpty" class="text-muted small mt-2"></div>
            </div>
          </div>
        </div>
      </div>
    </section>

    <section class="mt-4">
      <div class="row g-3">
        <div class="col-xl-6">
          <div class="card shadow-sm chart-card">
            <div class="card-body">
              <div class="chart-title">Confidence-Adjusted Throughput</div>
              <div class="chart-subtitle">Effective tokens/sec weighted by average logprob.</div>
              <div class="chart-canvas">
                <canvas id="effectiveTpsChart" aria-label="Effective tokens per second chart" role="img"></canvas>
              </div>
              <div id="effectiveTpsEmpty" class="text-muted small mt-2"></div>
            </div>
          </div>
        </div>
        <div class="col-xl-6">
          <div class="card shadow-sm chart-card">
            <div class="card-body">
              <div class="chart-title">Average Logprob</div>
              <div class="chart-subtitle">Higher is better (less uncertainty).</div>
              <div class="chart-canvas">
                <canvas id="avgLogprobChart" aria-label="Average logprob chart" role="img"></canvas>
              </div>
              <div id="avgLogprobEmpty" class="text-muted small mt-2"></div>
            </div>
          </div>
        </div>
      </div>
    </section>

    <section class="mt-4">
      <div class="card shadow-sm chart-card">
        <div class="card-body">
          <div class="chart-title">Correlation Snapshot</div>
          <div class="chart-subtitle">How accuracy moves with throughput and latency.</div>
          <div class="chart-canvas">
            <canvas id="correlationChart" aria-label="Correlation chart" role="img"></canvas>
          </div>
          <div id="correlationEmpty" class="text-muted small mt-2"></div>
        </div>
      </div>
    </section>

    <section class="mt-4">
      <div class="card shadow-sm chart-card">
        <div class="card-body">
          <div class="chart-title">Input Tokens vs Processing</div>
          <div class="chart-subtitle">Accuracy records reveal how input length affects TTFT and throughput.</div>
          <div class="filter-row">
            <span class="filter-label">Model filter:</span>
            <select class="form-select form-select-sm w-auto" id="inputTokenModelFilter"></select>
          </div>
          <div class="row g-3">
            <div class="col-lg-6">
              <div class="chart-canvas">
                <canvas id="inputTokensTtftChart" aria-label="Input tokens vs time to first token" role="img"></canvas>
              </div>
              <div id="inputTokensTtftEmpty" class="text-muted small mt-2"></div>
            </div>
            <div class="col-lg-6">
              <div class="chart-canvas">
                <canvas id="inputTokensTpsChart" aria-label="Input tokens vs tokens per second" role="img"></canvas>
              </div>
              <div id="inputTokensTpsEmpty" class="text-muted small mt-2"></div>
            </div>
          </div>
        </div>
      </div>
    </section>

    <section class="mt-4">
      <div class="card shadow-sm">
        <div class="card-header bg-white">
          <h5 class="mb-0">Per-Model Details</h5>
        </div>
        <div class="card-body">
          <div class="accordion" id="modelAccordion"></div>
        </div>
      </div>
    </section>
  </main>

  <script src="https://code.jquery.com/jquery-3.7.1.min.js"></script>
  <script src="https://cdn.jsdelivr.net/npm/bootstrap@5.3.3/dist/js/bootstrap.bundle.min.js"></script>
  <script src="https://cdn.jsdelivr.net/npm/chart.js@4.4.2/dist/chart.umd.min.js"></script>
  <script>
    var metrics = {{ .MetricsJSON }};
  </script>
  <script>
    (function($) {
      function formatNumber(value, decimals) {
        if (value === null || value === undefined || isNaN(value)) {
          return '-';
        }
        return Number(value).toFixed(decimals);
      }

      function formatPercent(value, decimals) {
        if (value === null || value === undefined || isNaN(value)) {
          return '-';
        }
        return (Number(value) * 100).toFixed(decimals) + '%';
      }

      function modelLabel(bundle) {
        var name = bundle.model || '-';
        var gpu = bundle.gpu || '-';
        return name + ' @ ' + gpu;
      }

      function getBenchmark(model) {
        return model.aggregates && model.aggregates.benchmark ? model.aggregates.benchmark : null;
      }

      function getBenchmarkPromptTPS(model) {
        var bench = getBenchmark(model);
        if (!bench || typeof bench.prompt_tokens_per_second !== 'number' || isNaN(bench.prompt_tokens_per_second)) {
          return null;
        }
        return bench.prompt_tokens_per_second;
      }

      function getBenchmarkGenTPS(model) {
        var bench = getBenchmark(model);
        if (!bench || typeof bench.generation_tokens_per_second !== 'number' || isNaN(bench.generation_tokens_per_second)) {
          return null;
        }
        return bench.generation_tokens_per_second;
      }

      function getBenchmarkThroughput(model) {
        var gen = getBenchmarkGenTPS(model);
        if (gen && gen > 0) {
          return gen;
        }
        var prompt = getBenchmarkPromptTPS(model);
        if (prompt && prompt > 0) {
          return prompt;
        }
        return null;
      }

      function createNumericCell(value, decimals) {
        var display = formatNumber(value, decimals);
        var $td = $('<td></td>').text(display);
        if (!isNaN(value)) {
          $td.attr('data-value', value);
        }
        return $td;
      }

      function createTextCell(text, dataValue, iconClass) {
        var $td = $('<td></td>');
        if (iconClass) {
          $td.append('<i class="' + iconClass + ' me-2"></i>');
        }
        $td.append(document.createTextNode(text || '-'));
        if (dataValue) {
          $td.attr('data-value', dataValue);
        }
        return $td;
      }

      var sortingAttached = false;
      var chartState = {
        accuracyThroughput: null,
        efficiency: null,
        latencyPercentile: null,
        correlation: null,
        effectiveTps: null,
        avgLogprob: null,
        inputTtft: null,
        inputTps: null
      };
      var allModels = metrics && metrics.models ? metrics.models : [];

      function getGpuOptions(models) {
        var seen = {};
        models.forEach(function(model) {
          if (model.gpu) {
            seen[model.gpu] = true;
          }
        });
        return Object.keys(seen).sort();
      }

      function applyGpuFilter(models) {
        var selected = $('#gpuFilter').val() || 'all';
        if (selected === 'all') {
          return models;
        }
        return models.filter(function(model) {
          return model.gpu === selected;
        });
      }

      function initGpuFilter(models) {
        var $filter = $('#gpuFilter');
        if (!$filter.length) {
          return;
        }
        var options = getGpuOptions(models);
        $filter.empty();
        $filter.append('<option value="all">All</option>');
        options.forEach(function(gpu) {
          $filter.append('<option value="' + gpu + '">' + gpu + '</option>');
        });
        $filter.off('change.metrics').on('change.metrics', function() {
          renderAll(applyGpuFilter(models));
        });
      }
      function updateSortIcons($header, direction) {
        $header.closest('tr').find('.sort-icon').each(function() {
          $(this).removeClass('fa-arrow-up fa-arrow-down').addClass('fa-arrows-up-down');
        });

        var $icon = $header.find('.sort-icon');
        if (direction === 'asc') {
          $icon.removeClass('fa-arrows-up-down fa-arrow-down').addClass('fa-arrow-up');
        } else if (direction === 'desc') {
          $icon.removeClass('fa-arrows-up-down fa-arrow-up').addClass('fa-arrow-down');
        }
      }

      function attachSorting() {
        if (sortingAttached) {
          return;
        }
        $('#modelsTable thead th.sortable').each(function(index) {
          var direction = 'none';
          $(this).on('click', function() {
            var type = $(this).data('type');
            direction = direction === 'asc' ? 'desc' : 'asc';
            sortTable(index, type, direction);
            updateSortIcons($(this), direction);
          });
        });
        sortingAttached = true;
      }

      function sortTable(columnIndex, type, direction) {
        var $tbody = $('#modelsTable tbody');
        var rows = $tbody.find('tr').get();
        rows.sort(function(a, b) {
          var A = $(a).children().eq(columnIndex).text();
          var B = $(b).children().eq(columnIndex).text();
          if (type === 'number') {
            A = parseFloat($(a).children().eq(columnIndex).attr('data-value')) || 0;
            B = parseFloat($(b).children().eq(columnIndex).attr('data-value')) || 0;
          }
          if (A < B) {
            return direction === 'asc' ? -1 : 1;
          }
          if (A > B) {
            return direction === 'asc' ? 1 : -1;
          }
          return 0;
        });
        $.each(rows, function(_, row) {
          $tbody.append(row);
        });
      }

      function applyTheme(theme) {
        var selected = theme === 'dark' ? 'dark' : 'light';
        document.documentElement.setAttribute('data-theme', selected);
        var toggle = document.getElementById('themeToggle');
        if (toggle) {
          var icon = toggle.querySelector('i');
          var label = selected === 'dark' ? 'Switch to light mode' : 'Switch to dark mode';
          toggle.setAttribute('aria-label', label);
          if (icon) {
            icon.className = selected === 'dark'
              ? 'fa-duotone fa-regular fa-sun'
              : 'fa-duotone fa-regular fa-moon';
          }
        }
        try {
          localStorage.setItem('agon-theme', selected);
        } catch (e) {}
      }

      function initThemeToggle() {
        var saved = null;
        try {
          saved = localStorage.getItem('agon-theme');
        } catch (e) {}
        applyTheme(saved || 'light');
        var toggle = document.getElementById('themeToggle');
        if (!toggle) {
          return;
        }
        toggle.addEventListener('click', function() {
          var current = document.documentElement.getAttribute('data-theme');
          applyTheme(current === 'dark' ? 'light' : 'dark');
        });
      }

      function populateSummary(models) {
        var grouped = {};
        models.forEach(function(model) {
          var gpu = model.gpu || 'unknown';
          grouped[gpu] = grouped[gpu] || [];
          grouped[gpu].push(model);
        });

        var $container = $('#summaryRows').empty();
        var gpus = Object.keys(grouped).sort();
        if (!gpus.length) {
          return;
        }

        gpus.forEach(function(gpu) {
          var entries = grouped[gpu];
          var bestThroughput = null;
          var bestLatency = null;
          var bestEfficiency = null;
          var bestAccuracy = null;
          var bestTradeoff = null;
          var pareto = [];

          entries.forEach(function(model) {
            var throughput = getBenchmarkThroughput(model);
            if (throughput === null) {
              throughput = model.aggregates && model.aggregates.throughput ? model.aggregates.throughput.avg_tokens_per_second : 0;
            }
            var latency = model.aggregates && model.aggregates.latency ? model.aggregates.latency.avg_total_ms : 0;
            var efficiency = model.aggregates && model.aggregates.efficiency ? model.aggregates.efficiency.accuracy_per_second : 0;
            var accuracy = model.aggregates && model.aggregates.accuracy ? model.aggregates.accuracy.accuracy : 0;
            var paretoFront = model.aggregates && model.aggregates.comparisons ? model.aggregates.comparisons.pareto_front : false;

            if (throughput > 0 && (!bestThroughput || throughput > bestThroughput.value)) {
              bestThroughput = { label: modelLabel(model), value: throughput };
            }
            if (latency > 0 && (!bestLatency || latency < bestLatency.value)) {
              bestLatency = { label: modelLabel(model), value: latency };
            }
            if (efficiency > 0 && (!bestEfficiency || efficiency > bestEfficiency.value)) {
              bestEfficiency = { label: modelLabel(model), value: efficiency };
            }
            if (accuracy > 0 && (!bestAccuracy || accuracy > bestAccuracy.value)) {
              bestAccuracy = { label: modelLabel(model), value: accuracy };
            }
            if (paretoFront) {
              pareto.push({ label: modelLabel(model), value: efficiency || 0 });
            }
          });

          if (pareto.length) {
            pareto.sort(function(a, b) { return b.value - a.value; });
            bestTradeoff = pareto[0];
          } else {
            bestTradeoff = bestEfficiency;
          }

          var row = $('<div class="row g-3 mb-3"></div>');
          var header = $('<div class="col-12"><h6 class="text-uppercase text-muted mb-0">GPU: ' + gpu + '</h6></div>');
          row.append(header);
          row.append(buildSummaryCard('fa-duotone fa-regular fa-rabbit-running fa-fw', 'Fastest Model', bestThroughput));
          row.append(buildSummaryCard('fa-duotone fa-regular fa-gauge-low', 'Best Latency', bestLatency));
          row.append(buildSummaryCard('fa-duotone fa-regular fa-gauge-high', 'Most Efficient', bestEfficiency));
          row.append(buildSummaryCard('fa-duotone fa-regular fa-bullseye-arrow', 'Most Accurate', bestAccuracy));
          row.append(buildSummaryCard('fa-duotone fa-regular fa-code-compare', 'Best Trade-off', bestTradeoff));
          $container.append(row);
        });
      }

      function buildSummaryCard(iconClass, label, entry) {
        var col = $('<div class="col-sm-6 col-lg-2"></div>');
        var card = $('<div class="card shadow-sm h-100"></div>');
        var body = $('<div class="card-body"></div>');
        body.append('<p style="font-size: 1.5em;" class="text-muted mb-1"><i class="' + iconClass + '"></i> ' + label + '</p>');
        body.append('<h5 class="card-title">' + (entry ? entry.label : '-') + '</h5>');
        card.append(body);
        col.append(card);
        return col;
      }

      function populateTable(models) {
        var $tbody = $('#modelsTable tbody').empty();
        models.forEach(function(model) {
          var accuracy = model.aggregates && model.aggregates.accuracy ? model.aggregates.accuracy.accuracy : null;
          var total = model.aggregates && model.aggregates.accuracy ? model.aggregates.accuracy.total : null;
          var deadlineRate = model.aggregates && model.aggregates.reliability ? model.aggregates.reliability.deadline_exceeded_rate : null;
          var avgTtft = model.aggregates && model.aggregates.latency ? model.aggregates.latency.avg_ttft_ms : null;
          var avgTps = model.aggregates && model.aggregates.throughput ? model.aggregates.throughput.avg_tokens_per_second : null;
          if (!avgTps || avgTps === 0) {
            avgTps = getBenchmarkThroughput(model);
          }
          var effTps = model.aggregates && model.aggregates.throughput ? model.aggregates.throughput.avg_effective_tps : null;
          var avgTotal = model.aggregates && model.aggregates.latency ? model.aggregates.latency.avg_total_ms : null;
          var avgOutput = model.aggregates && model.aggregates.token_usage ? model.aggregates.token_usage.avg_output_tokens : null;
          var efficiency = model.aggregates && model.aggregates.efficiency ? model.aggregates.efficiency.accuracy_per_second : null;
          var stability = model.aggregates && model.aggregates.stability ? model.aggregates.stability.tokens_per_second_cv : null;
          var pareto = model.aggregates && model.aggregates.comparisons ? model.aggregates.comparisons.pareto_front : false;

          var $row = $('<tr></tr>');
          $row.append(createTextCell(model.model || '-', model.model, 'fa-duotone fa-regular fa-cube'));
          $row.append(createTextCell(model.gpu || '-', model.gpu, 'fa-duotone fa-regular fa-microchip'));
          $row.append(createNumericCell(accuracy * 100, 1));
          $row.append(createNumericCell(total, 0));
          $row.append(createNumericCell(deadlineRate * 100, 1));
          $row.append(createNumericCell(avgTtft, 0));
          $row.append(createNumericCell(avgTps, 2));
          $row.append(createNumericCell(effTps, 2));
          $row.append(createNumericCell(avgTotal, 0));
          $row.append(createNumericCell(avgOutput, 1));
          $row.append(createNumericCell(efficiency, 3));
          $row.append(createNumericCell(stability, 3));
          $row.append(createTextCell(pareto ? 'yes' : 'no', pareto ? 1 : 0));
          $tbody.append($row);
        });
      }
      function buildAccuracyThroughputChart(models) {
        var canvas = document.getElementById('accuracyThroughputChart');
        if (!canvas) {
          return;
        }
        function getColorForAccuracy(accuracy) {
          if (accuracy >= 70) return '#334155';
          if (accuracy >= 50) return '#64748B';
          if (accuracy >= 35) return '#94A3B8';
          return '#CBD5E1';
        }

        var points = [];
        models.forEach(function(model) {
          var accuracy = model.aggregates && model.aggregates.accuracy ? model.aggregates.accuracy.accuracy : null;
          var throughput = getBenchmarkThroughput(model);
          if (throughput === null) {
            throughput = model.aggregates && model.aggregates.throughput ? model.aggregates.throughput.avg_tokens_per_second : null;
          }
          if (accuracy === null || throughput === null) {
            return;
          }
          points.push({
            x: throughput,
            y: accuracy * 100,
            modelName: modelLabel(model)
          });
        });
        if (points.length === 0) {
          if (chartState.accuracyThroughput) {
            chartState.accuracyThroughput.destroy();
            chartState.accuracyThroughput = null;
          }
          $('#accuracyThroughputEmpty').text('No accuracy data available for this report.');
          return;
        }

        var chartData = points.map(function(point) {
          return {
            x: point.x,
            y: point.y,
            modelName: point.modelName,
            backgroundColor: getColorForAccuracy(point.y)
          };
        });

        var labelPlugin = {
          id: 'modelLabels',
          afterDatasetsDraw: function(chart) {
            var ctx = chart.ctx;
            chart.data.datasets.forEach(function(dataset, datasetIndex) {
              var meta = chart.getDatasetMeta(datasetIndex);
              meta.data.forEach(function(element, index) {
                var data = chartData[index];
                if (!data) {
                  return;
                }
                var modelName = (data.modelName || '').split('@')[0].trim();
                if (!modelName) {
                  return;
                }
                ctx.fillStyle = '#0F172A';
                ctx.font = 'bold 11px sans-serif';
                ctx.textAlign = 'center';
                ctx.textBaseline = 'bottom';
                ctx.fillText(modelName, element.x, element.y - 12);
              });
            });
          }
        };

        if (chartState.accuracyThroughput) {
          chartState.accuracyThroughput.destroy();
        }
        chartState.accuracyThroughput = new Chart(canvas, {
          type: 'scatter',
          data: {
            datasets: [{
              data: chartData,
              pointRadius: 8,
              pointHoverRadius: 12,
              pointBackgroundColor: chartData.map(function(d) { return d.backgroundColor; }),
              pointBorderColor: '#ffffff',
              pointBorderWidth: 2,
              pointHoverBorderWidth: 3
            }]
          },
          options: {
            responsive: true,
            maintainAspectRatio: false,
            animation: false,
            scales: {
              x: {
                title: {
                  display: true,
                  text: 'Throughput (tokens/second)',
                  font: {
                    size: 14,
                    weight: 'bold'
                  },
                  color: '#64748B'
                },
                grid: {
                  color: 'rgba(0, 0, 0, 0.05)'
                },
                ticks: {
                  color: '#64748B'
                }
              },
              y: {
                title: {
                  display: true,
                  text: 'Accuracy (%)',
                  font: {
                    size: 14,
                    weight: 'bold'
                  },
                  color: '#64748B'
                },
                suggestedMin: 0,
                suggestedMax: 100,
                grid: {
                  color: 'rgba(0, 0, 0, 0.05)'
                },
                ticks: {
                  color: '#64748B',
                  callback: function(value) {
                    return value + '%';
                  }
                }
              }
            },
            plugins: {
              legend: {
                display: false
              },
              tooltip: {
                callbacks: {
                  label: function(context) {
                    var point = context.raw || {};
                    var tps = typeof point.x === 'number' ? point.x.toFixed(2) : 'n/a';
                    var acc = typeof point.y === 'number' ? point.y.toFixed(1) : 'n/a';
                    return [
                      'Throughput: ' + tps + ' tokens/sec',
                      'Accuracy: ' + acc + '%'
                    ];
                  },
                  title: function(items) {
                    if (!items.length) {
                      return 'model';
                    }
                    var data = chartData[items[0].dataIndex] || {};
                    return data.modelName || 'model';
                  }
                }
              }
            }
          },
          plugins: [labelPlugin]
        });
      }

      function buildEfficiencyChart(models) {
        var canvas = document.getElementById('efficiencyChart');
        if (!canvas) {
          return;
        }
        var labels = [];
        var values = [];
        models.forEach(function(model) {
          var efficiency = model.aggregates && model.aggregates.efficiency ? model.aggregates.efficiency.accuracy_per_second : null;
          if (efficiency === null || isNaN(efficiency)) {
            return;
          }
          labels.push(modelLabel(model));
          values.push(efficiency);
        });
        if (!values.length) {
          if (chartState.efficiency) {
            chartState.efficiency.destroy();
            chartState.efficiency = null;
          }
          $('#efficiencyEmpty').text('No efficiency data available for this report.');
          return;
        }
        if (chartState.efficiency) {
          chartState.efficiency.destroy();
        }
        chartState.efficiency = new Chart(canvas, {
          type: 'bar',
          data: {
            labels: labels,
            datasets: [{
              label: 'Accuracy per second',
              data: values,
              backgroundColor: '#3B82F6'
            }]
          },
          options: {
            responsive: true,
            maintainAspectRatio: false,
            animation: false,
            scales: {
              x: {
                ticks: { color: '#64748B' }
              },
              y: {
                title: {
                  display: true,
                  text: 'Accuracy per second',
                  color: '#64748B'
                },
                ticks: { color: '#64748B' }
              }
            },
            plugins: {
              legend: { display: false },
              tooltip: {
                callbacks: {
                  label: function(context) {
                    return formatNumber(context.raw, 3);
                  }
                }
              }
            }
          }
        });
      }

      function buildEffectiveTpsChart(models) {
        var canvas = document.getElementById('effectiveTpsChart');
        if (!canvas) {
          return;
        }
        var labels = [];
        var values = [];
        models.forEach(function(model) {
          var effTps = model.aggregates && model.aggregates.throughput ? model.aggregates.throughput.avg_effective_tps : null;
          if (effTps === null || isNaN(effTps)) {
            return;
          }
          labels.push(modelLabel(model));
          values.push(effTps);
        });
        if (!values.length) {
          if (chartState.effectiveTps) {
            chartState.effectiveTps.destroy();
            chartState.effectiveTps = null;
          }
          $('#effectiveTpsEmpty').text('No effective TPS data available for this report.');
          return;
        }
        if (chartState.effectiveTps) {
          chartState.effectiveTps.destroy();
        }
        chartState.effectiveTps = new Chart(canvas, {
          type: 'bar',
          data: {
            labels: labels,
            datasets: [{
              label: 'Effective tokens/sec',
              data: values,
              backgroundColor: '#14B8A6'
            }]
          },
          options: {
            responsive: true,
            maintainAspectRatio: false,
            animation: false,
            scales: {
              x: {
                ticks: { color: '#64748B' }
              },
              y: {
                title: {
                  display: true,
                  text: 'Effective tokens/sec',
                  color: '#64748B'
                },
                ticks: { color: '#64748B' }
              }
            },
            plugins: {
              legend: { display: false },
              tooltip: {
                callbacks: {
                  label: function(context) {
                    return formatNumber(context.raw, 2);
                  }
                }
              }
            }
          }
        });
      }

      function buildAvgLogprobChart(models) {
        var canvas = document.getElementById('avgLogprobChart');
        if (!canvas) {
          return;
        }
        var labels = [];
        var values = [];
        models.forEach(function(model) {
          var avgLogprob = model.aggregates && model.aggregates.throughput ? model.aggregates.throughput.avg_logprob : null;
          if (avgLogprob === null || isNaN(avgLogprob)) {
            return;
          }
          labels.push(modelLabel(model));
          values.push(avgLogprob);
        });
        if (!values.length) {
          if (chartState.avgLogprob) {
            chartState.avgLogprob.destroy();
            chartState.avgLogprob = null;
          }
          $('#avgLogprobEmpty').text('No logprob data available for this report.');
          return;
        }
        if (chartState.avgLogprob) {
          chartState.avgLogprob.destroy();
        }
        chartState.avgLogprob = new Chart(canvas, {
          type: 'bar',
          data: {
            labels: labels,
            datasets: [{
              label: 'Average logprob',
              data: values,
              backgroundColor: '#64748B'
            }]
          },
          options: {
            responsive: true,
            maintainAspectRatio: false,
            animation: false,
            scales: {
              x: {
                ticks: { color: '#64748B' }
              },
              y: {
                title: {
                  display: true,
                  text: 'Average logprob',
                  color: '#64748B'
                },
                ticks: { color: '#64748B' }
              }
            },
            plugins: {
              legend: { display: false },
              tooltip: {
                callbacks: {
                  label: function(context) {
                    return formatNumber(context.raw, 3);
                  }
                }
              }
            }
          }
        });
      }

      function buildLatencyPercentileChart(models) {
        var canvas = document.getElementById('latencyPercentileChart');
        if (!canvas) {
          return;
        }
        var labels = [];
        var p50 = [];
        var p90 = [];
        var p95 = [];
        models.forEach(function(model) {
          var latency = model.aggregates ? model.aggregates.latency : null;
          if (!latency) {
            return;
          }
          labels.push(modelLabel(model));
          p50.push(latency.median_ttft_ms || 0);
          p90.push(latency.p90_ttft_ms || 0);
          p95.push(latency.p95_ttft_ms || 0);
        });
        if (!labels.length) {
          if (chartState.latencyPercentile) {
            chartState.latencyPercentile.destroy();
            chartState.latencyPercentile = null;
          }
          $('#latencyPercentileEmpty').text('No latency percentile data available for this report.');
          return;
        }
        if (chartState.latencyPercentile) {
          chartState.latencyPercentile.destroy();
        }
        chartState.latencyPercentile = new Chart(canvas, {
          type: 'bar',
          data: {
            labels: labels,
            datasets: [
              { label: 'P50 TTFT (ms)', data: p50, backgroundColor: '#94A3B8' },
              { label: 'P90 TTFT (ms)', data: p90, backgroundColor: '#64748B' },
              { label: 'P95 TTFT (ms)', data: p95, backgroundColor: '#334155' }
            ]
          },
          options: {
            responsive: true,
            maintainAspectRatio: false,
            animation: false,
            scales: {
              x: { ticks: { color: '#64748B' } },
              y: {
                title: { display: true, text: 'TTFT (ms)', color: '#64748B' },
                ticks: { color: '#64748B' }
              }
            },
            plugins: {
              legend: {
                position: 'bottom',
                labels: { color: '#64748B' }
              }
            }
          }
        });
      }

      function buildCorrelationChart(models) {
        var canvas = document.getElementById('correlationChart');
        if (!canvas) {
          return;
        }
        var labels = [];
        var accVsTps = [];
        var accVsLatency = [];
        models.forEach(function(model) {
          var corr = model.aggregates ? model.aggregates.correlations : null;
          if (!corr) {
            return;
          }
          labels.push(modelLabel(model));
          accVsTps.push(corr.accuracy_vs_throughput || 0);
          accVsLatency.push(corr.accuracy_vs_total_ms || 0);
        });
        if (!labels.length) {
          if (chartState.correlation) {
            chartState.correlation.destroy();
            chartState.correlation = null;
          }
          $('#correlationEmpty').text('No correlation data available for this report.');
          return;
        }
        if (chartState.correlation) {
          chartState.correlation.destroy();
        }
        chartState.correlation = new Chart(canvas, {
          type: 'bar',
          data: {
            labels: labels,
            datasets: [
              { label: 'Accuracy vs Throughput', data: accVsTps, backgroundColor: '#10B981' },
              { label: 'Accuracy vs Total Latency', data: accVsLatency, backgroundColor: '#F59E0B' }
            ]
          },
          options: {
            responsive: true,
            maintainAspectRatio: false,
            animation: false,
            scales: {
              x: { ticks: { color: '#64748B' } },
              y: {
                min: -1,
                max: 1,
                title: { display: true, text: 'Correlation', color: '#64748B' },
                ticks: { color: '#64748B' }
              }
            },
            plugins: {
              legend: { position: 'bottom', labels: { color: '#64748B' } },
              tooltip: {
                callbacks: {
                  label: function(context) {
                    return formatNumber(context.raw, 3);
                  }
                }
              }
            }
          }
        });
      }
      function buildInputTokenCharts(models) {
        var ttftCanvas = document.getElementById('inputTokensTtftChart');
        var tpsCanvas = document.getElementById('inputTokensTpsChart');
        if (!ttftCanvas || !tpsCanvas) {
          return;
        }

        var palette = [
          '#334155', '#64748B', '#94A3B8', '#CBD5E1', '#3B82F6',
          '#1D4ED8', '#0EA5E9', '#38BDF8', '#14B8A6', '#10B981'
        ];
        var modelColors = {};
        models.forEach(function(model, index) {
          modelColors[modelLabel(model)] = palette[index % palette.length];
        });

        function collectPoints(selectedLabel) {
          var datasets = [];
          models.forEach(function(model) {
            var label = modelLabel(model);
            if (selectedLabel && selectedLabel !== 'all' && label !== selectedLabel) {
              return;
            }
            var records = model.accuracy || [];
            if (!records.length) {
              return;
            }
            var color = modelColors[label] || '#64748B';
            var ttftPoints = [];
            var tpsPoints = [];
            records.forEach(function(record) {
              var inputTokens = Number(record.input_tokens);
              if (isNaN(inputTokens)) {
                return;
              }
              var ttftMs = Number(record.time_to_first_token);
              if (!isNaN(ttftMs)) {
                ttftPoints.push({ x: inputTokens, y: ttftMs, modelName: label });
              }
              var tps = Number(record.tokens_per_second);
              if (!isNaN(tps)) {
                tpsPoints.push({ x: inputTokens, y: tps, modelName: label });
              }
            });
            if (ttftPoints.length) {
              datasets.push({
                model: label,
                color: color,
                ttft: ttftPoints,
                tps: tpsPoints
              });
            }
          });
          return datasets;
        }

        function buildChart(canvas, datasets, yLabel, yFormatter) {
          return new Chart(canvas, {
            type: 'scatter',
            data: {
              datasets: datasets.map(function(dataset) {
                return {
                  label: dataset.model,
                  data: yLabel.indexOf('TTFT') !== -1 ? dataset.ttft : dataset.tps,
                  backgroundColor: dataset.color,
                  borderColor: '#ffffff',
                  borderWidth: 1,
                  pointRadius: 6,
                  pointHoverRadius: 9
                };
              })
            },
            options: {
              responsive: true,
              maintainAspectRatio: false,
              animation: false,
              scales: {
                x: {
                  title: {
                    display: true,
                    text: 'Input tokens',
                    font: { size: 13, weight: 'bold' },
                    color: '#64748B'
                  },
                  grid: { color: 'rgba(0, 0, 0, 0.05)' },
                  ticks: { color: '#64748B' }
                },
                y: {
                  title: {
                    display: true,
                    text: yLabel,
                    font: { size: 13, weight: 'bold' },
                    color: '#64748B'
                  },
                  grid: { color: 'rgba(0, 0, 0, 0.05)' },
                  ticks: {
                    color: '#64748B',
                    callback: yFormatter
                  }
                }
              },
              plugins: {
                legend: {
                  position: 'bottom',
                  labels: {
                    usePointStyle: true,
                    boxWidth: 8,
                    color: '#64748B'
                  }
                },
                tooltip: {
                  callbacks: {
                    title: function(items) {
                      if (!items.length) {
                        return 'model';
                      }
                      var point = items[0].raw || {};
                      return point.modelName || items[0].dataset.label || 'model';
                    },
                    label: function(context) {
                      var point = context.raw || {};
                      var x = typeof point.x === 'number' ? point.x.toFixed(0) : 'n/a';
                      var y = typeof point.y === 'number' ? point.y.toFixed(2) : 'n/a';
                      return 'Input tokens: ' + x + ', ' + yLabel + ': ' + y;
                    }
                  }
                }
              }
            }
          });
        }

        var filter = $('#inputTokenModelFilter').empty();
        filter.append('<option value="all">All models</option>');
        models.forEach(function(model) {
          filter.append('<option value="' + modelLabel(model) + '">' + modelLabel(model) + '</option>');
        });

        function renderCharts() {
          if (chartState.inputTtft) {
            chartState.inputTtft.destroy();
          }
          if (chartState.inputTps) {
            chartState.inputTps.destroy();
          }
          $('#inputTokensTtftEmpty').text('');
          $('#inputTokensTpsEmpty').text('');

          var selected = filter.val();
          var datasets = collectPoints(selected);
          if (!datasets.length) {
            chartState.inputTtft = null;
            chartState.inputTps = null;
            $('#inputTokensTtftEmpty').text('No input token data available for this selection.');
            $('#inputTokensTpsEmpty').text('No input token data available for this selection.');
            return;
          }
          chartState.inputTtft = buildChart(ttftCanvas, datasets, 'TTFT (ms)', function(value) { return Math.round(value) + ' ms'; });
          chartState.inputTps = buildChart(tpsCanvas, datasets, 'Tokens per second', function(value) { return Math.round(value); });
        }

        filter.on('change', renderCharts);
        renderCharts();
      }

      function buildAccordion(models) {
        var $accordion = $('#modelAccordion').empty();
        if (!models.length) {
          $accordion.append('<div class="text-muted">No model details available.</div>');
          return;
        }
        models.forEach(function(model, index) {
          var bundleId = 'model_' + index;
          var headerId = 'heading_' + index;
          var label = modelLabel(model);
          var accuracy = model.aggregates && model.aggregates.accuracy ? model.aggregates.accuracy : {};
          var latency = model.aggregates && model.aggregates.latency ? model.aggregates.latency : {};
          var throughput = model.aggregates && model.aggregates.throughput ? model.aggregates.throughput : {};
          var benchmark = model.aggregates && model.aggregates.benchmark ? model.aggregates.benchmark : {};
          var reliability = model.aggregates && model.aggregates.reliability ? model.aggregates.reliability : {};
          var efficiency = model.aggregates && model.aggregates.efficiency ? model.aggregates.efficiency : {};
          var distributions = model.aggregates && model.aggregates.distributions ? model.aggregates.distributions : {};
          var correlations = model.aggregates && model.aggregates.correlations ? model.aggregates.correlations : {};
          var metadata = model.aggregates && model.aggregates.metadata ? model.aggregates.metadata : {};

          var bodyParts = [];
          bodyParts.push('<div class="row">');
          bodyParts.push('<div class="col-md-6">');
          bodyParts.push('<h6>Accuracy</h6><ul class="list-unstyled mb-3">');
          bodyParts.push('<li><strong>Accuracy:</strong> ' + formatPercent(accuracy.accuracy, 1) + '</li>');
          bodyParts.push('<li><strong>Error rate:</strong> ' + formatPercent(accuracy.error_rate, 1) + '</li>');
          bodyParts.push('<li><strong>Avg difficulty:</strong> ' + formatNumber(accuracy.avg_difficulty, 2) + '</li>');
          bodyParts.push('<li><strong>Avg margin:</strong> ' + formatNumber(accuracy.avg_margin_of_error, 2) + '</li>');
          bodyParts.push('</ul><h6>Latency</h6><ul class="list-unstyled mb-3">');
          bodyParts.push('<li><strong>Avg TTFT:</strong> ' + formatNumber(latency.avg_ttft_ms, 0) + ' ms</li>');
          bodyParts.push('<li><strong>TTFT P50/P90/P95:</strong> ' + formatNumber(latency.median_ttft_ms, 0) + ' / ' + formatNumber(latency.p90_ttft_ms, 0) + ' / ' + formatNumber(latency.p95_ttft_ms, 0) + ' ms</li>');
          bodyParts.push('<li><strong>Avg total:</strong> ' + formatNumber(latency.avg_total_ms, 0) + ' ms</li>');
          bodyParts.push('</ul>');
          bodyParts.push('<h6>Reliability</h6><ul class="list-unstyled mb-3">');
          bodyParts.push('<li><strong>Deadline rate:</strong> ' + formatPercent(reliability.deadline_exceeded_rate, 1) + '</li>');
          bodyParts.push('<li><strong>Timeout rate:</strong> ' + formatPercent(reliability.timeout_rate, 1) + '</li>');
          bodyParts.push('</ul>');
          bodyParts.push('</div>');
          bodyParts.push('<div class="col-md-6">');
          bodyParts.push('<h6>Throughput &amp; Efficiency</h6><ul class="list-unstyled mb-3">');
          bodyParts.push('<li><strong>Avg TPS:</strong> ' + formatNumber(throughput.avg_tokens_per_second, 2) + '</li>');
          bodyParts.push('<li><strong>Effective TPS:</strong> ' + formatNumber(throughput.avg_effective_tps, 2) + '</li>');
          bodyParts.push('<li><strong>Avg logprob:</strong> ' + formatNumber(throughput.avg_logprob, 3) + '</li>');
          bodyParts.push('<li><strong>TPS P90:</strong> ' + formatNumber(throughput.p90_tokens_per_second, 2) + '</li>');
          bodyParts.push('<li><strong>Accuracy per second:</strong> ' + formatNumber(efficiency.accuracy_per_second, 3) + '</li>');
          bodyParts.push('<li><strong>Tokens/sec per param:</strong> ' + formatNumber(efficiency.tokens_per_second_per_param, 6) + '</li>');
          bodyParts.push('</ul>');
          bodyParts.push('<h6>Benchmarks</h6><ul class="list-unstyled mb-3">');
          bodyParts.push('<li><strong>Prompt TPS:</strong> ' + formatNumber(benchmark.prompt_tokens_per_second, 2) + '</li>');
          bodyParts.push('<li><strong>Generation TPS:</strong> ' + formatNumber(benchmark.generation_tokens_per_second, 2) + '</li>');
          bodyParts.push('<li><strong>Avg benchmark (ms):</strong> ' + formatNumber((benchmark.avg_benchmark_ns || 0) / 1000000, 2) + '</li>');
          bodyParts.push('<li><strong>Run count:</strong> ' + formatNumber(benchmark.run_count, 0) + '</li>');
          bodyParts.push('</ul>');
          bodyParts.push('<h6>Correlations</h6><ul class="list-unstyled mb-3">');
          bodyParts.push('<li><strong>Acc vs TPS:</strong> ' + formatNumber(correlations.accuracy_vs_throughput, 3) + '</li>');
          bodyParts.push('<li><strong>Acc vs Total:</strong> ' + formatNumber(correlations.accuracy_vs_total_ms, 3) + '</li>');
          bodyParts.push('<li><strong>TTFT vs Input:</strong> ' + formatNumber(correlations.ttft_vs_input_tokens, 3) + '</li>');
          bodyParts.push('</ul>');
          bodyParts.push('<h6>Distributions</h6><ul class="list-unstyled mb-3">');
          if (distributions.ttft_ms) {
            bodyParts.push('<li><strong>TTFT stddev:</strong> ' + formatNumber(distributions.ttft_ms.stddev, 2) + '</li>');
          }
          if (distributions.tokens_per_second) {
            bodyParts.push('<li><strong>TPS stddev:</strong> ' + formatNumber(distributions.tokens_per_second.stddev, 2) + '</li>');
          }
          bodyParts.push('</ul>');
          bodyParts.push('<h6>Metadata</h6><ul class="list-unstyled mb-3">');
          bodyParts.push('<li><strong>Backend:</strong> ' + (metadata.backend || '-') + '</li>');
          bodyParts.push('<li><strong>Model type:</strong> ' + (metadata.model_type || '-') + '</li>');
          bodyParts.push('<li><strong>Context size:</strong> ' + formatNumber(metadata.context_size, 0) + '</li>');
          bodyParts.push('</ul>');
          bodyParts.push('</div>');
          bodyParts.push('</div>');
          var body = bodyParts.join('');

          var $item = $('<div class="accordion-item"></div>');
          var header = ''
            + '<h2 class="accordion-header" id="' + headerId + '">' 
            + '<button class="accordion-button collapsed" type="button" data-bs-toggle="collapse" data-bs-target="#' + bundleId + '" aria-expanded="false" aria-controls="' + bundleId + '">' 
            + label
            + '</button>'
            + '</h2>';
          var content = ''
            + '<div id="' + bundleId + '" class="accordion-collapse collapse" aria-labelledby="' + headerId + '" data-bs-parent="#modelAccordion">'
            + '<div class="accordion-body">' + body + '</div>'
            + '</div>';
          $item.append(header);
          $item.append(content);
          $accordion.append($item);
        });
      }

      function renderAll(models) {
        populateSummary(models);
        populateTable(models);
        attachSorting();
        buildAccuracyThroughputChart(models);
        buildEfficiencyChart(models);
        buildEffectiveTpsChart(models);
        buildAvgLogprobChart(models);
        buildLatencyPercentileChart(models);
        buildCorrelationChart(models);
        buildInputTokenCharts(models);
        buildAccordion(models);
      }

      $(function() {
        initThemeToggle();
        if (!metrics) {
          return;
        }

        $('#generatedAt').text(new Date().toLocaleString());
        initGpuFilter(allModels);
        renderAll(applyGpuFilter(allModels));
      });
    })(jQuery);
  </script>
</body>
</html>
`
