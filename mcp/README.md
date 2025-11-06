# agon MCP Server

This directory contains a tiny, dependency‑free Model Context Protocol (MCP) server implemented in Go. It communicates over stdio using JSON‑RPC 2.0 with Content‑Length framing (similar to LSP).

## Tools Overview

The MCP server exposes the following tools:

*   **`current_weather`**: Fetches current weather conditions for a given location.
*   **`current_time`**: Retrieves the current local time and timezone.
*   **`available_tools`**: List the available tools on the MCP server.

## Files

*   `mcp/main.go`: Stdio MCP server implementation in Go.
*   `mcp/tools/`: Contains the implementations for individual tools.

## Build and Run

### Prerequisites

*   Go toolchain installed.

### Building

To build the `agon-mcp` binary, you can use `goreleaser` (which also builds `agon`):

```bash
goreleaser release --snapshot --clean --skip=publish
```

Alternatively, you can build it manually:

```bash
go build -o dist/agon-mcp ./mcp
```

### Running

The `agon-mcp` server is typically started and managed by the `agon` CLI when `mcpMode` is enabled.

## Protocol Notes

The MCP server communicates using JSON-RPC 2.0 over standard I/O with Content-Length framing.

### Transport

*   `stdio` with ``Content-Length: <bytes>\r\n\r\n<json>`` frames.

### Supported Methods

*   **`initialize`**: Returns server information and capabilities (tools list/call).
*   **`ping`**: Returns an empty result (useful for health checks).
*   **`tools/list`**: Returns tool definitions, including their JSON Schemas.
*   **`tools/call`**: Executes a specified tool with provided arguments and returns an array of content parts.

## Tool Details

### `current_weather`

*   **Name**: `current_weather`
*   **Description**: Provides weather conditions for a *specific geographical location*. Use this tool for queries about temperature, precipitation, wind, or forecasts. **Do not use this tool for queries about the current time.**
*   **Input Schema**:
    ```json
    {
      "type": "object",
      "properties": {"location": {"type": "string", "description": "The city and state (e.g., 'Portland, OR') or city and country (e.g., 'London, UK'). You MUST provide a location. If the user only gives a city, you MUST ask for the state or country to avoid ambiguity."}},
      "required": ["location"]
    }
    ```
*   **Returns**: Two content parts:
    *   `type: "json"`: Raw current conditions as JSON.
    *   `type: "interpret"`: A server-provided prompt instructing the LLM to produce a natural-language summary.

    *Note*: The MCP provider in `agon` detects this pair and performs a non‑streaming follow‑up chat with the LLM: it supplies the JSON and the prompt, disables tools for that round, and renders the LLM’s natural‑language result to the console. The raw JSON is not printed directly.

### `current_time`

*   **Name**: `current_time`
*   **Description**: Use this tool *only* for queries about the current time, such as 'What time is it?' or 'What is the current time?'. **This tool cannot provide any weather information.**
*   **Input Schema**:
    ```json
    {
      "type": "object",
      "properties": {},
      "required": []
    }
    ```
*   **Returns**: Two content parts:
    *   `type: "json"`: Raw current time data as JSON.
    *   `type: "interpret"`: A server-provided prompt instructing the LLM to produce a natural-language summary.

## Example Frames

### Request Example (`initialize`)

```
Content-Length: 88

{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"clientInfo":{"name":"test"}}}
```

### Response Example (`initialize`)

```
Content-Length: 166

{"jsonrpc":"2.0","id":1,"result":{"serverInfo":{"name":"agon-mcp","version":"0.1.0"},"capabilities":{"tools":{"list":true,"call":true}}}}
```

## Notes

*   This server aims to be simple and illustrative, not a full MCP reference implementation.
*   You can extend its functionality by adding new tools in `mcp/main.go` and updating the list returned by `tools/list`.

## Debug Logging

When `debug` mode is enabled in the main `agon` CLI configuration, the `agon-mcp` server logs its activity to `agon-mcp-server.log`. This includes:

*   Failures and errors during external API calls (e.g., geocoding/weather fetch).
*   MCP provider logs when it sends JSON+prompt to the LLM for interpretation and when it receives the interpretation, including sizes and timing.
*   Context cancellation events are also logged when a request is interrupted, providing insight into graceful shutdowns.