## What “integration into Agon” looks like

### 1) Add a benchmark dimension: `rag_mode`

Every run becomes a tuple like:

* `(model, prompt_set, rag_mode, trial_id)`

Where:

* `rag_mode = off` → model sees only the system+user prompt
* `rag_mode = on` → Agon does retrieval, injects context, then calls the model

That gives you a simple chart expansion:

* Existing charts (tokens/sec, TTFT, total time, accuracy) become **split-bars** or **two lines** per model: `RAG off` vs `RAG on`.

### 2) Keep everything identical except context injection

To make the comparison meaningful:

* Same model
* Same user prompts
* Same generation settings (temperature, top_p, max_tokens)
* Same trials count
* Same stop conditions / output constraints

Only difference:

* `rag_off`: no retrieval context
* `rag_on`: retrieved chunks injected

### 3) Collect RAG-specific telemetry

In addition to your existing metrics, add:

* `retrieval_ms`
* `k` (top-K chunks)
* `context_tokens` (tokens added by retrieved chunks)
* `source_coverage` (optional: count of distinct documents used)
* `citations_used` (optional: did the answer include references to retrieved sources)

This is key because RAG can increase prompt tokens; you’ll want to show:

* **Accuracy gain**
* **Cost in prompt size**
* **Latency impact from retrieval**

---

## A repeatable “RAG benchmark pack”

The trick: you want a **stable document corpus** and a **stable prompt set** that reliably exposes the failure mode RAG fixes: *missing or specific domain knowledge*.

### Documents to add (make a small, curated corpus)

Create a folder like: `rag_corpus/` and keep it versioned.

Minimum recommended set:

1. `agon_readme.md`

* Your tool’s purpose, modes, flags, how to run benchmarks.

2. `report_summary.md`

* A condensed, human-written summary of your latest findings (models, conclusions, key tradeoffs).
* Important: keep this stable between runs unless you’re intentionally testing drift.

3. `metrics_definitions.md`

* Definitions: TTFT, tokens/sec, total time, accuracy method, difficulty tiers.

4. `model_notes.md`

* A small table you maintain: model families, expected strengths/weaknesses, CPU performance notes.

5. `faq_troubleshooting.md`

* Common runtime issues, weird behaviors, “what to do if…”.

Optional but powerful:

* `golden_answers.jsonl` (Q/A pairs with canonical answers used for scoring)

**Why these docs?**
They let RAG retrieve *exact* facts and consistent explanations, instead of the model “freestyling” explanations. This targets the accuracy/consistency drop you see in small models.

---

## System prompts (RAG OFF vs RAG ON)

Use two system prompts that are nearly identical, except one tells the model about context + how to use it.

### System prompt: RAG OFF

> You are an assistant answering questions about the Agon project and CPU-only LLM benchmarking.
> Be concise and accurate.
> If you do not know a fact, say you don’t know rather than guessing.
> When asked for steps, provide them as numbered commands or bullet points.

### System prompt: RAG ON

> You are an assistant answering questions about the Agon project and CPU-only LLM benchmarking.
> You will be given a **CONTEXT** section containing retrieved excerpts from the user’s documents.
> Use CONTEXT as the primary source of truth.
> If CONTEXT does not contain the answer, say you don’t know rather than guessing.
> When answering, cite the relevant context by referencing the provided source labels (e.g., [doc:filename]).
> Be concise and accurate.
> When asked for steps, provide them as numbered commands or bullet points.

**Why this matters:** it forces a measurable change: RAG-on answers should become **less hallucinatory** and more grounded—especially for fast/small models.

---

## User prompts you can run repeatedly (a good benchmark set)

You want prompts that:

* Are answerable from your corpus (so RAG can help)
* Tend to produce wrong or shallow answers without context (especially on small models)
* Have clear “correctness” criteria

Here’s a reusable set of 12 prompts (mix of factual + procedural + constrained output):

1. **Project identity**

* “In 2 sentences, what is Agon and what problem does it solve?”

2. **How to run a benchmark**

* “Give the exact command sequence to run the standard benchmark suite and generate the report.”

3. **Explain metrics (grounding)**

* “Define TTFT, tokens/sec, and total latency as used in this project.”

4. **Interpretation (domain-specific)**

* “Explain why a model can be top in throughput but weak in accuracy, using how *this project* measures both.”

5. **Configuration detail**

* “Where is the report output written, and how do I change it?”

6. **Reproducibility**

* “List 5 steps to make results comparable across runs on CPU-only hardware.”

7. **Model routing recommendation**

* “Given the report’s tradeoffs, suggest a two-model pipeline: one fast draft model and one accurate model.”

8. **Failure handling**

* “What should I do if a run fails halfway through? Provide a recovery checklist.”

9. **JSON constrained task**

* “Return a JSON object with keys: {purpose, key_metrics, recommended_models}. Keep values short strings.”

10. **Doc lookup**

* “Which document explains how accuracy is computed? Answer with the filename and a 1-sentence summary.”

11. **Hard constraint**

* “Provide steps to add a RAG corpus and run a RAG-enabled query. Use only commands, no prose.”

12. **Small-model trap**

* “What is the single biggest limitation of tiny CPU-only models in this benchmark, and what mitigations does Agon support?”

These prompts are designed so:

* **RAG OFF**: small models will often be vague, wrong on file paths / naming, or invent details.
* **RAG ON**: answers should cite the exact docs and become consistent.

---

## How RAG retrieval should be done for the benchmark

To keep runs stable:

* Chunk size: ~500 tokens
* Overlap: ~50 tokens
* `top_k`: 4–6
* Similarity: cosine
* Optional rerank: off (keep it simple + repeatable)
* Context budget: cap it (e.g., max 1,200 tokens of retrieved text)

Agon should inject context like:

**CONTEXT**

* [doc:report_summary.md] …excerpt…
* [doc:metrics_definitions.md] …excerpt…
* [doc:faq_troubleshooting.md] …excerpt…

Then user question.

---

## Scoring “with RAG” vs “without RAG”

You’ll want at least two scores:

### A) Performance metrics (you already chart these)

* TTFT
* tokens/sec
* total time
* output tokens

RAG adds:

* retrieval_ms
* context_tokens

### B) Quality metrics (the point of RAG)

Pick one of these approaches:

**Option 1: Exact-match / checklist scoring (best for repeatability)**
For each prompt, define required elements (like a unit test):

* Must mention correct report path
* Must reference correct doc name
* Must output valid JSON

Score: `% requirements met`

**Option 2: LLM-as-judge (easier, less deterministic)**
Have a fixed “judge” model grade outputs vs a rubric.
It’s useful, but can add noise (and costs extra inference).

Given your benchmarking mindset, I’d do **Option 1** for the RAG comparison charts.

---

## Where you should see improvements based on your report’s pattern

RAG helps most where your report shows models struggle:

* **accuracy on harder / domain-specific questions**
* **consistency across runs**
* **reduced hallucination** (wrong claims, wrong file paths, invented features)

What you’ll likely see in charts:

### Accuracy chart

* **RAG ON > RAG OFF** for smaller models (biggest delta)
* More modest gain for your already-accurate models

### Latency chart

* Slight increase in:

  * prompt tokens
  * TTFT (sometimes)
  * total time (depending on context size)
* But retrieval itself should be small compared to CPU inference (especially if local)

### “Efficiency” framing

RAG is effectively a way to get:

* **accuracy gains without moving to a slower/bigger model**
  So the story in your follow-up article becomes:

> “RAG shifts the Pareto curve on CPU-only hardware.”

---

## A clean way to present it in your report UI

For each model, show:

* **Two bars**: Accuracy (RAG off / on)
* **Two bars**: Total time (RAG off / on)
* A small annotation: `+context_tokens`, `+retrieval_ms`

This makes the trade-off obvious and honest.