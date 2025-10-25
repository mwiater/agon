Successful request, with one retry:


```
debug 2025/10/25 09:45:38 Tools: false
debug 2025/10/25 09:46:16 Tools: {current_weather, current_time}
debug 2025/10/25 09:46:56 Tool request: current_weather {"__mcp_attempt":0,"__user_prompt":"What is the weather in Denver, CO?","q":"Denver,CO"}
debug 2025/10/25 09:46:56 Tools: {current_weather, current_time}
debug 2025/10/25 09:47:39 Tool request: current_weather {"__user_prompt":"What is the weather in Denver, CO?","location":"Denver, CO"}
debug 2025/10/25 09:47:41 Tools: false
debug 2025/10/25 09:48:25 Tool result: current_weather The current temperature in Denver, Colorado is around 52 degrees Celsius, with a mix of sunny and partly cloudy skies, making it an ideal day to enjoy the outdoors. However, it's also not too late for a hike or a walk outside, with light winds and plenty of sunshine expected today.


[2025-10-25T09:45:35-07:00] MCP server started: binary=dist/agon-mcp_linux_amd64_v1/agon-mcp pid=66075
[2025-10-25T09:45:35-07:00] Available MCP tools: current_weather, current_time
[2025-10-25T09:45:35-07:00] MCP provider ready: using local server
[2025-10-25T09:45:36-07:00] Tool invoked: tool=loaded_models host=Ollama01
[2025-10-25T09:45:36-07:00] Tool bypassed: tool=loaded_models host=Ollama01 reason=delegated to Ollama API
[2025-10-25T09:45:38-07:00] Tool invoked: tool=ensure_model host=Ollama01 model=llama3.2:1b
[2025-10-25T09:45:59-07:00] Tool bypassed: tool=ensure_model host=Ollama01 model=llama3.2:1b reason=delegated to Ollama API
[2025-10-25T09:46:16-07:00] Tool invoked: tool=chat host=Ollama01 model=llama3.2:1b messages=1
[2025-10-25T09:46:16-07:00] Forwarding request: host=Ollama01 model=llama3.2:1b messages=1 tools=2
[2025-10-25T09:46:56-07:00] Tool requested: tool=current_weather args={"__mcp_attempt":0,"__user_prompt":"What is the weather in Denver, CO?","q":"Denver,CO"}
[2025-10-25T09:46:56-07:00] MCP tool detail: tool=current_weather attempt 0/1 failed: Error: 'location' argument is required.
[2025-10-25T09:46:56-07:00] MCP->LLM fix send: tool=current_weather host=Ollama01 model=llama3.2:1b
[2025-10-25T09:47:39-07:00] Tool requested: tool=current_weather args={"__user_prompt":"What is the weather in Denver, CO?","location":"Denver, CO"}
[2025-10-25T09:47:41-07:00] MCP->LLM interpret send: tool=current_weather host=Ollama01 model=llama3.2:1b bytes_json=178
[2025-10-25T09:48:25-07:00] MCP->LLM interpret recv: tool=current_weather host=Ollama01 model=llama3.2:1b chars=282 dur=44.818155018s
[2025-10-25T09:48:25-07:00] Tool executed: tool=current_weather host=Ollama01 model=llama3.2:1b output=The current temperature in Denver, Colorado is around 52 degrees Celsius, with a mix of sunny and partly cloudy skies, making it an ideal day to enjoy the outdo…
[2025-10-25T09:48:25-07:00] MCP->LLM fix recv: tool=current_weather host=Ollama01 model=llama3.2:1b chars=305 dur=1m29.49257372s
```

UI:

```
[Tool current_weather]
[Tool current_weather]
{Response: LLM natural language response about the weather in Denver, CO.}
```

Assume the first `[Tool current_weather]` was printed, then retried?


Failure:

```
debug 2025/10/25 09:53:02 Tools: false
debug 2025/10/25 09:53:23 Tools: {current_weather, current_time}
debug 2025/10/25 09:53:59 Tool request: current_weather {"__mcp_attempt":0,"__user_prompt":"What is the current weather in Portland, OR?","function":"weather","parameters":"{ 'city': 'Portland', 'country': 'USA' }","type":""}
debug 2025/10/25 09:53:59 Tools: {current_weather, current_time}


[2025-10-25T09:52:59-07:00] MCP server started: binary=dist/agon-mcp_linux_amd64_v1/agon-mcp pid=66752
[2025-10-25T09:52:59-07:00] Available MCP tools: current_weather, current_time
[2025-10-25T09:52:59-07:00] MCP provider ready: using local server
[2025-10-25T09:53:00-07:00] Tool invoked: tool=loaded_models host=Ollama01
[2025-10-25T09:53:00-07:00] Tool bypassed: tool=loaded_models host=Ollama01 reason=delegated to Ollama API
[2025-10-25T09:53:02-07:00] Tool invoked: tool=ensure_model host=Ollama01 model=llama3.2:1b
[2025-10-25T09:53:12-07:00] Tool bypassed: tool=ensure_model host=Ollama01 model=llama3.2:1b reason=delegated to Ollama API
[2025-10-25T09:53:23-07:00] Tool invoked: tool=chat host=Ollama01 model=llama3.2:1b messages=1
[2025-10-25T09:53:23-07:00] Forwarding request: host=Ollama01 model=llama3.2:1b messages=1 tools=2
[2025-10-25T09:53:59-07:00] Tool requested: tool=current_weather args={"__mcp_attempt":0,"__user_prompt":"What is the current weather in Portland, OR?","function":"weather","parameters":"{ 'city': 'Portland', 'country': 'USA' }","type":""}
[2025-10-25T09:53:59-07:00] MCP tool detail: tool=current_weather attempt 0/1 failed: Error: 'location' argument is required.
[2025-10-25T09:53:59-07:00] MCP->LLM fix send: tool=current_weather host=Ollama01 model=llama3.2:1b
[2025-10-25T09:54:42-07:00] MCP->LLM fix recv: tool=current_weather host=Ollama01 model=llama3.2:1b chars=97 dur=42.70377153s
[2025-10-25T09:54:42-07:00] [*ERROR*] Tool bypassed: tool=chat host=Ollama01 model=llama3.2:1b reason=delegated to Ollama API
```