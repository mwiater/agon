agon MCP Server (Go, experimental)

This directory contains a tiny, dependency‑free Model Context Protocol (MCP) server implemented in Go. It communicates over stdio using JSON‑RPC 2.0 with Content‑Length framing (similar to LSP). It exposes three simple tools:

- echo: Returns the provided text unchanged.
- word_count: Counts words in the provided text.
- sentiment: Naive sentiment analysis (positive/negative/neutral) based on small keyword lists.

Files
- mcp/main.go: Stdio MCP server implementation in Go.

Build/Run
- Prerequisite: Go toolchain
- Run directly: `go run ./mcp`
- Or build: `go build -o dist/agon-mcp ./mcp` then execute the binary

Protocol Notes
- Transport: stdio with `Content-Length: <bytes>\r\n\r\n<json>` frames.
- Supported methods:
  - initialize: Returns serverInfo and capabilities (tools list/call).
  - tools/list: Returns tool definitions with JSON Schemas.
  - tools/call: Executes a tool and returns `content` as an array of text parts.
  - ping: Returns empty result (handy for health checks).

Tools
1) echo
   - name: "echo"
   - input_schema:
     {
       "type": "object",
       "properties": {"text": {"type": "string"}},
       "required": ["text"]
     }
   - returns: same text

2) word_count
   - name: "word_count"
   - input_schema:
     {
       "type": "object",
       "properties": {"text": {"type": "string"}},
       "required": ["text"]
     }
   - returns: integer count as text (e.g., "5")

3) sentiment
   - name: "sentiment"
   - input_schema:
     {
       "type": "object",
       "properties": {"text": {"type": "string"}},
       "required": ["text"]
     }
   - returns: label and score in plain text (e.g., "positive (score=0.75)")

Example Frames
Request:
  Content-Length: 88\r\n\r\n
  {"jsonrpc":"2.0","id":1,"method":"initialize","params":{"clientInfo":{"name":"test"}}}

Response:
  Content-Length: 166\r\n\r\n
  {"jsonrpc":"2.0","id":1,"result":{"serverInfo":{"name":"agon-mcp","version":"0.1.0"},"capabilities":{"tools":{"list":true,"call":true}}}}

Notes
- This server aims to be simple and illustrative, not a full MCP reference.
- Extend by adding tools in `mcp/main.go` and updating the list returned by tools/list.
4) current_weather
   - name: "current_weather"
   - input_schema:
     {
       "type": "object",
       "properties": {"location": {"type": "string"}},
       "required": ["location"]
     }
   - returns: two content parts
     - type: "json" — raw current conditions as JSON
     - type: "interpret" — a server-provided prompt instructing the LLM to produce a natural-language summary

   The MCP provider in agon detects this pair and performs a non‑streaming follow‑up chat with the LLM: it supplies the JSON and the prompt, disables tools for that round, and renders the LLM’s natural‑language result to the console. The raw JSON is not printed directly.

Debug Logging
- The server logs failures and errors during geocoding/weather fetch.
- The MCP provider logs when it sends the JSON+prompt to the LLM and when it receives the interpretation, including sizes and timing.
