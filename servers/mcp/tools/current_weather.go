package tools

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
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
	Latitude             float64 `json:"latitude"`
	Longitude            float64 `json:"longitude"`
	GenerationtimeMs     float64 `json:"generationtime_ms"`
	UtcOffsetSeconds     int     `json:"utc_offset_seconds"`
	Timezone             string  `json:"timezone"`
	TimezoneAbbreviation string  `json:"timezone_abbreviation"`
	Elevation            float64 `json:"elevation"`
	CurrentUnits         struct {
		Time                string `json:"time"`
		Interval            string `json:"interval"`
		Temperature         string `json:"temperature_2m"`
		RelativeHumidity    string `json:"relative_humidity_2m"`
		IsDay               string `json:"is_day"`
		Precipitation       string `json:"precipitation"`
		CloudCover          string `json:"cloud_cover"`
		WindSpeed10M        string `json:"wind_speed_10m"`
		ApparentTemperature string `json:"apparent_temperature"`
	} `json:"current_units"`
	Current struct {
		Time                string  `json:"time"`
		Interval            int     `json:"interval"`
		Temperature         float64 `json:"temperature_2m"`
		RelativeHumidity    int     `json:"relative_humidity_2m"`
		IsDay               int     `json:"is_day"`
		Precipitation       float64 `json:"precipitation"`
		CloudCover          int     `json:"cloud_cover"`
		WindSpeed10M        float64 `json:"wind_speed_10m"`
		ApparentTemperature float64 `json:"apparent_temperature"`
	} `json:"current"`
	DailyUnits struct {
		Time             string `json:"time"`
		TemperatureMax   string `json:"temperature_2m_max"`
		TemperatureMin   string `json:"temperature_2m_min"`
		Sunrise          string `json:"sunrise"`
		Sunset           string `json:"sunset"`
		PrecipitationSum string `json:"precipitation_sum"`
	} `json:"daily_units"`
	Daily struct {
		Time             []string  `json:"time"`
		TemperatureMax   []float64 `json:"temperature_2m_max"`
		TemperatureMin   []float64 `json:"temperature_2m_min"`
		Sunrise          []string  `json:"sunrise"`
		Sunset           []string  `json:"sunset"`
		PrecipitationSum []float64 `json:"precipitation_sum"`
	} `json:"daily"`
}

type ParsedWeather struct {
	Timezone            string
	Temperature         string
	RelativeHumidity    string
	IsDay               bool
	Precipitation       string
	CloudCover          string
	WindSpeed10M        string
	ApparentTemperature string
	Low                 string
	High                string
	Sunrise             string
	Sunset              string
	TotalPrecipitation  string
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

	parsedWeather, err := NewParsedWeather(weather)
	if err != nil {
		fmt.Println("Error parsing weather:", err)
		os.Exit(1)
	}

	currentWeather, err := json.Marshal(parsedWeather)
	if err != nil {
		return nil, fmt.Errorf("Error preparing weather response: %w", err)
	}

	interpretPrompt := strings.Join([]string{
		"You are a helpful assistant. Interpret the provided JSON weather data and reply in natural language in 2 sentences or less.",
		"Avoid repeating raw numbers unnecessarily; keep it concise and readable by a non-technical user.",
		"JSON Weather Data: " + string(currentWeather),
	}, " ")

	return []ContentPart{
		{Type: "json", Text: string(currentWeather)},
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
		"https://api.open-meteo.com/v1/forecast?latitude=%s&longitude=%s&daily=temperature_2m_max,temperature_2m_min,sunrise,sunset,precipitation_sum&current=temperature_2m,relative_humidity_2m,is_day,precipitation,cloud_cover,wind_speed_10m,apparent_temperature&timezone=auto&forecast_days=1&wind_speed_unit=mph&temperature_unit=fahrenheit",
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

// NewParsedWeather is a "constructor" function that transforms the raw
// API response into your clean ParsedWeather struct.
func NewParsedWeather(raw openMeteoResponse) (*ParsedWeather, error) {

	// --- Daily Data Check ---
	// We must ensure the daily slices are not empty before
	// trying to access them, or the program will panic.
	if len(raw.Daily.Time) == 0 ||
		len(raw.Daily.TemperatureMax) == 0 ||
		len(raw.Daily.TemperatureMin) == 0 ||
		len(raw.Daily.Sunrise) == 0 ||
		len(raw.Daily.Sunset) == 0 ||
		len(raw.Daily.PrecipitationSum) == 0 {
		return nil, fmt.Errorf("daily forecast data is missing or incomplete")
	}

	// All data seems present, proceed with transformation.
	p := &ParsedWeather{
		Timezone: raw.Timezone,

		// --- Current Data ---
		Temperature:         formatFloat(raw.Current.Temperature, raw.CurrentUnits.Temperature),
		RelativeHumidity:    formatInt(raw.Current.RelativeHumidity, raw.CurrentUnits.RelativeHumidity),
		IsDay:               raw.Current.IsDay == 1,
		Precipitation:       formatFloat(raw.Current.Precipitation, raw.CurrentUnits.Precipitation),
		CloudCover:          formatInt(raw.Current.CloudCover, raw.CurrentUnits.CloudCover),
		WindSpeed10M:        formatFloat(raw.Current.WindSpeed10M, raw.CurrentUnits.WindSpeed10M),
		ApparentTemperature: formatFloat(raw.Current.ApparentTemperature, raw.CurrentUnits.ApparentTemperature),

		// --- Daily Data (accessing [0] is now safe) ---
		High:               formatFloat(raw.Daily.TemperatureMax[0], raw.DailyUnits.TemperatureMax),
		Low:                formatFloat(raw.Daily.TemperatureMin[0], raw.DailyUnits.TemperatureMin),
		TotalPrecipitation: formatFloat(raw.Daily.PrecipitationSum[0], raw.DailyUnits.PrecipitationSum),
		// Use the time-formatting helper for a cleaner look
		Sunrise: formatTime(raw.Daily.Sunrise[0]),
		Sunset:  formatTime(raw.Daily.Sunset[0]),
	}

	return p, nil
}

// 4. --- HELPER FUNCTIONS ---

// formatFloat formats a float64 to one decimal place and appends the unit.
func formatFloat(val float64, unit string) string {
	return fmt.Sprintf("%.1f %s", val, unit)
}

// formatInt formats an int and appends the unit.
func formatInt(val int, unit string) string {
	// Handle units like "%" where a space is awkward
	if unit == "%" {
		return fmt.Sprintf("%d%s", val, unit)
	}
	return fmt.Sprintf("%d %s", val, unit)
}

// formatTime parses an ISO-ish time string and formats it to be more
// human-readable (e.g., "7:05 AM").
// It gracefully falls back to the raw string if parsing fails.
func formatTime(rawTime string) string {
	// The API format is "2006-01-02T15:04"
	t, err := time.Parse("2006-01-02T15:04", rawTime)
	if err != nil {
		return rawTime // Fallback to raw string
	}
	// Format to "3:04 PM" (e.g., "7:05 AM", "4:44 PM")
	return t.Format("3:04 PM")
}
