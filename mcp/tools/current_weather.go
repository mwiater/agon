package tools

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/xeipuuv/gojsonschema"
)

// nominatimResponse defines the fields we need from OpenStreetMap.
type nominatimResponse struct {
	Lat string `json:"lat"`
	Lon string `json:"lon"`
}

// openMeteoResponse defines the fields we need from Open-Meteo.
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

// httpClient is reused across requests to avoid recreating transport resources.
var httpClient = &http.Client{Timeout: 10 * time.Second}

// CurrentWeatherDefinition describes the weather tool to the MCP host.
func CurrentWeatherDefinition() Definition {
	return Definition{
		Name:        CurrentWeatherName,
		Description: "Get the current weather for a given location.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"location": map[string]any{
					"type":        "string",
					"description": "The city and state, e.g. San Francisco, CA",
				},
			},
			"required": []string{"location"},
		},
	}
}

// CurrentWeatherTool returns the complete, wrapped tool definition.
func CurrentWeatherTool() Tool {
	return Tool{
		Type:     "function",
		Function: CurrentWeatherDefinition(), // Call your existing function here
	}
}

// ValidateCurrentWeatherData takes a JSON string, extracts the "arguments"
// object, and validates it against the CurrentWeatherDefinition's schema.
func ValidateCurrentWeatherData(jsonString string) (bool, error) {
	// 1. Load the schema from the function
	schemaDef := CurrentWeatherDefinition().Parameters
	schemaLoader := gojsonschema.NewGoLoader(schemaDef)

	// 2. Parse the input string to find the "arguments"
	var inputData map[string]json.RawMessage
	if err := json.Unmarshal([]byte(jsonString), &inputData); err != nil {
		return false, fmt.Errorf("could not parse outer JSON: %w", err)
	}

	// 3. Extract the "arguments" JSON (which is what we want to validate)
	argumentsJSON, ok := inputData["arguments"]
	if !ok {
		// If the input doesn't even have an "arguments" key, it's invalid.
		return false, fmt.Errorf("input JSON missing 'arguments' key")
	}

	// 4. Create a loader for the "arguments" JSON
	documentLoader := gojsonschema.NewBytesLoader(argumentsJSON)

	// 5. Perform the validation
	result, err := gojsonschema.Validate(schemaLoader, documentLoader)
	if err != nil {
		// This error is for issues with the validation process itself
		return false, fmt.Errorf("schema validation error: %w", err)
	}

	// 6. Check the result
	if result.Valid() {
		return true, nil
	}

	// 7. If invalid, build a comprehensive error message
	var errs []string
	for _, desc := range result.Errors() {
		errs = append(errs, desc.String())
	}
	return false, fmt.Errorf("JSON validation failed: %s", strings.Join(errs, ", "))
}

// CurrentWeather executes the weather lookup workflow and returns JSON content for the LLM to interpret.
func CurrentWeather(args map[string]any) ([]ContentPart, error) {
	locationVal, ok := args["location"]
	if !ok {
		return nil, fmt.Errorf("Error: 'location' argument is required.")
	}
	location, ok := locationVal.(string)
	if !ok {
		return nil, fmt.Errorf("Error: 'location' argument must be a string.")
	}
	if location == "" {
		return nil, fmt.Errorf("Error: 'location' argument cannot be empty.")
	}

	weather, err := getGeocodedWeather(location)
	if err != nil {
		return nil, fmt.Errorf("Error fetching weather: %v", err)
	}

	jsonWeather, err := json.Marshal(weather.Current)
	if err != nil {
		return nil, fmt.Errorf("Error preparing weather response: %w", err)
	}

	interpretPrompt := strings.Join([]string{
		"You are a helpful assistant. Interpret the provided JSON weather data and reply in natural language in 2 sentences or less.",
		"Avoid repeating raw numbers unnecessarily; keep it concise and readable by a non-technical user.",
		"JSON Weather Data: " + string(jsonWeather),
	}, " ")

	return []ContentPart{
		{Type: "json", Text: string(jsonWeather)},
		{Type: "interpret", Text: interpretPrompt},
	}, nil
}

func getGeocodedWeather(location string) (openMeteoResponse, error) {
	geoURL := fmt.Sprintf("https://nominatim.openstreetmap.org/search?q=%s&format=jsonv2&limit=1", url.QueryEscape(location))

	req, err := http.NewRequest("GET", geoURL, nil)
	if err != nil {
		return openMeteoResponse{}, fmt.Errorf("failed to create geocoding request: %v", err)
	}
	req.Header.Set("User-Agent", "mcp-weather-tool/1.0 (dev)")

	resp, err := httpClient.Do(req)
	if err != nil {
		return openMeteoResponse{}, fmt.Errorf("geocoding request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return openMeteoResponse{}, fmt.Errorf("geocoding service returned status: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return openMeteoResponse{}, fmt.Errorf("failed to read geocoding response: %v", err)
	}

	var geoResp []nominatimResponse
	if err := json.Unmarshal(body, &geoResp); err != nil {
		return openMeteoResponse{}, fmt.Errorf("failed to parse geocoding JSON: %v", err)
	}

	if len(geoResp) == 0 {
		return openMeteoResponse{}, fmt.Errorf("location not found: '%s'", location)
	}

	lat := geoResp[0].Lat
	lon := geoResp[0].Lon

	weatherURL := fmt.Sprintf(
		"https://api.open-meteo.com/v1/forecast?latitude=%s&longitude=%s&current=temperature_2m,relative_humidity_2m,apparent_temperature,is_day,precipitation,wind_speed_10m&temperature_unit=fahrenheit&wind_speed_unit=mph",
		lat, lon,
	)

	resp, err = httpClient.Get(weatherURL)
	if err != nil {
		return openMeteoResponse{}, fmt.Errorf("weather request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return openMeteoResponse{}, fmt.Errorf("weather service returned status: %s", resp.Status)
	}

	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return openMeteoResponse{}, fmt.Errorf("failed to read weather response: %v", err)
	}

	var weatherResp openMeteoResponse
	if err := json.Unmarshal(body, &weatherResp); err != nil {
		return openMeteoResponse{}, fmt.Errorf("failed to parse weather JSON: %v", err)
	}

	return weatherResp, nil
}
