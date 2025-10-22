// mcp/main.go
// Minimal MCP server over stdio (JSON-RPC 2.0 + Content-Length framing)
// Tools: echo, word_count, sentiment (naive)
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// Protocol data types
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

// tools/list shape
type toolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

// tools/call result content part
type contentPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// tools/call params
type toolsCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// framing helpers
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

func makeResult(id any, result any) jsonrpcResponse {
	return jsonrpcResponse{JSONRPC: "2.0", ID: id, Result: result}
}

func makeError(id any, code int, msg string) jsonrpcResponse {
	return jsonrpcResponse{JSONRPC: "2.0", ID: id, Error: &jsonrpcError{Code: code, Message: msg}}
}

func toolDefinitions() []toolDef {
	schema := func() map[string]any {
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"text": map[string]any{"type": "string"},
			},
			"required": []string{"text"},
		}
	}
	return []toolDef{
		{
			Name:        "echo",
			Description: "Return the same text provided.",
			InputSchema: schema(),
		},
		{
			Name:        "word_count",
			Description: "Count words in the given text (split on whitespace).",
			InputSchema: schema(),
		},
		{
			Name:        "sentiment",
			Description: "Naive sentiment (positive/negative/neutral) using keyword lists.",
			InputSchema: schema(),
		},
	}
}

var positiveWords = map[string]struct{}{
	"good": {}, "great": {}, "excellent": {}, "awesome": {}, "fantastic": {},
	"love": {}, "like": {}, "amazing": {}, "happy": {}, "positive": {},
	"nice": {}, "wonderful": {},
}

var negativeWords = map[string]struct{}{
	"bad": {}, "terrible": {}, "awful": {}, "hate": {}, "dislike": {},
	"sad": {}, "angry": {}, "poor": {}, "horrible": {}, "negative": {},
	"worse": {},
}

func runTool(name string, args map[string]any) []contentPart {
	getText := func() string {
		if v, ok := args["text"]; ok {
			switch t := v.(type) {
			case string:
				return t
			default:
				b, _ := json.Marshal(t)
				return string(b)
			}
		}
		return ""
	}

	switch name {
	case "echo":
		text := getText()
		return []contentPart{{Type: "text", Text: text}}

	case "word_count":
		text := getText()
		// Split on whitespace and count non-empty
		fields := strings.Fields(text)
		return []contentPart{{Type: "text", Text: fmt.Sprintf("%d", len(fields))}}

	case "sentiment":
		text := getText()
		// tokenization: strip simple punctuation and lowercase
		split := strings.Fields(text)
		pos, neg := 0, 0
		for _, tok := range split {
			t := strings.ToLower(strings.Trim(tok, ".,!?;:\"'()[]{}-"))
			if t == "" {
				continue
			}
			if _, ok := positiveWords[t]; ok {
				pos++
			}
			if _, ok := negativeWords[t]; ok {
				neg++
			}
		}
		label := "neutral"
		score := 0.0
		if pos > neg {
			label = "positive"
			score = float64(pos-neg) / float64(max(1, pos+neg))
		} else if neg > pos {
			label = "negative"
			score = float64(neg-pos) / float64(max(1, pos+neg))
		}
		return []contentPart{{Type: "text", Text: fmt.Sprintf("%s (score=%.2f)", label, score)}}
	}

	return []contentPart{{Type: "text", Text: fmt.Sprintf("Unknown tool: %s", name)}}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

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

func main() {
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
