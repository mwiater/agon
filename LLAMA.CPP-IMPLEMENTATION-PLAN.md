# LLAMA.CPP IMPLEMENTATION PLAN

## Source summary (Hugging Face blog)
- llama.cpp server now includes a "router mode" for model management.
- Router mode runs a separate process per model; crashes are isolated per model.
- Auto-discovery scans the llama.cpp cache (LLAMA_CACHE or ~/.cache/llama.cpp) or a custom --models-dir folder for GGUF files.
- On-demand loading: first request loads a model automatically.
- LRU eviction: when --models-max is reached, the least recently used model unloads.
- Request routing: the "model" field in the request selects the model.
- Model management endpoints:
  - GET /models (list models + status)
  - POST /models/load (explicitly load)
  - POST /models/unload (explicitly unload)
- OpenAI-compatible API remains available for chat (example uses /v1/chat/completions).
- Key options: --models-dir, --models-max, --no-models-autoload, --models-preset.

## Goals
- Add a new provider type for llama.cpp alongside Ollama.
- Support router-mode model management (list, load, unload) where the server exposes it.
- Reuse existing config + CLI workflows with minimal disruption.
- Preserve existing Ollama and MCP behavior.

## Non-goals (initial scope)
- Implement llama.cpp model download/pull by name (llama.cpp uses local GGUF files; download is done via llama-server -hf or manual file management).
- Implement llama.cpp-specific tuning UI beyond existing parameter fields.
- Replace the existing provider factory or MCP pipeline behavior.

## Architectural decisions
- Add a new ChatProvider implementation: internal/providers/llamacpp.
- Add a new LLMHost implementation: internal/models/llama_host.go for model list/load/unload integration.
- Use the OpenAI-compatible HTTP endpoints for chat streaming.
- Support router mode endpoints for model management when present; gracefully degrade if not supported.
- Use host.Type = "llama.cpp" (or "llamacpp") in config; keep Ollama as default.

## Remaining work

### 1) Validate /models schema against real server output
- Run `go run scripts/llamacpp_integration_check.go -config config/config.example.LlamaCpp.json`.
- Capture and store the raw `/models` JSON output for the target server version.
- Compare observed fields/status values to the current parser:
  - `data` or `models` arrays
  - fields `id`, `name`, `model`, `path`
  - status formats: string vs `{ "value": "..." }`
- If any fields/status variants are missing:
  - Add parsing support in `internal/models/llama_host.go` and `internal/providers/llamacpp/provider.go`.
  - Extend `internal/models/llama_host_test.go` with the new schema variant.
- Decide on the canonical display name if multiple identifiers are present (document in README if needed).

### 2) Verify accepted OpenAI parameters for llama.cpp
- Use the integration script param probe output to identify accepted/rejected fields.
- For any rejected fields:
  - Remove or conditionally omit them in `applyParameters()` (or log if provided).
  - Update `README.md` to reflect the supported subset for llama.cpp.
- Re-run `go test ./...` after any mapping changes.

### 3) Confirm tool-calling and JSON mode behavior on real server
- With a llama.cpp model that advertises tools, run a chat that triggers a tool call.
- Verify:
  - `tools` + `tool_choice: "auto"` are accepted without errors.
  - Responses return `tool_calls` in either `message` (non-streaming) or `delta` (streaming).
  - MCP execution path returns tool output correctly.
- If the server rejects tools or omits tool calls:
  - Update the no-capability detection strings in `isNoToolCapabilityResponse`.
  - Document the limitation in `README.md` under the compatibility matrix.
- For JSON mode:
  - Send a request with `response_format: {"type":"json_object"}`.
  - If rejected, add a fallback to omit `response_format` and log the behavior.

### 4) Decide llama.cpp-specific config surface (if any)
- Determine if any server flags should be expressed in config (e.g., router mode assumptions).
- If needed:
  - Add new optional fields to `internal/appconfig.Config` and document them in `README.md`.
  - Keep defaults aligned with current behavior (no breaking changes).
- If not needed:
  - Document that llama.cpp settings are controlled via server CLI flags.

### 5) Optional: llama.cpp embeddings support (deferred)
- Investigate whether llama.cpp supports `/v1/embeddings` or a custom embedding endpoint.
- If supported:
  - Add a llama.cpp branch in `internal/rag/embedding.go`.
  - Add unit tests covering the new path.
- If not supported:
  - Document that embeddings remain Ollama-only.

## Validation plan
- Run `go test ./...` after any code changes.
- Run the integration script against a real llama.cpp router-mode server:
  - Capture `/models` raw output.
  - Capture param probe output for supported fields.
  - Record tool-call + JSON mode behavior.
