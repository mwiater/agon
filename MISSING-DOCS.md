# Missing Documentation

This file contains a list of features, configuration options, and internal packages that are not currently covered in the existing markdown documentation.

## Undocumented Commands and Flags

The following commands and flags are not documented or are only partially documented in the `docs/CLI_REFERENCE.md` file.

### Persistent Flags

These flags are available for all commands:

-   `--config, -c`: string, config file (e.g., configs/config.json)
-   `--debug`: bool, enable debug logging
-   `--multimodelMode`: bool, enable multi-model mode
-   `--pipelineMode`: bool, enable pipeline mode
-   `--jsonMode`: bool, enable JSON output mode
-   `--mcpMode`: bool, proxy LLM traffic through the MCP server
-   `--mcpBinary`: string, path to the MCP server binary (defaults per OS)
-   `--mcpInitTimeout`: int, seconds to wait for MCP startup (0 = default)
-   `--logFile`: string, path to the log file

### Command-Specific Flags

-   **`agon benchmark accuracy`**:
    -   `--parameterTemplate`: string, Parameter template to apply (accuracy|generic|fact_checker|creative)
-   **`agon benchmark model`**:
    -   `--model, -m`: string, model name to benchmark (required)
    -   `--gpu, -g`: string, GPU name for output filename (required)
    -   `--benchmark-endpoint, -b`: string, benchmark server endpoint URL (required)
-   **`agon fetch modelmetadata`**:
    -   `--endpoints`: []string, comma-separated list of base URLs to query (required)
    -   `--gpu`: string, GPU identifier used as a filename prefix (required)

### Command Discrepancies and Missing Information

-   **`agon run`**: This command group exists in the code but has no subcommands. The documentation in `docs/ACCURACY.md` incorrectly refers to `agon run accuracy` instead of `agon benchmark accuracy`.
-   **`agon sync configs`**: The command exists, but it's missing a description in the source code.

## Undocumented Configuration Options

The following configuration options are available in `config.json` but are not documented.

### Top-level Configuration

-   `RagMode`: (bool) Enable RAG mode.
-   `RagCorpusPath`: (string) Path to the RAG corpus.
-   `RagIndexPath`: (string) Path to the RAG index.
-   `RagEmbeddingModel`: (string) The model to use for generating embeddings.
-   `RagEmbeddingHost`: (string) The host to use for generating embeddings.
-   `RagChunkSizeTokens`: (int) The size of chunks in tokens for RAG.
-   `RagChunkOverlapTokens`: (int) The number of overlapping tokens between chunks for RAG.
-   `RagTopK`: (int) The number of top-K documents to retrieve for RAG.
-   `RagContextTokenLimit`: (int) The token limit for the context in RAG.
-   `RagSimilarity`: (string) The similarity metric to use for RAG.
-   `RagAllowedExtensions`: ([]string) A list of allowed file extensions for the RAG corpus.
-   `RagExcludeGlobs`: ([]string) A list of glob patterns to exclude from the RAG corpus.
-   `MCPBinary`: (string) Path to the `agon-mcp` server binary.
-   `MCPInitTimeout`: (int) Timeout in seconds for MCP server initialization.
-   `MCPRetryCount`: (int) The number of times to retry a failed MCP tool call.
-   `TimeoutSeconds`: (int) Timeout in seconds for API requests.
-   `LogFile`: (string) Path to the log file.
-   `BenchmarkCount`: (int) The number of iterations for benchmarks.
-   `Metrics`: (bool) Enable metrics collection.

### Host Configuration

-   `Name`: (string) A friendly name for the host.
-   `URL`: (string) The base URL of the API endpoint.
-   `Type`: (string) The type of host. Currently, only `llama.cpp` is supported.
-   `Models`: ([]string) A list of model identifiers to manage on this host.
-   `SystemPrompt`: (string) A custom system prompt to use for all interactions with this host.
-   `ParameterTemplate`: (string) The parameter template to use. Available templates are `generic`, `fact_checker`, `creative`, and `accuracy`.
-   `Parameters`: (object) A key-value map of `llama.cpp` parameters to override the template.

### Llama.cpp Parameters (`Parameters` object)

The `Parameters` object in the host configuration can contain any of the parameters supported by `llama.cpp`. These are not documented and the list is extensive. Refer to the `llama.cpp` documentation for a complete list.

### Parameter Templates

The following parameter templates are available:
-   `generic` (aliased as `generic_chat`, `chat`, `default`)
-   `fact_checker` (aliased as `fact`, `factchecker`, `fact-check`)
-   `creative` (aliased as `creative_writing`, `writer`)
-   `accuracy` (aliased as `acc`)

## Providers

### `ollama` Provider
The `internal/providers/ollama/` directory exists but is empty, so there is no `ollama` provider implemented yet.

### `llamacpp` Provider
-   **Router Mode Dependency:** This provider has features that depend on the `llama.cpp` server running in "router mode".
-   **Automatic Request Fields:** It automatically adds `response_fields` and `timings_per_token` to requests to get detailed metrics.
-   **Tool Handling:** It has specific logic for handling tool calls.

### `mcp` Provider
-   **Proxy:** This provider acts as a proxy to a fallback provider (`llamacpp`).
-   **Tool Execution:** It communicates with an `agon-mcp` server process to execute tools.
-   **Advanced Features:** It has a sophisticated retry and "fix with LLM" mechanism for tool calls, which is not documented.
-   **Tool Discovery:** It discovers tools at runtime by calling the `tools/list` RPC method on the `agon-mcp` server.

## MCP Tools

The following tools are available via the `agon-mcp` server:

-   `current_time`: Takes no arguments and returns the current time.
-   `current_weather`: Takes a `location` string as an argument and returns the weather for that location. It has external dependencies on OpenStreetMap and Open-Meteo.
-   `available_tools`: Takes no arguments and returns a list of available tools.

## Internal Packages

This is a high-level overview of the purpose of the internal packages:

-   `internal/accuracy`: Contains the logic for the accuracy testing workflow.
-   `internal/appconfig`: Manages loading and interpreting application configuration.
-   `internal/benchmark`: Contains the logic for running benchmarks against the models.
-   `internal/chat`: Contains the core logic for running the chat functionality.
-   `internal/commands`: Contains the implementation of all the CLI commands.
-   `internal/logging`: Provides logging functionality for the application.
-   `internal/metrics`: Contains the logic for aggregating, analyzing, and reporting on model performance metrics.
-   `internal/models`: Contains the logic for managing models, including fetching metadata and handling model parameters.
-   `internal/providerfactory`: Responsible for creating instances of different providers.
-   `internal/providers`: Defines the interfaces for interacting with different AI model providers.
-   `internal/rag`: Contains the implementation of the Retrieval-Augmented Generation (RAG) functionality.
-   `internal/reports`: Related to generating HTML reports for metrics.
-   `internal/tui`: Responsible for rendering the terminal user interface.
-   `internal/util`: Contains utility functions used throughout the application.
