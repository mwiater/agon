# agon

![agon](.screens/agon_llamacpp.png)

[![Go Reference](https://pkg.go.dev/badge/github.com/mwiater/agon@v0.2.1.svg)](https://pkg.go.dev/github.com/mwiater/agon@v0.2.1)

`agon` is a terminal-first companion for interacting with large language models that expose a llama.cpp-compatible API. It's designed for quickly comparing small LLMs, and running them in various modes like multi-model chat, pipeline, and more.

## Getting Started

### 1. Installation

You'll need GoReleaser to build the binaries. You can find the installation instructions here: https://goreleaser.com/install/

Once you have GoReleaser, you can build the `agon` and `agon-mcp` binaries:

```bash
goreleaser release --snapshot --clean --skip=publish
```

The binaries will be in the `dist/` directory.

### 2. Configuration

`agon` needs a configuration file to know about your llama.cpp hosts. Create a `configs/config.json` file with the following content:

```json
{
  "hosts": [
    {
      "name": "LlamaCpp01",
      "url": "http://localhost:8080",
      "type": "llama.cpp",
      "models": [
        "qwen3:1.7b",
        "llama3.2:1b"
      ],
      "systemprompt": "You are a helpful and concise assistant.",
      "parameterTemplate": "generic"
    }
  ]
}
```

Make sure to replace the `url` and `models` with your own llama.cpp server details.

### 3. Run a Chat

#### Single Model Chat

To start a chat with a single model, run:

```bash
agon chat
```

`agon` will prompt you to select a host and a model from your configuration file.

#### Multi-Model Chat

To compare models side-by-side, use the `--multimodelMode` flag:

```bash
agon chat --multimodelMode
```

This will start a chat where your prompt is sent to multiple models at once, and you can see their responses in parallel.

## Features

*   **Multi-Host Management**: Connect to any number of llama.cpp hosts.
*   **Interactive Chat**: A terminal-based UI for single-model, multi-model, and pipeline chats.
*   **Multimodel Chat Mode**: Compare up to four models side-by-side.
*   **Pipeline Mode**: Chain models together, where the output of one is the input for the next.
*   **Accuracy Batch**: Run a prompt suite to test model accuracy.
*   **MCPMode**: Enable tool usage by proxying requests through a local `agon-mcp` server.
*   **Model Management**: A suite of commands to `list`, `unload`, `pull`, `delete`, and `sync` models.

## Dive Deeper

For more detailed information on advanced features, please refer to the documentation in the `docs` directory:

*   [**Operating Modes**](docs/OPERATING_MODES.md): Detailed descriptions of all operating modes.
*   [**CLI Command Reference**](docs/CLI_REFERENCE.md): A complete reference for all CLI commands.
*   [**Benchmarking**](docs/BENCHMARKING.md): How to run benchmarks.
*   [**Accuracy**](docs/ACCURACY.md): How to run accuracy tests.
*   [**Services**](docs/SERVICES.md): How to set up `llama.cpp` as a service.
*   [**Testing**](docs/TESTS.md): Information on the project's test suite.

## License

This project is distributed under the [MIT License](LICENSE).
