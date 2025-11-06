package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/mwiater/agon/internal/logging"
)

type jsonrpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonrpcError   `json:"error,omitempty"`
}

type jsonrpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcMetadata struct {
	method string
	host   string
	model  string
	tool   string
}

func (p *Provider) storeRPCMeta(id string, meta rpcMetadata) {
	p.rpcMetaMu.Lock()
	if p.rpcMeta == nil {
		p.rpcMeta = make(map[string]rpcMetadata)
	}
	p.rpcMeta[id] = meta
	p.rpcMetaMu.Unlock()
}

func (p *Provider) popRPCMeta(id string) (rpcMetadata, bool) {
	p.rpcMetaMu.Lock()
	defer p.rpcMetaMu.Unlock()
	if p.rpcMeta == nil {
		return rpcMetadata{}, false
	}
	meta, ok := p.rpcMeta[id]
	if ok {
		delete(p.rpcMeta, id)
	}
	return meta, ok
}

func normalizeID(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return ""
	}
	if trimmed[0] == '"' {
		if unquoted, err := strconv.Unquote(trimmed); err == nil {
			return unquoted
		}
		trimmed = strings.Trim(trimmed, "\"")
	}
	return trimmed
}

func (p *Provider) nextID() int64 {
	p.seqMu.Lock()
	defer p.seqMu.Unlock()
	p.seq++
	return p.seq
}

func (p *Provider) writeRawFrame(data []byte) error {
	if _, err := fmt.Fprintf(p.writer, "Content-Length: %d\r\n\r\n", len(data)); err != nil {
		return err
	}
	if _, err := p.writer.Write(data); err != nil {
		return err
	}
	return p.writer.Flush()
}

func (p *Provider) readResponse(ctx context.Context) (jsonrpcResponse, []byte, error) {
	type result struct {
		resp jsonrpcResponse
		raw  []byte
		err  error
	}
	done := make(chan result, 1)
	go func() {
		r, raw, err := p.readResponseBlocking()
		done <- result{resp: r, raw: raw, err: err}
	}()

	select {
	case <-ctx.Done():
		return jsonrpcResponse{}, nil, ctx.Err()
	case res := <-done:
		return res.resp, res.raw, res.err
	}
}

func (p *Provider) readResponseBlocking() (jsonrpcResponse, []byte, error) {
	headers := make(map[string]string)
	for {
		line, err := p.reader.ReadString('\n')
		if err != nil {
			return jsonrpcResponse{}, nil, err
		}
		if line == "\r\n" || line == "\n" {
			break
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if idx := strings.IndexByte(line, ':'); idx >= 0 {
			headers[strings.ToLower(strings.TrimSpace(line[:idx]))] = strings.TrimSpace(line[idx+1:])
		}
	}

	cl, ok := headers["content-length"]
	if !ok {
		return jsonrpcResponse{}, nil, fmt.Errorf("missing Content-Length header")
	}

	var length int
	if _, err := fmt.Sscanf(cl, "%d", &length); err != nil {
		return jsonrpcResponse{}, nil, fmt.Errorf("invalid Content-Length: %w", err)
	}

	body := make([]byte, length)
	if _, err := io.ReadFull(p.reader, body); err != nil {
		return jsonrpcResponse{}, nil, err
	}

	var resp jsonrpcResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return jsonrpcResponse{}, body, err
	}
	return resp, body, nil
}

func (p *Provider) rpcCall(ctx context.Context, method string, params map[string]any, meta rpcMetadata) (jsonrpcResponse, error) {
	p.rpcMu.Lock()
	defer p.rpcMu.Unlock()

	id := p.nextID()
	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
	}
	if params != nil {
		payload["params"] = params
	}
	if meta.method == "" {
		meta.method = method
	}
	if meta.host == "" {
		meta.host = p.defaultMCPHost()
	}
	metaKey := fmt.Sprintf("%d", id)
	p.storeRPCMeta(metaKey, meta)

	data, err := json.Marshal(payload)
	if err != nil {
		p.popRPCMeta(metaKey)
		return jsonrpcResponse{}, err
	}
	logging.LogRequest("AGON->MCP", meta.host, meta.model, toolLabel(meta), data)

	if err := p.writeRawFrame(data); err != nil {
		p.popRPCMeta(metaKey)
		return jsonrpcResponse{}, err
	}

	resp, raw, err := p.readResponse(ctx)
	if err != nil {
		p.popRPCMeta(metaKey)
		return jsonrpcResponse{}, err
	}

	respID := normalizeID(resp.ID)
	if respID == "" {
		respID = metaKey
	}
	storedMeta, ok := p.popRPCMeta(respID)
	if ok {
		meta = storedMeta
	}

	payloadIn := raw
	if len(payloadIn) == 0 {
		if data, marshalErr := json.Marshal(resp); marshalErr == nil {
			payloadIn = data
		}
	}
	logging.LogRequest("MCP->AGON", meta.host, meta.model, toolLabel(meta), payloadIn)

	if resp.Error != nil {
		return jsonrpcResponse{}, fmt.Errorf("%s", resp.Error.Message)
	}
	return resp, nil
}
