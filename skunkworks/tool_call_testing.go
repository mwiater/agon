package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/k0kubun/pp"
	"github.com/mwiater/agon/internal/appconfig"
	"github.com/mwiater/agon/internal/models"
)

// --- Configuration ---

// Set the number of times to loop through the entire model list.
const interactionCount = 5

// Set the path to your agon CLI tool.
// const agonCLIPath = "dist/agon_windows_amd64_v1/agon.exe"
const agonCLIPath = "dist/agon_linux_amd64_v1/agon"

// Set the output filename for the final JSON report.
const outputReportFile = "skunkworks/reports/model_tools_report.json"

// Set the log file path
const logFilePath = "skunkworks/logs/tool_tester.log"

// Set your hosts. The number of hosts determines the number of parallel requests.
var (
	hosts = []string{}

	// Set your models to be tested.
	toolModels = []string{
		//"llama3.2:3b",
		"granite4:350m",
		//"granite4:1b",
		//"granite4:micro",
		//"granite3.1-moe:3b",
		//"deepseek-r1:1.5b",
		//"llama3.2:1b",
		//"granite3.1-moe:1b",
		//"qwen3:1.7b",
	}
)

// Select the active payload template for this run.
type payloadMode string

const (
	payloadModeWeather payloadMode = "weather"
	payloadModeTime    payloadMode = "time"
)

var payloadModes = []payloadMode{
	payloadModeWeather,
	payloadModeTime,
}

var responseFilePaths = map[payloadMode]string{
	payloadModeWeather: "skunkworks/responses/weather.json",
	payloadModeTime:    "skunkworks/responses/time.json",
}

type payloadConfig struct {
	prompt       string
	expectedTool string
}

var payloadConfigs = map[payloadMode]payloadConfig{
	payloadModeWeather: {
		prompt:       "What is the weather in Portland, OR?",
		expectedTool: "current_weather",
	},
	payloadModeTime: {
		prompt:       "What is the current time?",
		expectedTool: "current_time",
	},
}

// PAYLOAD_TEMPLATE_CURRENT_WEATHER is the JSON payload sent to the API TO TEST THE WEATHER TOOL.
// The %s is a placeholder for the model name.
const PAYLOAD_TEMPLATE_CURRENT_WEATHER = `{
  "model": "%s",
  "stream": false,
  "messages": [
    {
      "role": "user",
      "content": "What is the weather in Portland, OR?"
    }
  ],
  "tools": [
    {
      "type": "function",
      "function": {
        "name": "current_weather",
        "description": "Get the current weather for a given location",
        "parameters": {
          "type": "object",
          "properties": {
            "location": {
              "type": "string",
              "description": "The city and state, e.g. San Francisco, CA"
            }
          },
          "required": ["location"]
        }
      }
    },
    {
      "type": "function",
      "function": {
        "name": "current_time",
        "description": "Get the current time.",
        "parameters": {
          "type": "object",
          "properties": {}
        }
      }
    }
  ]
}`

// PAYLOAD_TEMPLATE_CURRENT_TIME is the JSON payload sent to the API TO TEST THE TIME TOOL.
// The %s is a placeholder for the model name.
const PAYLOAD_TEMPLATE_CURRENT_TIME = `{
  "model": "%s",
  "stream": false,
  "messages": [
    {
      "role": "user",
      "content": "What is the current time?"
    }
  ],
  "tools": [
    {
      "type": "function",
      "function": {
        "name": "current_weather",
        "description": "Get the current weather for a given location",
        "parameters": {
          "type": "object",
          "properties": {
            "location": {
              "type": "string",
              "description": "The city and state, e.g. San Francisco, CA"
            }
          },
          "required": ["location"]
        }
      }
    },
    {
      "type": "function",
      "function": {
        "name": "current_time",
        "description": "Get the current time.",
        "parameters": {
          "type": "object",
          "properties": {}
        }
      }
    }
  ]
}`

// --- Structs for Parsing ---

// Struct for the API response with correct nesting
type apiResponse struct {
	Message struct {
		Role      string     `json:"role"`
		Content   string     `json:"content"`
		ToolCalls []toolCall `json:"tool_calls"`
	} `json:"message"`
}

// Struct for the standard tool_calls format.
type toolCall struct {
	Function struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
	} `json:"function"`
}

// Struct for the embedded JSON in the content field.
type embeddedToolCall struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// No regex needed - we'll use a smarter extraction approach

// --- Job/Worker Pool Structs ---

type job struct {
	modelName string
	mode      payloadMode
}

type result struct {
	modelName   string
	success     bool
	rawResponse json.RawMessage // The raw JSON response body
	mode        payloadMode
}

type toolValidation struct {
	expectedName    string
	criterion1Label string
	criterion2Label string
	validateArgs    func(map[string]interface{}) bool
}

// --- Main Application ---

var successfulResult = color.New(color.FgGreen).SprintFunc()
var failedResult = color.New(color.FgRed).SprintFunc()

func main() {
	loadedCfg, err := appconfig.Load("")
	if err != nil {
		fmt.Printf("failed to load temp config: %v", err)
	}

	// Populate hosts from loaded config
	for _, host := range loadedCfg.Hosts {
		hosts = append(hosts, host.URL)
	}

	if len(hosts) == 0 {
		pp.Println("No hosts found in configuration. Exiting.")
		os.Exit(1)
	}

	// Ensure log directory exists before opening file
	logDir := getDir(logFilePath)
	if logDir != "." {
		if err := os.MkdirAll(logDir, 0755); err != nil {
			log.Fatalf("Failed to create log directory: %v", err)
		}
	}

	// Use a file for logging as well as stdout
	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	defer logFile.Close()

	// Set log output to be multi-writer: stdout and the file
	mw := io.MultiWriter(os.Stdout, logFile)
	log.SetOutput(mw)

	// Truncate output files before starting
	log.Println("Truncating output files...")
	// We use "{}" for JSON files as it's the minimal valid content.
	truncateFile(outputReportFile, "{}")
	for _, path := range responseFilePaths {
		truncateFile(path, "{}")
	}

	log.Println("Starting request runner...")
	log.Printf("Total Models: %d", len(toolModels))
	log.Printf("Parallel Hosts (Batch Size): %d", len(hosts))
	log.Printf("Total Iterations: %d", interactionCount)
	log.Printf("Total Requests to be made: %d", len(toolModels)*interactionCount*len(payloadModes))
	log.Println("----------------------------------------")

	// successCounts will store the total number of successful responses for each model.
	successCounts := make(map[payloadMode]map[string]int)
	for _, mode := range payloadModes {
		successCounts[mode] = make(map[string]int)
		for _, model := range toolModels {
			successCounts[mode][model] = 0
		}
	}

	numHosts := len(hosts)

	// --- Initial Unload ---
	// Unload models once before the entire test run starts.
	log.Println("--- Initial Model Unload ---")
	models.UnloadModels(&loadedCfg)
	log.Println("----------------------------------------")

	// Create response files for streaming writes
	respFiles := make(map[payloadMode]*os.File)
	firstModel := make(map[payloadMode]*bool)

	for mode, path := range responseFilePaths {
		file, err := createResponseFile(path)
		if err != nil {
			log.Fatalf("Failed to create response file for %s: %v", mode, err)
		}
		respFiles[mode] = file
		defer file.Close()

		flag := new(bool)
		*flag = true
		firstModel[mode] = flag

		if _, err := file.WriteString("{\n"); err != nil {
			log.Fatalf("Failed to write to response file for %s: %v", mode, err)
		}
	}

	// Main loop for n iterations.
	log.Printf("Beginning %d test iterations...", interactionCount)
	for i := 0; i < interactionCount; i++ {
		log.Printf("--- Starting Iteration %d/%d ---", i+1, interactionCount)

		for _, mode := range payloadModes {
			log.Printf("===> Running mode: %s", mode)

			// Loop through models in batches of numHosts
			totalBatches := (len(toolModels) + numHosts - 1) / numHosts
			for j := 0; j < len(toolModels); j += numHosts {
				// Determine the models for this specific batch
				batchEnd := j + numHosts
				if batchEnd > len(toolModels) {
					batchEnd = len(toolModels)
				}
				batchModels := toolModels[j:batchEnd]
				numInBatch := len(batchModels)

				logline := fmt.Sprintf("[Iter %d | Mode %s] Processing batch %d/%d (Models %d-%d)...", i+1, mode, (j/numHosts)+1, totalBatches, j+1, batchEnd)
				log.Print(successfulResult(logline))

				// 1. Set up the worker pool channels for this batch.
				logline = fmt.Sprintf("[Iter %d | Mode %s | Batch %d] Setting up job and result channels.", i+1, mode, (j/numHosts)+1)
				log.Print(successfulResult(logline))

				jobs := make(chan job, numInBatch)
				results := make(chan result, numInBatch)

				// 2. Start workers (one per host).
				logline = fmt.Sprintf("[Iter %d | Mode %s | Batch %d] Starting %d workers...", i+1, mode, (j/numHosts)+1, numHosts)
				log.Print(successfulResult(logline))

				for w := 0; w < numHosts; w++ {
					go worker(w, hosts[w], jobs, results)
				}

				// 3. Send all jobs (one per model) for this batch.
				logline = fmt.Sprintf("[Iter %d | Mode %s | Batch %d] Sending %d jobs to workers...", i+1, mode, (j/numHosts)+1, numInBatch)
				log.Print(successfulResult(logline))

				for _, model := range batchModels {
					jobs <- job{modelName: model, mode: mode}
				}
				close(jobs) // Signal that no more jobs will be sent for this batch.
				logline = fmt.Sprintf("[Iter %d | Mode %s | Batch %d] All jobs sent. Channel closed.", i+1, mode, (j/numHosts)+1)
				log.Print(successfulResult(logline))

				// 4. Collect all results for this batch.
				logline = fmt.Sprintf("[Iter %d | Mode %s | Batch %d] Waiting to collect %d results...", i+1, mode, (j/numHosts)+1, numInBatch)
				log.Print(successfulResult(logline))

				respFile, respOK := respFiles[mode]
				firstFlag, flagOK := firstModel[mode]

				for k := 0; k < numInBatch; k++ {
					res := <-results
					if res.success {
						successCounts[mode][res.modelName]++
					}

					// Stream response to file immediately
					if respOK && flagOK {
						if err := streamResponseToFile(respFile, res.modelName, res.rawResponse, firstFlag); err != nil {
							log.Printf("ERROR: Failed to stream response for %s (mode %s): %v", res.modelName, mode, err)
						}
					} else {
						log.Printf("WARNING: Missing response writer for mode %s; skipping stream for model %s", mode, res.modelName)
					}

					logline = fmt.Sprintf("[Iter %d | Mode %s | Batch %d] Collected result %d/%d (Model: %s, Success: %t)", i+1, mode, (j/numHosts)+1, k+1, numInBatch, res.modelName, res.success)

					if res.success {
						log.Print(successfulResult(logline))
					} else {
						log.Print(failedResult(logline))
					}

				}
				logline = fmt.Sprintf("[Iter %d | Mode %s | Batch %d] All results for this batch collected.", i+1, mode, (j/numHosts)+1)
				log.Print(successfulResult(logline))

				// 5. Unload models *after* the batch is complete.
				logline = fmt.Sprintf("[Iter %d | Mode %s | Batch %d] Batch complete. Unloading models...", i+1, mode, (j/numHosts)+1)
				log.Print(successfulResult(logline))

				models.UnloadModels(&loadedCfg)
				logline = fmt.Sprintf("[Iter %d | Mode %s | Batch %d] Model unload complete. Moving to next batch.", i+1, mode, (j/numHosts)+1)
				log.Print(successfulResult(logline))
			}
		}

		logline := fmt.Sprintf("--- Finished Iteration %d/%d ---", i+1, interactionCount)
		log.Print(successfulResult(logline))
		log.Println("----------------------------------------")
	} // End of iteration loop

	// Close the JSON object in response files
	for mode, file := range respFiles {
		if _, err := file.WriteString("\n}\n"); err != nil {
			log.Printf("ERROR: Failed to close JSON in response file for %s: %v", mode, err)
		}
	}

	// 6. Process and display final results.
	log.Println("All iterations complete. Generating final report...")
	finalReport := processResults(successCounts, interactionCount)

	// Print and save the summary report
	printReport(finalReport, interactionCount)
	saveReport(finalReport, outputReportFile)

	log.Println("Script finished.")
}

// --- Worker Function ---

// worker represents a single concurrent goroutine tied to a specific host.
func worker(id int, hostURL string, jobs <-chan job, results chan<- result) {
	// Create a single HTTP client for this worker.
	client := &http.Client{
		Timeout: 600 * time.Second,
	}

	log.Printf("[Worker %d | Host %s] Worker started. Waiting for jobs...", id, hostURL)
	for j := range jobs {
		config, hasConfig := payloadConfigs[j.mode]
		expectedTool := "unknown"
		prompt := ""
		if hasConfig {
			expectedTool = config.expectedTool
			prompt = config.prompt
		} else {
			log.Printf("[Worker %d | Host %s] WARNING: No payload config for mode %s", id, hostURL, j.mode)
		}

		log.Printf("[Worker %d | Host %s] Starting job for model %s (mode %s, expected tool: %s)", id, hostURL, j.modelName, j.mode, expectedTool)
		if prompt != "" {
			log.Printf("[Worker %d | Host %s] User prompt for model %s (mode %s): %q", id, hostURL, j.modelName, j.mode, prompt)
		}

		// Create the specific payload for this model.
		payloadBytes, err := buildPayload(j.mode, j.modelName)
		if err != nil {
			log.Printf("[Worker %d | Host %s] ERROR creating payload for %s (mode %s): %v", id, hostURL, j.modelName, j.mode, err)
			errJSON := createErrorJSON(fmt.Sprintf("PAYLOAD BUILD ERROR: %v", err))
			results <- result{modelName: j.modelName, success: false, rawResponse: errJSON, mode: j.mode}
			continue
		}
		log.Printf("[Worker %d | Host %s] Payload built for %s (mode %s).", id, hostURL, j.modelName, j.mode)

		// Pretty-print payloadBytes by reading the io.Reader, then recreate a new reader for the HTTP request.
		payloadData, err := io.ReadAll(payloadBytes)
		if err != nil {
			log.Printf("[Worker %d | Host %s] ERROR reading payload for %s (mode %s): %v", id, hostURL, j.modelName, j.mode, err)
			errJSON := createErrorJSON(fmt.Sprintf("PAYLOAD READ ERROR: %v", err))
			results <- result{modelName: j.modelName, success: false, rawResponse: errJSON, mode: j.mode}
			continue
		}

		// Pretty print payload as JSON text
		fmt.Println(string(payloadData))

		// Recreate the payload reader for the HTTP request
		payloadBytes = bytes.NewBuffer(payloadData)

		// Make the HTTP POST request.
		log.Printf("[Worker %d | Host %s] Sending POST request for %s (mode %s)...", id, hostURL, j.modelName, j.mode)
		resp, err := client.Post(hostURL+"/api/chat", "application/json", payloadBytes)
		if err != nil {
			log.Printf("[Worker %d | Host %s] ERROR during request for %s (mode %s): %v", id, hostURL, j.modelName, j.mode, err)
			errJSON := createErrorJSON(fmt.Sprintf("HTTP POST ERROR: %v", err))
			results <- result{modelName: j.modelName, success: false, rawResponse: errJSON, mode: j.mode}
			continue
		}
		log.Printf("[Worker %d | Host %s] Received response for %s (mode %s, Status: %s).", id, hostURL, j.modelName, j.mode, resp.Status)

		// Read the response body.
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			log.Printf("[Worker %d | Host %s] ERROR reading body for %s (mode %s): %v", id, hostURL, j.modelName, j.mode, err)
			errJSON := createErrorJSON(fmt.Sprintf("HTTP BODY READ ERROR: %v", err))
			results <- result{modelName: j.modelName, success: false, rawResponse: errJSON, mode: j.mode}
			continue
		}

		// Check for non-200 OK status codes and wrap response
		if resp.StatusCode != http.StatusOK {
			log.Printf("[Worker %d | Host %s] ERROR: Server returned non-200 status (%s) for %s (mode %s).", id, hostURL, resp.Status, j.modelName, j.mode)
			errJSON := createErrorJSON(fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body)))
			results <- result{modelName: j.modelName, success: false, rawResponse: errJSON, mode: j.mode}
			continue
		}

		log.Printf("[Worker %d | Host %s] Response body read for %s (mode %s, %d bytes).", id, hostURL, j.modelName, j.mode, len(body))

		// Check if the response meets the success criteria.
		isSuccess := checkSuccess(body, j.modelName, j.mode, id, hostURL)

		log.Printf("[Worker %d | Host %s] Finished job for model %s (mode %s, Success: %t)", id, hostURL, j.modelName, j.mode, isSuccess)
		results <- result{modelName: j.modelName, success: isSuccess, rawResponse: body, mode: j.mode}
	}
	log.Printf("[Worker %d | Host %s] No more jobs. Worker shutting down.", id, hostURL)
}

// --- Helper Functions ---

// buildPayload creates the JSON payload for a specific model.
func buildPayload(mode payloadMode, modelName string) (io.Reader, error) {
	switch mode {
	case payloadModeWeather:
		payload := fmt.Sprintf(PAYLOAD_TEMPLATE_CURRENT_WEATHER, modelName)
		return bytes.NewBuffer([]byte(payload)), nil
	case payloadModeTime:
		payload := fmt.Sprintf(PAYLOAD_TEMPLATE_CURRENT_TIME, modelName)
		return bytes.NewBuffer([]byte(payload)), nil
	default:
		return nil, fmt.Errorf("unsupported payload mode: %s", mode)
	}
}

// createErrorJSON marshals an error string into a valid json.RawMessage
func createErrorJSON(errMsg string) json.RawMessage {
	errResp := struct {
		Error string `json:"error"`
	}{
		Error: errMsg,
	}
	raw, _ := json.Marshal(errResp)
	return raw
}

// isValidLocation checks if a location string matches Portland, OR
func isValidLocation(location interface{}) bool {
	if location == nil {
		return false
	}

	locStr, ok := location.(string)
	if !ok {
		return false
	}

	// Accept "Portland, OR" in various forms (only "location" field is valid, not "city")
	normalized := strings.TrimSpace(locStr)
	return normalized == "Portland, OR" ||
		normalized == "Portland,OR" ||
		normalized == "Portland, Oregon"
}

// extractJSONFromContent attempts to find and extract valid JSON objects from text
func extractJSONFromContent(content string) []string {
	var jsonStrings []string

	// Limit search to prevent hanging on large content
	if len(content) > 10000 {
		content = content[:10000]
	}

	// Find all positions where { appears
	for i := 0; i < len(content); i++ {
		if content[i] == '{' {
			// Try to extract a valid JSON object starting here
			// Use a simple brace counter to find the matching closing brace
			braceCount := 0
			inString := false
			escaped := false

			for j := i; j < len(content) && j < i+1000; j++ {
				char := content[j]

				// Handle escape sequences in strings
				if escaped {
					escaped = false
					continue
				}

				if char == '\\' {
					escaped = true
					continue
				}

				// Track whether we're inside a string
				if char == '"' {
					inString = !inString
					continue
				}

				// Only count braces outside of strings
				if !inString {
					if char == '{' {
						braceCount++
					} else if char == '}' {
						braceCount--

						// Found a complete JSON object
						if braceCount == 0 {
							candidate := content[i : j+1]

							// Verify it's valid JSON
							var test map[string]interface{}
							if err := json.Unmarshal([]byte(candidate), &test); err == nil {
								// Only include if it has name or arguments fields
								if _, hasName := test["name"]; hasName {
									jsonStrings = append(jsonStrings, candidate)
								} else if _, hasArgs := test["arguments"]; hasArgs {
									jsonStrings = append(jsonStrings, candidate)
								}
							}

							break // Move to next potential JSON object
						}
					}
				}
			}
		}

		// Safety limit: stop after finding 100 JSON objects
		if len(jsonStrings) >= 100 {
			break
		}
	}

	return jsonStrings
}

// checkSuccess implements the two success criteria with improved validation.
func checkSuccess(body []byte, modelName string, mode payloadMode, workerID int, hostURL string) bool {
	validation := getToolValidation(mode)
	var resp apiResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		log.Printf("[Worker %d | Host %s] Failed to unmarshal JSON for %s (mode %s). Body: %s", workerID, hostURL, modelName, mode, string(body))
		return false
	}

	// Criterion 1: Check for a valid standard tool call with safer field access
	if len(resp.Message.ToolCalls) > 0 {
		for _, tc := range resp.Message.ToolCalls {
			// Must call the correct tool: current_weather (not current_time)
			if tc.Function.Name != "" {
				log.Printf("[Worker %d | Host %s] Observed tool_calls entry for %s (mode %s): %s", workerID, hostURL, modelName, mode, tc.Function.Name)
			}
			if tc.Function.Name == validation.expectedName {
				if validation.validateArgs(tc.Function.Arguments) {
					log.Printf("[Worker %d | Host %s] SUCCESS for %s via Criterion 1 (%s).", workerID, hostURL, modelName, validation.criterion1Label)
					log.Printf("[Worker %d | Host %s] Actual tool used for %s (mode %s): %s", workerID, hostURL, modelName, mode, tc.Function.Name)
					return true
				}
			} else if tc.Function.Name != "" {
				log.Printf("[Worker %d | Host %s] WRONG TOOL for %s (mode %s): Called %s instead of %s", workerID, hostURL, modelName, mode, tc.Function.Name, validation.expectedName)
			}
		}
	} else {
		log.Printf("[Worker %d | Host %s] No direct tool_calls present for %s (mode %s); checking message content.", workerID, hostURL, modelName, mode)
	}

	// Criterion 2: Check for parsable JSON within the "content" field
	if resp.Message.Content != "" {
		log.Printf("[Worker %d | Host %s] Checking content field for embedded JSON for %s (mode %s, content length: %d)", workerID, hostURL, modelName, mode, len(resp.Message.Content))

		// Extract potential JSON objects from the content
		jsonStrings := extractJSONFromContent(resp.Message.Content)
		log.Printf("[Worker %d | Host %s] Found %d potential JSON objects in content for %s (mode %s)", workerID, hostURL, len(jsonStrings), modelName, mode)

		// Try each extracted JSON string
		for i, jsonStr := range jsonStrings {
			var embeddedCall embeddedToolCall
			if err := json.Unmarshal([]byte(jsonStr), &embeddedCall); err == nil {
				log.Printf("[Worker %d | Host %s] Parsed embedded JSON %d for %s (mode %s): name=%s", workerID, hostURL, i+1, modelName, mode, embeddedCall.Name)
				// Successfully decoded a JSON object, now validate it
				if embeddedCall.Name == validation.expectedName {
					if validation.validateArgs(embeddedCall.Arguments) {
						log.Printf("[Worker %d | Host %s] SUCCESS for %s via Criterion 2 (%s).", workerID, hostURL, modelName, validation.criterion2Label)
						log.Printf("[Worker %d | Host %s] Actual tool used via embedded JSON for %s (mode %s): %s", workerID, hostURL, modelName, mode, embeddedCall.Name)
						return true
					}
				} else if embeddedCall.Name != "" {
					log.Printf("[Worker %d | Host %s] WRONG TOOL for %s (mode %s): Embedded call to %s instead of %s", workerID, hostURL, modelName, mode, embeddedCall.Name, validation.expectedName)
				}
			}
		}
	}

	log.Printf("[Worker %d | Host %s] FAILURE for %s (mode %s). Expected tool %s was not invoked.", workerID, hostURL, modelName, mode, validation.expectedName)
	return false
}

// processResults calculates percentages and builds the final report map.
func processResults(counts map[payloadMode]map[string]int, totalInteractions int) map[payloadMode]map[string]interface{} {
	log.Println("Processing final results...")
	report := make(map[payloadMode]map[string]interface{})
	for mode, modelCounts := range counts {
		modeReport := make(map[string]interface{})
		for model, count := range modelCounts {
			var percent float64
			if totalInteractions > 0 {
				percent = (float64(count) / float64(totalInteractions)) * 100.0
			}
			modeReport[model] = map[string]interface{}{
				"success_count":   count,
				"percent_success": percent,
				"total_runs":      totalInteractions,
			}
		}
		report[mode] = modeReport
	}
	log.Println("Final results processed.")
	return report
}

// printReport prints the final formatted results to the console.
func printReport(report map[payloadMode]map[string]interface{}, totalInteractions int) {
	fmt.Println("\n--- FINAL REPORT ---")
	fmt.Printf("Based on %d interaction(s) per model.\n\n", totalInteractions)
	for mode, toolModels := range report {
		fmt.Printf("Mode: %s\n", mode)
		for model, raw := range toolModels {
			stats := raw.(map[string]interface{})
			count := stats["success_count"].(int)
			percent := stats["percent_success"].(float64)

			fmt.Printf("  %s: %d (%.2f%% success)\n", model, count, percent)
		}
		fmt.Println()
	}
	fmt.Println("--------------------")
}

// saveReport saves the final summary report map to a JSON file.
func saveReport(report map[payloadMode]map[string]interface{}, filename string) {
	dir := getDir(filename)
	if dir != "." {
		log.Printf("Ensuring report directory exists: %s", dir)
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Fatalf("Failed to create directories for report: %v", err)
		}
	}

	log.Println("Marshalling final report to JSON...")
	fileData, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal final report to JSON: %v", err)
	}

	log.Printf("Writing final report to %s...", filename)
	if err := os.WriteFile(filename, fileData, 0644); err != nil {
		log.Fatalf("Failed to write final report to %s: %v", filename, err)
	}

	log.Printf("Successfully saved report to %s", filename)
}

// createResponseFile creates and opens the response file for streaming writes
func createResponseFile(filename string) (*os.File, error) {
	dir := getDir(filename)
	if dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create directory: %v", err)
		}
	}

	file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %v", err)
	}

	return file, nil
}

// streamResponseToFile writes a single response to the file immediately
func streamResponseToFile(file *os.File, modelName string, response json.RawMessage, firstModel *bool) error {
	// Add comma if not the first model
	if !*firstModel {
		if _, err := file.WriteString(",\n"); err != nil {
			return err
		}
	}
	*firstModel = false

	// Write the model name as key
	modelKeyJSON, err := json.Marshal(modelName)
	if err != nil {
		return err
	}

	if _, err := file.WriteString(fmt.Sprintf("  %s: ", string(modelKeyJSON))); err != nil {
		return err
	}

	// Write the response (already in JSON format)
	if _, err := file.Write(response); err != nil {
		return err
	}

	return nil
}

// truncateFile ensures a directory exists and writes placeholder content to a file.
func truncateFile(filename string, content string) {
	dir := getDir(filename)
	if dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Fatalf("Failed to create directory for %s: %v", filename, err)
		}
	}
	if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
		log.Fatalf("Failed to truncate %s: %v", filename, err)
	}
}

// getDir extracts the directory path from a full file path.
func getDir(path string) string {
	if i := lastIndex(path, "/"); i != -1 {
		return path[:i]
	}
	if i := lastIndex(path, "\\"); i != -1 {
		return path[:i]
	}
	return "."
}

// lastIndex finds the last occurrence of a separator in a string.
func lastIndex(s, sep string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == sep[0] {
			return i
		}
	}
	return -1
}

func getToolValidation(mode payloadMode) toolValidation {
	switch mode {
	case payloadModeTime:
		return toolValidation{
			expectedName:    "current_time",
			criterion1Label: "Standard Tool Call",
			criterion2Label: "Embedded JSON",
			validateArgs:    validateTimeArgs,
		}
	default:
		return toolValidation{
			expectedName:    "current_weather",
			criterion1Label: "Standard Tool Call",
			criterion2Label: "Embedded JSON",
			validateArgs:    validateWeatherArgs,
		}
	}
}

func validateWeatherArgs(args map[string]interface{}) bool {
	if args == nil {
		return false
	}

	if loc, ok := args["location"]; ok {
		return isValidLocation(loc)
	}

	return false
}

func validateTimeArgs(args map[string]interface{}) bool {
	if args == nil {
		return true
	}
	return len(args) == 0
}
