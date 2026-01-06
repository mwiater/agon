# RAG implementation plan for Agon

This plan turns the RAG idea into a staged, verifiable implementation. Each section lists the changes and a short set of commands to validate that section before moving on. It includes the accuracy-focused corpus, prompt guidance, and reporting notes needed for a verifiable RAG lift.

## External resources and installations

Goal: be explicit about what external services, models, and storage are required or optional.

Required (minimum, JSONL-only store)
- Embedding model available on a llama.cpp host (example: `mxbai-embed-large:335m`).
- A local JSONL index stored in the repo (ex: `rag_corpus_accuracy_v1/index.jsonl`).
- No external vector DB or SQLite extension is required for the initial implementation.

Optional (future, not required for initial JSONL path)
- Local SQLite file with vector extension (if you later choose to use `sqlite-vss` or `sqlite-vec`).
- A standalone vector DB (Qdrant, Chroma, Milvus) if you later prefer a server-backed store.

Installation notes (JSONL-only)
- No extra install is needed beyond the embedding model already in llama.cpp.
- The index file is rebuilt locally with `agon rag index` and committed to the repo for repeatable benchmarks.

Verification
```powershell
agon list models
rg -n "mxbai-embed-large" config
```

## Section 1: Config, data model, and CLI surface

Goal: introduce explicit RAG configuration with a boolean `ragMode` toggle, and update types to carry rag metadata.

Changes
- Add a RAG config block to `internal/appconfig/appconfig.go` with a boolean `ragMode`. If `ragMode` is `false`, all other RAG configuration values are ignored. If `ragMode` is `true`, the remaining RAG fields are honored.
- Keep storage simple for v1: `indexPath` points to a JSONL file (ex: `rag_corpus_accuracy_v1/index.jsonl`). Do not add DB or external store settings yet.
- Include ingestion fields for corpus discovery (ex: `corpusPath`, `allowedExtensions`, `excludeGlobs`).
- Include retrieval fields (ex: `embeddingModel`, `embeddingHost`, `chunkSizeTokens`, `chunkOverlapTokens`, `topK`, `contextTokenLimit`, `similarity: "cosine"`).
- Extend `accuracy/types.go` with RAG fields on `AccuracyResult` (ex: `rag_mode`, `retrieval_ms`, `context_tokens`, `top_k`, `source_coverage`, `citations_used`).
- Add a new CLI group under `internal/cli` for RAG lifecycle (index/build) and ensure `agon show config` surfaces the new RAG config.
- Add example config(s) under `config/` (ex: `config.example.RAGAccuracy.json`) showing `ragMode: true` with the embedding model defined.

Verify
```powershell
rg -n "\"rag\"" internal/appconfig/appconfig.go config
go test ./internal/appconfig
agon show config --config config/config.example.RAGAccuracy.json
```

## Section 2: Accuracy-focused corpus and indexing pipeline

Goal: add a versioned corpus that targets accuracyMode prompts and build embeddings into JSONL.

Changes
- Create `rag_corpus_accuracy_v1/` (versioned) with focused docs:
  - `facts_and_constants.md` (atomic numbers, biology, geometry, calendar facts, human body facts, computing, US states, physical constants, planet facts)
  - `units_and_percent.md` (meters to cm, percent definition/examples)
  - `logic_and_traps.md` (contrapositive/converse/inverse, "you have" interpretation, "all but N" phrasing)
- Add `internal/rag` package to manage chunking, embedding, and JSONL storage.
- Define a JSONL index format at `rag_corpus_accuracy_v1/index.jsonl` storing `chunk_id`, `doc`, `offset`, `text`, `embedding`.
- Implement `agon rag index` to build or refresh the index from the corpus using the configured embedding model and host.
- Document the ingestion flow: discover files, read text, chunk, embed, write JSONL records.

Verify
```powershell
agon rag index --config config/config.example.RAGAccuracy.json
Get-ChildItem rag_corpus_accuracy_v1
rg -n "\"chunk_id\"|\"embedding\"" rag_corpus_accuracy_v1
```

## Section 3: Retrieval and context assembly

Goal: implement query-time retrieval and deterministic context formatting for RAG mode (accuracy prompts).

Changes
- Add `internal/rag/retriever.go` to load the index, embed a query, and compute cosine similarity for `topK`.
- Add `internal/rag/formatter.go` to assemble a context block with stable ordering and source labels like `[doc:filename]`.
- Track and return RAG telemetry (`retrieval_ms`, `context_tokens`, `source_coverage`) for downstream metrics.
- Add focused unit tests for chunking, similarity ranking, and context formatting.
Documentation details to include
- Retrieval flow steps (query -> embed -> topK -> context format -> prompt injection).
- Explicit note that retrieval reads from `rag_corpus_accuracy_v1/index.jsonl` only (no external DB).

Default retrieval settings (keep stable for benchmarks)
- Chunk size: ~500 tokens
- Chunk overlap: ~50 tokens
- `top_k`: 4-6
- Similarity: cosine
- Rerank: off
- Context budget: cap at ~1,200 tokens

Context injection format
```
CONTEXT
[doc:facts_and_constants.md] ...excerpt...
[doc:units_and_percent.md] ...excerpt...
[doc:logic_and_traps.md] ...excerpt...
```

Verify
```powershell
go test ./internal/rag
rg -n "CONTEXT|doc:" internal/rag
```

## Section 4: Accuracy runner integration (rag_mode compare)

Goal: run accuracy prompts with RAG off/on while keeping everything else identical, and use accuracy-focused corpus docs.

Changes
- Update `accuracy/accuracy.go` to run each prompt twice when `ragMode` is `true`: once without context and once with retrieved context (the RAG-off pass is a control run).
- Keep `accuracy/accuracy_prompts.json` as the prompt source; do not change question content.
- Extend prompt suite parsing to support two system prompts (`system_prompt_off`, `system_prompt_on`) or derive the RAG ON prompt at runtime. The RAG ON prompt should add: "If CONTEXT is provided, treat it as authoritative reference material."
- Implement a new scoring mode for RAG prompts (checklist or required-fields scoring) to avoid breaking existing numeric accuracy scoring.
- Record `rag_mode` and RAG telemetry in `accuracy/results/*.jsonl`.
Documentation details to include
- The exact system prompts and the required output constraints for RAG tests (integer-only output).
- A checklist schema example for scoring (what counts as correct), or confirm numeric scoring remains unchanged.

Verify
```powershell
agon accuracy --config config/config.example.RAGAccuracy.json
rg -n "\"rag_mode\"" accuracy/results
```

## Prompt set and system prompts (preserve exact intent)

Accuracy tests should continue to use `accuracy/accuracy_prompts.json` unchanged. This suite intentionally mixes knowledge retrieval, computation, and language traps. RAG is expected to lift the knowledge items most.

System prompt (RAG OFF, existing)
```
You are a precise facts and logic engine. Output ONLY the final numerical answer as a single integer. No text, no explanation, no units, no commas.
```

System prompt (RAG ON, minimal change)
```
You are a precise facts and logic engine. Output ONLY the final numerical answer as a single integer. No text, no explanation, no units, no commas.
If CONTEXT is provided, treat it as authoritative reference material.
```

Expected impact by prompt category
- Strong gains on knowledge retrieval questions: 1, 2, 5, 8, 15, 16, 17, 18, 19, 20.
- Small or no gains on arithmetic/pattern/state tracking: 3, 4, 6, 7, 13, 14, 21, 23, 24, 25.
- Moderate gains on language/logic traps if `logic_and_traps.md` is included: 9, 12, 22.

Optional: add a second accuracy pack for a stronger signal
- `accuracyMode_retrieval_heavy.json` with 80-90% factual prompts that map to the corpus.

## Section 5: Metrics analysis and reporting

Goal: show RAG off/on comparison in the metrics report with retrieval overhead.

Changes
- Update `internal/cli/analyze_metrics.go` to read RAG fields from accuracy JSONL and pass them through to the analysis layer.
- Extend `internal/metrics/analyze.go` data model and charts to split accuracy and latency by `rag_mode`.
- Add UI annotations for `context_tokens` and `retrieval_ms` in the report.

Verify
```powershell
agon analyze metrics --accuracy-results accuracy/results
rg -n "rag|retrieval_ms|context_tokens" reports/metrics-report.html
```

## Reporting notes (accuracy story)

- Add a new axis `rag_mode` to accuracy results (off vs on) for every prompt.
- Show Accuracy by Category with split bars (RAG off vs on).
- Add an Accuracy Delta per model chart (rag_on minus rag_off).
- Always annotate the tradeoff with `context_tokens`, `retrieval_ms`, and `top_k`.

## Section 6: Documentation and guardrails

Goal: capture the new workflow and keep results repeatable.

Changes
- Update `README.md` to describe RAG mode, corpus location, ingestion/indexing steps, retrieval steps, and example commands.
- Add a small "RAG benchmark pack" doc in `rag_corpus_accuracy_v1/README.md` explaining how to keep the corpus stable and focused on accuracy prompts.
- Add a note to config examples about using the same model/prompt set for RAG off/on comparisons.
Documentation details to include
- JSONL-only install path (default) and a brief note that DB-backed stores are future options.
- A minimal setup path for repeatable benchmarks.

Verify
```powershell
rg -n "RAG|rag_corpus|rag mode" README.md rag_corpus_accuracy_v1
```

## Notes and open questions to resolve early

- Embedding host selection: use a dedicated host (recommended) or reuse the current model host?
- Token counting strategy for `context_tokens`: use a simple heuristic (word count) or a llama.cpp tokenization call?
- Index persistence: store embeddings in JSONL under `rag_corpus_accuracy_v1/` (default) or in a separate cache directory?
- Scoring for RAG prompts: exact string checks, required keys in JSON, or a small checklist language?
