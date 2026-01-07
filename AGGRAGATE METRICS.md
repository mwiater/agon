## 1. Metric Category: Model Quality & Logic

These metrics evaluate the "intelligence" of the  quantized model.

| Metric | Calculation Method | Utility |
| --- | --- | --- |
| **Global Accuracy Rate** | Count `correct: true` / total entries in `.jsonl`.

 | Baseline quality score for the model on this hardware.

 |
| **Difficulty vs. Error Correlation** | Plot `difficulty` (x-axis) against `% Correct` (y-axis).

 | Identifies the "breaking point" where logic fails as complexity increases.

 |
| **Confidence Calibration** | Compare `logprob` of correct vs. incorrect responses.

 | Determines if the model is "confidently wrong" or "hesitantly correct".

 |
| **Constraint Adherence** | Check if `output_tokens` matches instructions like "Output only the final number".

 | Measures how well the model follows system prompts and formatting rules.

 |
| **Mean Margin of Error** | Average of ( `expectedAnswer` - `response` ) for failed math prompts.

 | Quantifies how "close" the model gets to the right answer in numerical tasks.

 |

---

## 2. Metric Category: Inference Performance

These metrics measure the speed and responsiveness of the system.

* 
**Average Time to First Token (TTFT):** The average of `time_to_first_token` across all samples in the `.jsonl` file. This represents the latency a user experiences before text starts appearing.


* 
**Prompt Processing Throughput:** Extracted from the `gguf.json` benchmark where `n_prompt` is 512. This is calculated as `n_prompt` / `avg_ns` (converted to seconds), representing how fast the CPU ingests context.


* 
**Generation Speed (Tokens/Sec):** The `avg_ts` value from the `gguf.json` where `n_gen` is 128. This measures the sustained speed of the Pentium N3710 during text creation.


* 
**Inference Jitter (Stability):** The `stddev_ts` (standard deviation) from the `gguf.json`. High jitter indicates that the CPU is struggling with thermal throttling or background process interference.


* 
**Token per Second Decay:** Correlate `tokens_per_second` with `total_duration_ms` in the `.jsonl` to see if speed drops during longer generation tasks.



---

## 3. Metric Category: Hardware Efficiency

These metrics correlate the model's footprint with the physical capabilities of the Intel Pentium N3710.

* 
**Memory Efficiency Ratio:** Compare the `model_size` (approx. 2.31 GB) to the system's available RAM. This evaluates the effectiveness of `use_mmap` on low-memory hardware.


* 
**Thread Scaling Factor:** Compare performance using `n_threads: 8` against the physical core count of the N3710 (4 cores). This identifies if hyper-threading is providing a benefit or causing overhead.


* 
**Parameter Density Performance:** Calculate `avg_ts` / `model_n_params`. This allows you to compare the efficiency of this 3.2B parameter model against larger or smaller models on the same CPU.


* 
**Context Window Saturation:** Compare `input_tokens` in the `.jsonl` against the `n_ctx` (32,768) to see if performance degrades as the context window fills.



---

## 4. Implementation Workflow

### Data Extraction

1. 
**Metadata Parser:** Load `pentium-n3710-1-60ghz_Falcon3-3B-Instruct-q5_k_m.json` to get the base configuration (sampling settings, `n_ctx`, `temperature`).


2. 
**Benchmark Aggregator:** Load `pentium-n3710-1-60ghz_falcon3-3b-instruct-q5_k-gguf.json` to extract raw hardware throughput (`avg_ts`, `avg_ns`, `stddev_ts`).


3. 
**Accuracy Processor:** Parse `pentium-n3710-1-60ghz_Falcon3-3B-Instruct-q5_k_m.jsonl` to calculate correctness percentages and per-token log probabilities.



### Visualization Strategy

* 
**Latency Histogram:** Map `time_to_first_token` to see the distribution of responsiveness.


* 
**Token Probabilities Chart:** Use the `top_logprobs` list to visualize the model's "internal debate" for incorrect answers. For example, in Prompt ID 22, checking why it considered 1 vs 0.