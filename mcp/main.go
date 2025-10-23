// mcp/main.go
// Minimal MCP server over stdio (JSON-RPC 2.0 + Content-Length framing)
// Tools: current_weather
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

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

// --- API Response Structs ---

// nominatimResponse defines the fields we need from OpenStreetMap
type nominatimResponse struct {
	Lat string `json:"lat"`
	Lon string `json:"lon"`
}

// openMeteoResponse defines the fields we need from Open-Meteo
type openMeteoResponse struct {
	CurrentUnits struct {
		Temperature   string `json:"temperature_2m"`
		ApparentTemp  string `json:"apparent_temperature"`
		Humidity      string `json:"relative_humidity_2m"`
		Precipitation string `json:"precipitation"`
		Rain          string `json:"rain"`
		WindSpeed     string `json:"wind_speed_10m"`
		WindDirection string `json:"wind_direction_10m"`
		WindGusts     string `json:"wind_gusts_10m"`
	} `json:"current_units"`
	Current struct {
		Temperature   float64 `json:"temperature_2m"`
		ApparentTemp  float64 `json:"apparent_temperature"`
		IsDay         int     `json:"is_day"`
		Humidity      float64 `json:"relative_humidity_2m"`
		Precipitation float64 `json:"precipitation"`
		Rain          float64 `json:"rain"`
		WindSpeed     float64 `json:"wind_speed_10m"`
		WindDirection float64 `json:"wind_direction_10m"`
		WindGusts     float64 `json:"wind_gusts_10m"`
	} `json:"current"`
}

// --- Global HTTP Client ---

var httpClient = &http.Client{
	Timeout: 10 * time.Second,
}

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

func toolDefinitions() []toolDef {
	return []toolDef{
		{
			Name:        "current_weather",
			Description: "Gets the current weather for a specified location.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"location": map[string]any{
						"type":        "string",
						"description": "The city and state, e.g., 'Portland, OR'",
					},
				},
				"required": []string{"location"},
			},
		},
	}
}

// --- Tool Implementation ---

// getGeocodedWeather handles the multi-step API calls
func getGeocodedWeather(location string) (string, error) {
	// Step 1: Geocode location string to lat/lon
	geoURL := fmt.Sprintf("https://nominatim.openstreetmap.org/search?q=%s&format=jsonv2&limit=1", url.QueryEscape(location))

	req, err := http.NewRequest("GET", geoURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create geocoding request: %v", err)
	}
	// Nominatim requires a descriptive User-Agent
	req.Header.Set("User-Agent", "mcp-weather-tool/1.0 (dev)")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("geocoding request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("geocoding service returned status: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read geocoding response: %v", err)
	}

	var geoResp []nominatimResponse
	if err := json.Unmarshal(body, &geoResp); err != nil {
		return "", fmt.Errorf("failed to parse geocoding JSON: %v", err)
	}

	if len(geoResp) == 0 {
		return "", fmt.Errorf("location not found: '%s'", location)
	}

	lat := geoResp[0].Lat
	lon := geoResp[0].Lon

	// Step 2: Get Weather from lat/lon
	weatherURL := fmt.Sprintf(
		"https://api.open-meteo.com/v1/forecast?latitude=%s&longitude=%s&current=temperature_2m,apparent_temperature,is_day,relative_humidity_2m,precipitation,rain,wind_speed_10m,wind_direction_10m,wind_gusts_10m",
		lat, lon,
	)

	resp, err = httpClient.Get(weatherURL)
	if err != nil {
		return "", fmt.Errorf("weather request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("weather service returned status: %s", resp.Status)
	}

	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read weather response: %v", err)
	}

	var weatherResp openMeteoResponse
	if err := json.Unmarshal(body, &weatherResp); err != nil {
		return "", fmt.Errorf("failed to parse weather JSON: %v", err)
	}

	// Step 3: Format output string for the LLM
	c := weatherResp.Current
	u := weatherResp.CurrentUnits
	dayNight := "night"
	if c.IsDay == 1 {
		dayNight = "day"
	}

	// This formatted string will be returned to the LLM
	result := fmt.Sprintf(
		"Weather for %s (Lat: %s, Lon: %s) (%s): Temp: %.1f%s, Feels Like: %.1f%s, Humidity: %.0f%s, Wind: %.1f%s @ %.0f°, Gusts: %.1f%s, Precip: %.2f%s, Rain: %.2f%s",
		location, lat, lon, dayNight,
		c.Temperature, u.Temperature,
		c.ApparentTemp, u.ApparentTemp,
		c.Humidity, u.Humidity,
		c.WindSpeed, u.WindSpeed,
		c.WindDirection,
		c.WindGusts, u.WindGusts,
		c.Precipitation, u.Precipitation,
		c.Rain, u.Rain,
	)

	return result, nil
}

func runTool(name string, args map[string]any) []contentPart {
	switch name {
	case "current_weather":
		locationVal, ok := args["location"]
		if !ok {
			return []contentPart{{Type: "text", Text: "Error: 'location' argument is required."}}
		}
		location, ok := locationVal.(string)
		if !ok {
			return []contentPart{{Type: "text", Text: "Error: 'location' argument must be a string."}}
		}
		if location == "" {
			return []contentPart{{Type: "text", Text: "Error: 'location' argument cannot be empty."}}
		}

		// Call the helper function
		weather, err := getGeocodedWeather(location)
		if err != nil {
			// Log the detailed error to stderr for the server operator
			log.Printf("Weather tool error for location '%s': %v", location, err)
			// Return a user-friendly error to the LLM
			return []contentPart{{Type: "text", Text: fmt.Sprintf("Error fetching weather: %v", err)}}
		}

		return []contentPart{{Type: "text", Text: weather}}
	}

	return []contentPart{{Type: "text", Text: fmt.Sprintf("Unknown tool: %s", name)}}
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
