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

## Implementation steps

## Progress update
- Completed:
  - Added llama.cpp ChatProvider at internal/providers/llamacpp/provider.go with /models load and /v1/chat/completions streaming/non-streaming.
  - Added llama.cpp LLMHost at internal/models/llama_host.go for /models list and /models/unload.
  - Added a multiplex provider to route calls by host.Type without impacting Ollama behavior.
  - Updated provider factory to select llama.cpp when host.Type is "llama.cpp" and to multiplex mixed host types.
  - Updated model host creation and unload flow to support llama.cpp hosts.
- In progress:
  - Documentation updates (README + config examples).
  - Test coverage for /models parsing and SSE streaming.
- Not started:
  - RAG embeddings for llama.cpp (pending endpoint confirmation).
  - Tool-calling support for llama.cpp (pending compatibility check).

### 1) Config and host types
- Add a new host type string ("llama.cpp" or "llamacpp") in config docs and examples.
- Update config example JSONs and README to show llama.cpp hosts and router-mode setup.
- Update internal/models/commands.go createHosts() to instantiate LlamaCppHost when host.Type matches.
- Add any llama.cpp-specific optional fields only if needed (avoid new schema changes unless necessary).

### 2) Llama.cpp model management (LLMHost)
Create internal/models/llama_host.go implementing LLMHost with the router endpoints:
- ListRawModels/ListModels:
  - Call GET {host.URL}/models
  - Parse list and status (loaded/loading/unloaded) and return formatted list.
- GetRunningModels:
  - Filter models from GET /models where status == "loaded".
- UnloadModel:
  - Call POST {host.URL}/models/unload with {"model": "..."}.
- PullModel/DeleteModel/GetModelParameters:
  - Not supported in router mode. Implement as no-op + user-facing message or return a clear error.
  - Note: ListModelParameters currently assumes Ollama /api/show; keep llama.cpp as unsupported.

### 3) Llama.cpp chat provider (ChatProvider)
Create internal/providers/llamacpp/provider.go:
- LoadedModels(ctx, host):
  - Reuse GET /models logic to report loaded models.
- EnsureModelReady(ctx, host, model):
  - Prefer POST /models/load if router mode is enabled.
  - If 404/unsupported, fall back to a lightweight chat/completions request to trigger auto-loading.
- Stream(ctx, req, callbacks):
  - Use POST {host.URL}/v1/chat/completions
  - Support streaming with SSE (data: {json} ... data: [DONE]).
  - For non-streaming, parse the JSON response and deliver a single OnChunk.
  - Map existing request data to OpenAI fields:
    - model: req.Model
    - messages: req.History + system prompt
    - temperature/top_p/etc from req.Parameters (verify llama.cpp accepts the fields)
    - stream: !req.DisableStreaming
  - Tools:
    - If llama.cpp supports OpenAI tool calls, pass tools in OpenAI format.
    - Otherwise, skip tool payloads and note in logs (or gate behind a config flag).
  - JSON mode:
    - If supported, map to OpenAI response_format (json_object) or vendor-specific flag.
    - If unsupported, ignore with a log message.

### 4) Provider selection and factory
- Update internal/providerfactory/factory.go to select llama.cpp provider when any host.Type is llama.cpp.
- If mixed host types are allowed, decide selection strategy:
  - Option A: choose provider per host (preferred, but requires refactor).
  - Option B: reject mixed types and return a config error.
- Update CLI initialization to surface a clear error if host types are mixed and unsupported.

### 5) RAG embedding support (optional, gated)
- Investigate llama.cpp embedding endpoint availability (OpenAI /v1/embeddings or custom).
- If supported:
  - Add a llama.cpp branch in internal/rag/embedding.go for embeddings.
- If not supported:
  - Keep embeddings Ollama-only and document limitation.

### 6) Logging, metrics, and error handling
- Log all llama.cpp HTTP calls using existing logging helpers (AGON->LLM, LLM->AGON).
- Ensure timeouts use cfg.RequestTimeout().
- Return clear errors when router endpoints are missing or return unexpected schema.

### 7) Documentation updates
- README: add llama.cpp router-mode setup and example config.
- Config examples: add a llama.cpp host variant.
- Add a brief compatibility matrix: Ollama vs llama.cpp features (model pull/delete, list, load/unload, tools, JSON mode).

## Testing plan
- Unit tests for llama.cpp host parsing of /models responses (loaded/loading/unloaded).
- Provider tests:
  - Non-streaming chat response parsing.
  - Streaming SSE parsing and chunk forwarding.
- Manual tests:
  - llama-server in router mode with multiple GGUF files.
  - Verify list models, load/unload, and chat routing by model name.

## Risks and open questions
- Confirm llama.cpp /models response schema (fields and status values).
- Confirm which OpenAI parameters llama.cpp accepts (top_k, repeat_penalty, min_p, etc.).
- Confirm tool-calling support and JSON mode compatibility.
- Decide behavior when host types are mixed in config.
- Decide whether to store llama.cpp-specific settings in config or rely on server CLI flags.

## Suggested rollout
- Phase 1: Basic provider + /models list/load/unload + chat (streaming and non-streaming).
- Phase 2: JSON mode + tool calls (if supported), embeddings.
- Phase 3: Improved UX (model status display, richer errors, docs polish).
