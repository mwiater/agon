# Tests

## Coverage Run

- Coverage summary provided from the last run:

```
github.com/mwiater/agon/cmd/agon                coverage: 0.0% of statements
ok      github.com/mwiater/agon/internal/accuracy       (cached)        coverage: 36.7% of statements
ok      github.com/mwiater/agon/internal/appconfig      (cached)        coverage: 47.4% of statements
        github.com/mwiater/agon/internal/benchmark              coverage: 0.0% of statements
        github.com/mwiater/agon/internal/chat           coverage: 0.0% of statements
ok      github.com/mwiater/agon/internal/commands       (cached)        coverage: 50.3% of statements
        github.com/mwiater/agon/internal/logging                coverage: 0.0% of statements
ok      github.com/mwiater/agon/internal/metrics        (cached)        coverage: 14.1% of statements
ok      github.com/mwiater/agon/internal/models (cached)        coverage: 39.3% of statements
ok      github.com/mwiater/agon/internal/providerfactory        (cached)        coverage: 63.3% of statements
?       github.com/mwiater/agon/internal/providers      [no test files]
ok      github.com/mwiater/agon/internal/providers/llamacpp     (cached)        coverage: 63.2% of statements
        github.com/mwiater/agon/internal/providers/mcp          coverage: 0.0% of statements
ok      github.com/mwiater/agon/internal/providers/multiplex    (cached)        coverage: 66.7% of statements
ok      github.com/mwiater/agon/internal/rag    (cached)        coverage: 16.3% of statements
ok      github.com/mwiater/agon/internal/tui    (cached)        coverage: 29.1% of statements
ok      github.com/mwiater/agon/internal/util   (cached)        coverage: 98.3% of statements
ok      github.com/mwiater/agon/servers/benchmark       (cached)        coverage: 23.7% of statements
        github.com/mwiater/agon/servers/mcp             coverage: 0.0% of statements
        github.com/mwiater/agon/servers/mcp/tools               coverage: 0.0% of statements
```

## Coverage Improvement Plan

Goal: dramatically raise overall coverage by prioritizing zero-coverage packages and high-branch logic, then expanding integration coverage.

1) Establish coverage tooling
- Fix the Go toolchain issue so `-coverprofile` works (install/enable `covdata` in the toolchain or pin a supported Go version).
- Add a documented test command and a minimal CI target that runs `go test ./... -coverprofile` and uploads coverage artifacts.

2) Zero-coverage packages first (highest ROI)
- `internal/logging`: add tests for log file rotation, log formatting, and debug flag behavior.
- `internal/chat`: add tests for message history building, JSON mode toggles, and error paths.
- `internal/benchmark`: add unit tests around request building, runner logic, and error handling for benchmark endpoints.
- `internal/providers/mcp`: add tests for tool schema mapping, request/response marshaling, and error cases.
- `servers/mcp` and `servers/mcp/tools`: add tests for tool dispatch, tool schema validation, and result handling.
- `cmd/agon`: add tests for CLI wiring, flag parsing, and command execution with fake configs.

3) Expand coverage in low-coverage packages
- `internal/metrics`: cover report generation, aggregation math, and edge cases (empty data, missing fields).
- `internal/rag`: test retrieval filters, scoring, and context assembly; add tests for exclusion globs and token limits.
- `internal/tui`: test view rendering for JSON/MCP badges and param display; add model selection and error-state tests.

4) Add provider-level integration tests
- `internal/providers/llamacpp`: add tests for non-JSON responses, SSE fallback parsing, and tool-call handling paths.
- `internal/providers/multiplex`: add tests for parallel stream coordination and error fan-in.

5) Add accuracy/benchmark scenario coverage
- `internal/accuracy`: add tests for response parsing in last-25-char logic, tolerance matching, and RAG on/off result flows.
- `servers/benchmark`: expand tests to include config validation, args building per GPU, and failure modes.

6) Raise config coverage
- Add tests that validate `parameterTemplate` requirement and parameter merging overrides in `appconfig`.
- Add tests for legacy config fallback with parameter templates and error messages.

7) Build a coverage map and targets
- Establish package targets (e.g., 60% for `internal/accuracy`, 70% for `internal/providers`, 80% for `internal/appconfig`).
- Track progress in `TESTS.md` and revise targets after the first pass.
