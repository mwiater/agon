package tools

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateCurrentWeatherData(t *testing.T) {
	valid := `{"arguments":{"location":"Austin, TX"}}`
	ok, err := ValidateCurrentWeatherData(valid)
	if err != nil || !ok {
		t.Fatalf("expected valid payload, got ok=%v err=%v", ok, err)
	}

	missingArgs := `{"foo":"bar"}`
	ok, err = ValidateCurrentWeatherData(missingArgs)
	if err == nil || ok {
		t.Fatalf("expected invalid payload, got ok=%v err=%v", ok, err)
	}
}

func TestNewParsedWeatherMissingDaily(t *testing.T) {
	raw := openMeteoResponse{}
	if _, err := NewParsedWeather(raw); err == nil {
		t.Fatalf("expected error for missing daily data")
	}
}

func TestNewParsedWeatherSuccess(t *testing.T) {
	raw := openMeteoResponse{
		Timezone: "America/Chicago",
		CurrentUnits: struct {
			Time                string `json:"time"`
			Interval            string `json:"interval"`
			Temperature         string `json:"temperature_2m"`
			RelativeHumidity    string `json:"relative_humidity_2m"`
			IsDay               string `json:"is_day"`
			Precipitation       string `json:"precipitation"`
			CloudCover          string `json:"cloud_cover"`
			WindSpeed10M        string `json:"wind_speed_10m"`
			ApparentTemperature string `json:"apparent_temperature"`
		}{
			Temperature:         "F",
			RelativeHumidity:    "%",
			Precipitation:       "in",
			CloudCover:          "%",
			WindSpeed10M:        "mph",
			ApparentTemperature: "F",
		},
		Current: struct {
			Time                string  `json:"time"`
			Interval            int     `json:"interval"`
			Temperature         float64 `json:"temperature_2m"`
			RelativeHumidity    int     `json:"relative_humidity_2m"`
			IsDay               int     `json:"is_day"`
			Precipitation       float64 `json:"precipitation"`
			CloudCover          int     `json:"cloud_cover"`
			WindSpeed10M        float64 `json:"wind_speed_10m"`
			ApparentTemperature float64 `json:"apparent_temperature"`
		}{
			Temperature:         72.2,
			RelativeHumidity:    55,
			IsDay:               1,
			Precipitation:       0.1,
			CloudCover:          20,
			WindSpeed10M:        5.4,
			ApparentTemperature: 70.1,
		},
		DailyUnits: struct {
			Time             string `json:"time"`
			TemperatureMax   string `json:"temperature_2m_max"`
			TemperatureMin   string `json:"temperature_2m_min"`
			Sunrise          string `json:"sunrise"`
			Sunset           string `json:"sunset"`
			PrecipitationSum string `json:"precipitation_sum"`
		}{
			TemperatureMax:   "F",
			TemperatureMin:   "F",
			Sunrise:          "iso",
			Sunset:           "iso",
			PrecipitationSum: "in",
		},
		Daily: struct {
			Time             []string  `json:"time"`
			TemperatureMax   []float64 `json:"temperature_2m_max"`
			TemperatureMin   []float64 `json:"temperature_2m_min"`
			Sunrise          []string  `json:"sunrise"`
			Sunset           []string  `json:"sunset"`
			PrecipitationSum []float64 `json:"precipitation_sum"`
		}{
			Time:             []string{"2025-01-01"},
			TemperatureMax:   []float64{81.2},
			TemperatureMin:   []float64{61.5},
			Sunrise:          []string{"2025-01-01T07:05"},
			Sunset:           []string{"2025-01-01T17:45"},
			PrecipitationSum: []float64{0.2},
		},
	}

	parsed, err := NewParsedWeather(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsed == nil {
		t.Fatalf("expected parsed weather")
	}
	if !strings.Contains(parsed.RelativeHumidity, "%") {
		t.Fatalf("expected percent formatting, got %s", parsed.RelativeHumidity)
	}
	if parsed.Sunrise != "7:05 AM" {
		t.Fatalf("expected formatted sunrise, got %s", parsed.Sunrise)
	}
	if parsed.Sunset != "5:45 PM" {
		t.Fatalf("expected formatted sunset, got %s", parsed.Sunset)
	}
}

func TestAvailableToolsPayload(t *testing.T) {
	parts, err := AvailableTools(map[string]any{})
	if err != nil {
		t.Fatalf("AvailableTools error: %v", err)
	}
	if len(parts) < 2 {
		t.Fatalf("expected at least json and interpret parts")
	}

	var payload []map[string]string
	if err := json.Unmarshal([]byte(parts[0].Text), &payload); err != nil {
		t.Fatalf("invalid json payload: %v", err)
	}
	if len(payload) == 0 {
		t.Fatalf("expected tools in payload")
	}
}
