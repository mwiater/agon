# Accuracy

## 1) Prerequisites: BENCHMARKING.md

Make sure you have generated model metadata as described in [BENCHMARKING.md](BENCHMARKING.md). The accuracy workflow requires metadata in `agonData/modelMetadata/`.

## 2) "run accuracy"

Run the accuracy batch workflow. It uses the prompt suite in `internal/accuracy/accuracy_prompts.json` and appends results to `agonData/modelAccuracy/`. It defaults to the `generic` parameter template unless overridden. Make sure your config points at the hosts/models you want evaluated.

**Examples**
```bash
agon run accuracy
```

```bash
agon run accuracy --parameterTemplate fact_checker
```

After the run completes, review the appended results in `agonData/modelAccuracy/`.
