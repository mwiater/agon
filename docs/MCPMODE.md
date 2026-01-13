# MCP Mode

## Setup

1.  Install the `agon-mcp` binary and make sure it is available in your system's `PATH`.
2.  Enable MCP mode in your `config.json` file:

    ```json
    {
      "mcpMode": true
    }
    ```

## Example Configuration

Start from the provided example file and edit hosts/models as needed:

```bash
cp configs/config.example.MCPMode.json configs/config.mcp.json
```

Open `configs/config.mcp.json` and update host URLs, models, and any parameters you want to test.

## Example Run

```bash
agon chat --config configs/config.mcp.json
```

## Example Prompt

Once the chat is running, try a tool-friendly prompt such as:

```
What is the current time and weather in Boston, MA?
```

## Current Available MCP Tools for Testing

*   `current_time`: Returns the current time.
*   `current_weather`: Returns the current weather for a given location.
