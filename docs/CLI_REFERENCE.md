## CLI Commands

### `agon chat`

Starts the main interactive chat UI. The UI mode is determined by the configuration file or command-line flags.

*   **Flags**:
    *   `--config, -c`: Path to a custom config file.
    *   `--multimodelMode`: Override config to start in Multimodel mode.
    *   `--pipelineMode`: Override config to start in Pipeline mode.
    *   `--debug`, `--jsonMode`, `--mcpMode`, etc.

*   **Examples**:
    *   Start a chat session with the default configuration:
        ```bash
        agon chat
        ```
    *   Start a chat session in Multimodel mode:
        ```bash
        agon chat --multimodelMode
        ```
    *   Start a chat session with a custom configuration file:
        ```bash
        agon chat --config /path/to/your/config.json
        ```

### `agon list`

*   **`agon list models`**: Lists all models specified in the config for each host and indicates if they are available on the host machine.
*   **`agon list commands`**: Lists all available commands.

### `agon pull`

*   **`agon pull models`**: Pulls any models from your config that are missing on the respective llama.cpp hosts (router mode required).

**Example**
```bash
agon pull models --config ../configs/config.example.LlamaCpp.json
```

### `agon delete`

*   **`agon delete models`**: Deletes specified models from their llama.cpp hosts (router mode required).

**Example**
```bash
agon delete models --config ../configs/config.example.LlamaCpp.json
```

### `agon sync`

*   **`agon sync models`**: Synchronizes each llama.cpp host to have exactly the models listed in the config (router mode required).

**Example**
```bash
agon sync models --config ../configs/config.example.LlamaCpp.json
```

*   **`agon sync configs`**: Synchronizes per-host config files from the main config.

**Example**
```bash
agon sync configs --config ../configs/config.json
```

### `agon unload`

*   **`agon unload models`**: Unloads models from memory on their hosts to free up resources.

**Example**
```bash
agon unload models --config ../configs/config.example.LlamaCpp.json
```

### `agon help`

*   **`agon help`**: Shows help for any command.

### `agon show`

*   **`agon show config`**: Displays the current, fully resolved configuration.

**Example**
```bash
agon show config --config ../configs/config.example.LlamaCpp.json
```

### `agon analyze`

*   **`agon analyze metrics`**: Generates metric analysis and an HTML report from benchmark outputs (includes the parameter template name from accuracy results in the report filename).

**Example**
```bash
agon analyze metrics --benchmarks-dir ../agonData/modelBenchmarks --metadata-dir ../agonData/modelMetadata
```

### `agon benchmark`

*   **`agon benchmark model`**: Runs a single benchmark against a benchmark server endpoint.
*   **`agon benchmark accuracy`**: Runs the accuracy batch workflow. Uses the prompt suite in `internal/accuracy/accuracy_prompts.json`, reads `../agonData/modelMetadata/` (use `agon fetch modelmetadata`), and appends results to `../agonData/modelAccuracy/`. Defaults to the `accuracy` parameter template unless overridden.

**Examples**
```bash
agon benchmark model --model llama-3-2-1b-instruct-q8_0.gguf --gpu radeon-rx-570 --benchmark-endpoint http://localhost:9999/benchmark
```

```bash
agon benchmark accuracy
```

```bash
agon benchmark accuracy --parameterTemplate fact_checker
```

### `agon fetch`

*   **`agon fetch modelmetadata`**: Fetches model metadata from configured hosts.

**Example**
```bash
agon fetch modelmetadata --endpoints http://localhost:8080,http://localhost:8081 --gpu radeon-rx-570
```

### `agon list`

*   **`agon list commands`**: Lists all available commands.

**Examples**
```bash
agon list commands
```


### `agon rag`

*   **`agon rag index`**: Builds the RAG JSONL index.
*   **`agon rag preview`**: Previews RAG retrieval and context assembly.

**Examples**
```bash
agon rag index --config ../configs/config.example.RAGAccuracy.json
```

```bash
agon rag preview "what is agon?" --config ../configs/config.example.RAGAccuracy.json
```
