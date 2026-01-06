# Current Accuracy Workflow

Command:
`go run cmd/agon/main.go accuracy --config config/config.AccuracyMode01.llamacpp01.json`

1) CLI startup and config load
- The `agon` CLI boots and parses `--config config/config.AccuracyMode01.llamacpp01.json`.
- Config is unmarshaled into `appconfig.Config` and stored as the active config.
- Logging is initialized (defaults to `agon.log` unless overridden).

2) Accuracy command execution
- `internal/cli/accuracy.go` calls `accuracy.RunAccuracy(GetConfig())`.
- `RunAccuracy` validates:
  - `AccuracyMode` is enabled in the config.
  - At least one host exists.
  - Each host has exactly one model (required for accuracy runs).
- `timeoutSeconds` comes from `cfg.RequestTimeout()` (default 600s).

3) Prompt suite load
- Prompts are read from `accuracy/accuracy_prompts.json`.
- Validates there is at least one test and `system_prompt` is not empty.

4) Results directory creation
- Ensures the output directory exists:
  - `agonData/modelAccuracy/`

5) Provider setup per host
- For each host in the config:
  - `providerfactory.NewChatProvider(cfg)` creates a llama.cpp provider.
  - `EnsureModelReady` is called for the single model on that host (router-mode `/models/load` when available).

6) Accuracy execution loop
- For each prompt in the suite:
  - Logs progress: `[i/total] Host / Model - Prompt: <prompt>`
  - Sends a non-streaming chat request via llama.cpp `/v1/chat/completions` with:
    - the suite `system_prompt`
    - the user prompt
  - Collects response text + timing/token metadata.

7) Correctness scoring
- Parses integer tokens from the response.
- Compares to expected answer with margin-of-error tolerance.
- Logs result: `[i/total] ... Result: correct=... response=... expected=...`

8) Output saved (JSONL)
- Each prompt produces one JSONL line written to:
  - `agonData/modelAccuracy/<model>.jsonl`
- Each line includes:
  - timestamp, host, model, prompt ID
  - prompt, expected answer, response, correctness
  - timing/token metrics
  - deadline flags (if any)

9) Deadline-exceeded handling
- If a request times out or returns a deadline exceeded error:
  - Writes a JSONL line with `deadlineExceeded=true`.
  - Response field contains the error text.

10) Completion
- After all prompts and hosts complete, the command exits.
- Results remain in `agonData/modelAccuracy/`.
