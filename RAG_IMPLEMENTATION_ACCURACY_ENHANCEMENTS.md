# How to integrate a RAG on/off comparison into Agon (for your exact prompts)

## Add a new axis: `rag_mode`

Every accuracy test run records:

* `rag_mode=off` → ask the model exactly as today
* `rag_mode=on` → retrieve context, inject it, then ask the model

Same model, same tests, same decoding settings.

Also record RAG-specific telemetry:

* `retrieval_ms`
* `context_tokens`
* `top_k`

Then your report can show:

* Accuracy %: RAG off vs on (per model)
* Latency cost: total time / TTFT deltas
* Breakdown by category (this is where the story will be strongest)

---

# What documents should you add for this accuracy suite?

Create a small corpus folder, version-controlled:

`rag_corpus_accuracy_v1/`

### 1) A “facts + constants” document (high ROI)

`facts_and_constants.md` containing short reference entries (not Q/A style), like:

* Atomic numbers (at least Helium)
* Common biology facts (spider legs)
* Geometry facts (triangle angles, cube faces)
* Calendar facts (September days)
* Human body facts (adult bones)
* Computing (bits/bytes)
* US states
* Physical constants (speed of light)
* Planet facts (Jupiter equatorial radius)

This alone will lift accuracy on:
**1,2,5,8,15,16,17,18,19,20** (and maybe more depending on model weakness).

### 2) A “units & percent” reference

`units_and_percent.md`

* meters↔cm
* percent definition and examples

Helps:
**10,11** (again: depends on model)

### 3) A “logic & language traps” primer (optional)

`logic_and_traps.md`
Written as *rules*, not answer key:

* **Contrapositive / converse / inverse** basics

  * e.g., “All healthy plants are green” does **not** mean “all green plants are healthy”
  * and “not green ⇒ unhealthy” is actually supported by the original statement (because it’s `healthy ⇒ green`, contrapositive is `not green ⇒ not healthy`)
* **Semantic trap pattern** like “If you take away 2 from 3, how many do YOU have?” → interpret “you have” as “you took”
* “all but N” phrasing (“sells all but 4” means 4 remain)

This can help:
**9,12,22** (and reduces random failure variance)

### 4) Keep it small

For benchmark stability, keep the corpus **tight** (a few pages total). You want retrieval to be deterministic-ish and fast.

---

# How RAG would work with your “ONLY output an integer” system prompt

Your system prompt is strict:

> “Output ONLY the final numerical answer as a single integer. No text…”

That’s fine. The RAG context just becomes silent scaffolding.

### RAG-On system prompt variant (minimal change)

Keep your existing system prompt, but add one line:

> “If CONTEXT is provided, treat it as authoritative reference material.”

So you avoid changing behavior beyond “use context”.

---

# Example run (story, using your exact test #16)

### User wants to run accuracyMode and show RAG improves accuracy

They run:

* `agon accuracy --model gemma3:270m --rag=off`
* `agon accuracy --model gemma3:270m --rag=on --rag-corpus rag_corpus_accuracy_v1`

### Test prompt

> “What is the equatorial radius of Jupiter in kilometers?”

Expected: `71492` with margin ±100.

### What happens

#### RAG OFF (small model behavior)

A small model might:

* guess a rough radius
* confuse with Earth or mean radius
* output something close but wrong (or totally wrong)

#### RAG retrieval (RAG ON)

Agon:

1. embeds the query
2. retrieves top chunks, e.g. from `facts_and_constants.md`:

   * “Jupiter equatorial radius: 71,492 km …”
3. injects it into the prompt as CONTEXT

#### Model output (still only integer)

`71492`

✅ Correct.

---

# Where you’ll see improvements in your charts (with this exact suite)

## 1) Biggest lift will show in “retrieval/constant” categories

These tests are *pure knowledge lookup*:

* 1 Helium atomic #
* 2 spider legs
* 5 triangle angles sum
* 8 cube faces
* 15 September days
* 16 Jupiter equatorial radius (with margin)
* 17 adult human bones
* 18 speed of light (with margin)
* 19 bits in bytes
* 20 US states

For smaller/faster models (the ones your report shows as throughput leaders but accuracy laggards), these are exactly where they:

* hallucinate
* substitute near facts
* output plausible nonsense

RAG anchors them.

**So your “accuracy %” bar for those models should move upward most.**

## 2) Little-to-no lift on arithmetic/pattern/state tracking

These don’t suffer from missing knowledge; they suffer from computation/attention:

* 3 144/4
* 4 doubling pattern
* 6 variable logic
* 7 clock degrees
* 13 square of 13
* 14 rectangle area
* 21 max of three ints
* 23 tank subtraction over hours
* 24 string letter → alphabet index → ×2
* 25 odd-one-out perfect squares

RAG can include “how to compute” rules, but that typically doesn’t move the needle much, because the model still has to *perform* the computation perfectly under your strict “integer-only” constraint.

**So your charts will likely show minimal deltas here.** That’s okay — it makes the RAG story honest.

## 3) Moderate lift on traps if you include `logic_and_traps.md`

These are where models misread English or logic:

* 9 apples “YOU have”
* 12 sells all but 4
* 22 conditional/contrapositive logic

RAG can reduce “gotcha” failures by grounding the interpretation rule.

---

# How to present this in the report so it’s convincing

### Add a new chart: Accuracy by Category (RAG off vs on)

This is the money shot.

Because overall accuracy can be diluted by arithmetic tasks, you want to show:

* Retrieval categories jump
* Arithmetic categories unchanged
* Traps improve if you include trap docs

### Add an “Accuracy Delta” chart per model

For each model:

* `Δ accuracy = rag_on - rag_off`
  This will highlight that smaller throughput-first models benefit most.

### Include RAG cost metrics next to it

* +context_tokens
* +retrieval_ms
  So readers see the trade-off: “small latency cost, accuracy gain”.

---

# One important recommendation

If your goal is to prove “RAG improves accuracy,” your current suite is *okay*, but it’s **mixed** (knowledge + computation). You’ll get a cleaner, bigger signal if you add a **second accuracy pack**:

### `accuracyMode_retrieval_heavy.json`

* 80–90% facts/constants
* includes some obscure facts the base models are less likely to know
* still numeric-only answers

That will show a dramatic RAG delta on CPU-only.
.
