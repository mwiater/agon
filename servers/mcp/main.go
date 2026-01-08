// servers/mcp/main.go
// Minimal MCP server over stdio (JSON-RPC 2.0 + Content-Length framing)
// Tools: available_tools, current_time, current_weather
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/mwiater/agon/internal/appconfig"
	"github.com/mwiater/agon/servers/mcp/tools"
)

var (
	configPath string
)

func init() {
	flag.StringVar(&configPath, "config", "", "path to the config file")
}

// --- Protocol data types ---

type jsonrpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonrpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type jsonrpcResponse struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      any           `json:"id,omitempty"`
	Result  any           `json:"result,omitempty"`
	Error   *jsonrpcError `json:"error,omitempty"`
}

// tools/call params
type toolsCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

var retryCount = (appconfig.Config{}).MCPRetryAttempts()

// --- Framing Helpers ---

func writeMessage(w *bufio.Writer, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Content-Length: %d\r\n\r\n", len(data)); err != nil {
		return err
	}
	if _, err := w.Write(data); err != nil {
		return err
	}
	return w.Flush()
}

func readMessage(r *bufio.Reader) (*jsonrpcRequest, error) {
	// Read headers until blank line
	headers := map[string]string{}
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return nil, io.EOF
			}
			return nil, err
		}
		// Normalize
		s := line
		if s == "\r\n" || s == "\n" {
			break
		}
		// Accumulate headers (allow LF-only too)
		s = strings.TrimRight(s, "\r\n")
		if s == "" {
			break
		}
		if i := strings.IndexByte(s, ':'); i >= 0 {
			key := strings.ToLower(strings.TrimSpace(s[:i]))
			val := strings.TrimSpace(s[i+1:])
			headers[key] = val
		}
	}
	clStr, ok := headers["content-length"]
	if !ok {
		return nil, fmt.Errorf("missing Content-Length")
	}
	var length int
	if _, err := fmt.Sscanf(clStr, "%d", &length); err != nil {
		return nil, fmt.Errorf("invalid Content-Length: %v", err)
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}
	var req jsonrpcRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}
	return &req, nil
}

// --- RPC Helpers ---

func makeResult(id any, result any) jsonrpcResponse {
	return jsonrpcResponse{JSONRPC: "2.0", ID: id, Result: result}
}

func makeError(id any, code int, msg string) jsonrpcResponse {
	return jsonrpcResponse{JSONRPC: "2.0", ID: id, Error: &jsonrpcError{Code: code, Message: msg}}
}

// --- Tool Definitions ---

func toolDefinitions() []tools.Definition {
	return []tools.Definition{
		tools.AvailableToolsDefinition(),
		tools.CurrentTimeDefinition(),
		tools.CurrentWeatherDefinition(),
	}
}

// --- Tool Implementation Wrapper ---

func runTool(name string, args map[string]any) []tools.ContentPart {
	handler := handlerFor(name)
	if handler == nil {
		return []tools.ContentPart{{Type: "text", Text: fmt.Sprintf("Unknown tool: %s", name)}}
	}

	return invokeWithRetries(name, handler, args)
}

func handlerFor(name string) tools.Handler {
	switch name {
	case tools.AvailableToolsName:
		return tools.AvailableTools
	case tools.CurrentWeatherName:
		return tools.CurrentWeather
	case tools.CurrentTimeName:
		return tools.CurrentTime
	default:
		return nil
	}
}

func attemptFromArgs(args map[string]any) int {
	if args == nil {
		return 1
	}
	if v, ok := args["__mcp_attempt"]; ok {
		switch val := v.(type) {
		case int:
			if val > 0 {
				return val
			}
		case int32:
			if val > 0 {
				return int(val)
			}
		case int64:
			if val > 0 {
				return int(val)
			}
		case float64:
			if n := int(val); n > 0 {
				return n
			}
		case float32:
			if n := int(val); n > 0 {
				return n
			}
		case string:
			if n, err := strconv.Atoi(val); err == nil && n > 0 {
				return n
			}
		}
	}
	return 1
}

func promptFromArgs(args map[string]any) string {
	if args == nil {
		return ""
	}
	if v, ok := args["__user_prompt"]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func invokeWithRetries(toolName string, handler tools.Handler, args map[string]any) []tools.ContentPart {
	attempt := attemptFromArgs(args)
	prompt := promptFromArgs(args)
	if attempt <= 0 {
		attempt = 1
	}
	content, err := handler(args)
	if err == nil {
		return content
	}

	maxRetries := retryCount
	logs := []tools.ContentPart{{Type: "log", Text: fmt.Sprintf("attempt %d/%d failed: %v", attempt, maxRetries, err)}}

	if attempt < maxRetries && prompt != "" {
		message := fmt.Sprintf("Tool error: %v\nOriginal request: %s\n Ensure that you provide the arguments to satisfy the tool requirements before trying again. ", err, prompt)
		logs = append(logs, tools.ContentPart{Type: "meta", Text: "retry"})
		logs = append(logs, tools.ContentPart{Type: "text", Text: message})
		return logs
	}

	logs = append(logs, tools.ContentPart{Type: "log", Text: fmt.Sprintf("giving up after %d attempts: %v", attempt, err)})
	logs = append(logs, tools.ContentPart{Type: "text", Text: "I could not handle your request."})
	return logs
}

// --- MCP Request Handler ---

func handleRequest(req *jsonrpcRequest, w *bufio.Writer) error {
	switch req.Method {
	case "initialize":
		result := map[string]any{
			"serverInfo":   map[string]any{"name": "agon-mcp", "version": "0.1.0"},
			"capabilities": map[string]any{"tools": map[string]any{"list": true, "call": true}},
		}
		return writeMessage(w, makeResult(req.ID, result))

	case "ping":
		return writeMessage(w, makeResult(req.ID, map[string]any{}))

	case "tools/list":
		result := map[string]any{"tools": toolDefinitions()}
		return writeMessage(w, makeResult(req.ID, result))

	case "tools/call":
		var p toolsCallParams
		if len(req.Params) > 0 {
			if err := json.Unmarshal(req.Params, &p); err != nil {
				return writeMessage(w, makeError(req.ID, -32602, "Invalid params"))
			}
		}
		if p.Arguments == nil {
			p.Arguments = map[string]any{}
		}
		content := runTool(p.Name, p.Arguments)
		result := map[string]any{"content": content}
		return writeMessage(w, makeResult(req.ID, result))
	}

	return writeMessage(w, makeError(req.ID, -32601, fmt.Sprintf("Method not found: %s", req.Method)))
}

// --- Main Server Loop ---

func main() {
	flag.Parse()
	cfg, err := appconfig.Load(configPath)
	if err == nil {
		retryCount = cfg.MCPRetryAttempts()
	}

	r := bufio.NewReader(os.Stdin)
	w := bufio.NewWriter(os.Stdout)

	for {
		req, err := readMessage(r)
		if err != nil {
			if err == io.EOF {
				return
			}
			// Try to send a generic server error if we can parse an id (we can't here); else break
			// write a best-effort error frame without id to keep stream sane
			_ = writeMessage(w, jsonrpcResponse{JSONRPC: "2.0", Error: &jsonrpcError{Code: -32000, Message: err.Error()}})
			return
		}
		if req == nil {
			// malformed; end
			return
		}
		if err := handleRequest(req, w); err != nil {
			// Attempt to report per-request error
			_ = writeMessage(w, makeError(req.ID, -32000, err.Error()))
			// Do not exit; continue processing
		}
	}
}
