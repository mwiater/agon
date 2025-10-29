package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"time"
)

// --- Configuration ---

// Set the number of times to loop through the entire model list.
const interactionCount = 1

// Set the path to your agon CLI tool.
const agonCLIPath = "dist/agon_windows_amd64_v1/agon.exe"

// Set the output filename for the final JSON report.
const outputReportFile = "skunkworks/reports/model_tools_report.json"

// Set the log file path
const logFilePath = "skunkworks/logs/tool_tester.log"

// Set the raw responses output path
const responsesFilePath = "skunkworks/responses/responses.json"

// Set your hosts. The number of hosts determines the number of parallel requests.
var (
	hosts = []string{
		"host-01",
		"host-02",
		"host-03",
		"host-04",
	}

	// Set your models to be tested.
	models = []string{
		"granite3.1-moe:1b",
		"granite3.1-moe:3b",
		"granite4:micro",
		"llama3.2:1b",
		"llama3.2:3b",
		"phi4-mini:3.8b",
		"qwen3:1.7b",
	}
)

// PAYLOAD_TEMPLATE is the JSON payload sent to the API.
// The %s is a placeholder for the model name.
const PAYLOAD_TEMPLATE = `{
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
        "name": "get_current_weather",
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
    }
  ]
}`

// --- Structs for Parsing ---
// These structs are now *only* used by checkSuccess for validation.
// They are NOT used for storing the response.

// Struct for the primary JSON response.
type apiResponse struct {
	ToolCalls []toolCall `json:"tool_calls"`
	Content   string     `json:"content"`
}

// Struct for the standard tool_calls format.
type toolCall struct {
	Function struct {
		Name      string `json:"name"`
		Arguments struct {
			Location string `json:"location"`
		} `json:"arguments"`
	} `json:"function"`
}

// Struct for the embedded JSON in the content field.
type embeddedToolCall struct {
	Name      string `json:"name"`
	Arguments struct {
		Location string `json:"location"`
	} `json:"arguments"`
}

// We use a regex to find any JSON-like objects within the content string.
var jsonRegex = regexp.MustCompile(`\{.*\}`)

// --- Job/Worker Pool Structs ---

type job struct {
	modelName string
}

type result struct {
	modelName   string
	success     bool
	rawResponse json.RawMessage // The raw JSON response body
}

// --- Main Application ---

func main() {
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
	truncateFile(responsesFilePath, "{}")

	log.Println("Starting request runner...")
	log.Printf("Total Models: %d", len(models))
	log.Printf("Parallel Hosts (Batch Size): %d", len(hosts))
	log.Printf("Total Iterations: %d", interactionCount)
	log.Printf("Total Requests to be made: %d", len(models)*interactionCount)
	log.Println("----------------------------------------")

	// successCounts will store the total number of successful responses for each model.
	successCounts := make(map[string]int)
	for _, model := range models {
		successCounts[model] = 0
	}

	// allResponses will store raw JSON responses keyed by model name.
	allResponses := make(map[string][]json.RawMessage)
	for _, model := range models {
		allResponses[model] = make([]json.RawMessage, 0, interactionCount) // Pre-allocate slice
	}

	numHosts := len(hosts)

	// --- Initial Unload ---
	// Unload models once before the entire test run starts.
	log.Println("--- Initial Model Unload ---")
	runUnloadCommand()
	log.Println("----------------------------------------")

	// Main loop for n iterations.
	log.Printf("Beginning %d test iterations...", interactionCount)
	for i := 0; i < interactionCount; i++ {
		log.Printf("--- Starting Iteration %d/%d ---", i+1, interactionCount)

		// NEW: Loop through models in batches of numHosts
		totalBatches := (len(models) + numHosts - 1) / numHosts
		for j := 0; j < len(models); j += numHosts {
			// Determine the models for this specific batch
			batchEnd := j + numHosts
			if batchEnd > len(models) {
				batchEnd = len(models)
			}
			batchModels := models[j:batchEnd]
			numInBatch := len(batchModels)

			log.Printf("[Iter %d] Processing batch %d/%d (Models %d-%d)...", i+1, (j/numHosts)+1, totalBatches, j+1, batchEnd)

			// 1. Set up the worker pool channels for this batch.
			log.Printf("[Iter %d, Batch %d] Setting up job and result channels.", i+1, (j/numHosts)+1)
			jobs := make(chan job, numInBatch)
			results := make(chan result, numInBatch)

			// 2. Start workers (one per host).
			// Workers will process jobs from the queue and terminate when the channel is closed.
			log.Printf("[Iter %d, Batch %d] Starting %d workers...", i+1, (j/numHosts)+1, numHosts)
			for w := 0; w < numHosts; w++ {
				go worker(w, hosts[w], jobs, results)
			}

			// 3. Send all jobs (one per model) for this batch.
			log.Printf("[Iter %d, Batch %d] Sending %d jobs to workers...", i+1, (j/numHosts)+1, numInBatch)
			for _, model := range batchModels {
				jobs <- job{modelName: model}
			}
			close(jobs) // Signal that no more jobs will be sent for this batch.
			log.Printf("[Iter %d, Batch %d] All jobs sent. Channel closed.", i+1, (j/numHosts)+1)

			// 4. Collect all results for this batch.
			log.Printf("[Iter %d, Batch %d] Waiting to collect %d results...", i+1, (j/numHosts)+1, numInBatch)
			for k := 0; k < numInBatch; k++ {
				res := <-results
				if res.success {
					successCounts[res.modelName]++
				}
				// Always store the response.
				allResponses[res.modelName] = append(allResponses[res.modelName], res.rawResponse)

				log.Printf("[Iter %d, Batch %d] Collected result %d/%d (Model: %s, Success: %t)", i+1, (j/numHosts)+1, k+1, numInBatch, res.modelName, res.success)
			}
			log.Printf("[Iter %d, Batch %d] All results for this batch collected.", i+1, (j/numHosts)+1)

			// 5. Unload models *after* the batch is complete.
			log.Printf("[Iter %d, Batch %d] Batch complete. Unloading models...", i+1, (j/numHosts)+1)
			runUnloadCommand()
			log.Printf("[Iter %d, Batch %d] Model unload complete. Moving to next batch.", i+1, (j/numHosts)+1)

		} // End of batch loop

		log.Printf("--- Finished Iteration %d/%d ---", i+1, interactionCount)
		log.Println("----------------------------------------")
	} // End of iteration loop

	// 6. Process and display final results.
	log.Println("All iterations complete. Generating final report...")
	finalReport := processResults(successCounts, interactionCount)

	// Save all the raw responses we collected
	saveAllResponses(allResponses, responsesFilePath)

	// Print and save the summary report
	printReport(finalReport, interactionCount)
	saveReport(finalReport, outputReportFile)

	log.Println("Script finished.")
}

// --- Worker Function ---

// worker represents a single concurrent goroutine tied to a specific host.
// It will repeatedly pull jobs from the `jobs` channel until it's closed,
// process them, and send the outcome to the `results` channel.
func worker(id int, hostURL string, jobs <-chan job, results chan<- result) {
	// Create a single HTTP client for this worker.
	client := &http.Client{
		Timeout: 600 * time.Second, // Updated timeout (600 seconds)
	}

	// Range over the jobs channel. This loop will
	// automatically terminate when the channel is closed and empty.
	log.Printf("[Worker %d | Host %s] Worker started. Waiting for jobs...", id, hostURL)
	for j := range jobs {
		log.Printf("[Worker %d | Host %s] Starting job for model %s", id, hostURL, j.modelName)

		// Create the specific payload for this model.
		payloadBytes, err := buildPayload(j.modelName)
		if err != nil {
			log.Printf("[Worker %d | Host %s] ERROR creating payload for %s: %v", id, hostURL, j.modelName, err)
			errJSON := createErrorJSON(fmt.Sprintf("PAYLOAD BUILD ERROR: %v", err))
			results <- result{modelName: j.modelName, success: false, rawResponse: errJSON}
			continue
		}
		log.Printf("[Worker %d | Host %s] Payload built for %s.", id, hostURL, j.modelName)

		// Make the HTTP POST request.
		log.Printf("[Worker %d | Host %s] Sending POST request for %s...", id, hostURL, j.modelName)
		resp, err := client.Post(hostURL+"/api/chat", "application/json", payloadBytes)
		if err != nil {
			log.Printf("[Worker %d | Host %s] ERROR during request for %s: %v", id, hostURL, j.modelName, err)
			errJSON := createErrorJSON(fmt.Sprintf("HTTP POST ERROR: %v", err))
			results <- result{modelName: j.modelName, success: false, rawResponse: errJSON}
			continue
		}
		log.Printf("[Worker %d | Host %s] Received response for %s (Status: %s).", id, hostURL, j.modelName, resp.Status)

		// Read the response body.
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close() // Ensure body is closed.
		if err != nil {
			log.Printf("[Worker %d | Host %s] ERROR reading body for %s: %v", id, hostURL, j.modelName, err)
			errJSON := createErrorJSON(fmt.Sprintf("HTTP BODY READ ERROR: %v", err))
			results <- result{modelName: j.modelName, success: false, rawResponse: errJSON}
			continue
		}

		// Check for non-200 OK status codes
		if resp.StatusCode != http.StatusOK {
			log.Printf("[Worker %d | Host %s] ERROR: Server returned non-200 status (%s) for %s.", id, hostURL, resp.Status, j.modelName)
			// Pass the raw body. If it's valid JSON (like an Ollama error),
			// it will be saved as a JSON object. If it's not (like plain text),
			// it will be saved as a JSON string.
			results <- result{modelName: j.modelName, success: false, rawResponse: body}
			continue
		}

		log.Printf("[Worker %d | Host %s] Response body read for %s (%d bytes).", id, hostURL, j.modelName, len(body))

		// Check if the response meets the success criteria.
		isSuccess := checkSuccess(body, j.modelName, id, hostURL)

		// Send the result with the raw body.
		log.Printf("[Worker %d | Host %s] Finished job for model %s (Success: %t)", id, hostURL, j.modelName, isSuccess)
		results <- result{modelName: j.modelName, success: isSuccess, rawResponse: body}
	}
	log.Printf("[Worker %d | Host %s] No more jobs. Worker shutting down.", id, hostURL)
}

// --- Helper Functions ---

// buildPayload creates the JSON payload for a specific model.
func buildPayload(modelName string) (io.Reader, error) {
	payload := fmt.Sprintf(PAYLOAD_TEMPLATE, modelName)
	return bytes.NewBuffer([]byte(payload)), nil
}

// runUnloadCommand shells out to the agon CLI to unload models.
func runUnloadCommand() {
	log.Println("Attempting to unload models via agon CLI...")
	cmd := exec.Command(agonCLIPath, "unload", "models")

	// Capture combined stdout/stderr.
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("ERROR running agon unload: %v\nOutput: %s", err, string(output))
	} else {
		log.Println("Agon unload command successful.")
	}
}

// createErrorJSON marshals an error string into a valid json.RawMessage
func createErrorJSON(errMsg string) json.RawMessage {
	errResp := struct {
		Error string `json:"error"`
	}{
		Error: errMsg,
	}
	// This marshal will not fail on this simple struct.
	raw, _ := json.Marshal(errResp)
	return raw
}

// checkSuccess implements your two success criteria.
// It now returns: (isLogicSuccess)
func checkSuccess(body []byte, modelName string, workerID int, hostURL string) bool {
	var resp apiResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		// Not valid JSON at all.
		log.Printf("[Worker %d | Host %s] Failed to unmarshal JSON for %s. Body: %s", workerID, hostURL, modelName, string(body))
		return false
	}

	// At this point, parsing was successful.

	// Criterion 1: Check for a valid standard tool call.
	if len(resp.ToolCalls) > 0 {
		tc := resp.ToolCalls[0]
		if tc.Function.Name == "get_current_weather" && tc.Function.Arguments.Location == "Portland, OR" {
			log.Printf("[Worker %d | Host %s] SUCCESS for %s via Criterion 1 (Standard Tool Call).", workerID, hostURL, modelName)
			return true
		}
	}

	// Criterion 2: Check for parsable JSON within the "content" field.
	if resp.Content != "" {
		// Find all JSON-like strings in the content.
		matches := jsonRegex.FindAllString(resp.Content, -1)
		for _, match := range matches {
			var embeddedCall embeddedToolCall
			if err := json.Unmarshal([]byte(match), &embeddedCall); err == nil {
				// We found one that parses! Check if it's correct.
				if embeddedCall.Name == "get_current_weather" && embeddedCall.Arguments.Location == "Portland, OR" {
					log.Printf("[Worker %d | Host %s] SUCCESS for %s via Criterion 2 (Embedded JSON).", workerID, hostURL, modelName)
					return true
				}
			}
		}
	}

	// If neither criterion was met.
	log.Printf("[Worker %d | Host %s] FAILURE for %s. No valid tool call found.", workerID, hostURL, modelName)
	return false
}

// processResults calculates percentages and builds the final report map.
func processResults(counts map[string]int, totalInteractions int) map[string]interface{} {
	log.Println("Processing final results...")
	report := make(map[string]interface{})
	for model, count := range counts {
		var percent float64
		if totalInteractions > 0 {
			percent = (float64(count) / float64(totalInteractions)) * 100.0
		}
		report[model] = map[string]interface{}{
			"success_count":   count,
			"percent_success": percent,
			"total_runs":      totalInteractions,
		}
	}
	log.Println("Final results processed.")
	return report
}

// printReport prints the final formatted results to the console.
func printReport(report map[string]interface{}, totalInteractions int) {
	fmt.Println("\n--- FINAL REPORT ---")
	fmt.Printf("Based on %d interaction(s) per model.\n\n", totalInteractions)
	for model, data := range report {
		stats := data.(map[string]interface{})
		count := stats["success_count"].(int)
		percent := stats["percent_success"].(float64)

		fmt.Printf("%s: %d (%.2f%% success)\n", model, count, percent)
	}
	fmt.Println("--------------------")
}

// saveReport saves the final summary report map to a JSON file.
func saveReport(report map[string]interface{}, filename string) {
	// Ensure the directory exists before writing the file.
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

// saveAllResponses saves all captured API responses to a JSON file.
// The input map is now map[string][]json.RawMessage
func saveAllResponses(responses map[string][]json.RawMessage, filename string) {
	log.Printf("Saving all responses by model to %s...", filename)

	// Ensure the directory exists
	dir := getDir(filename)
	if dir != "." {
		log.Printf("Ensuring response directory exists: %s", dir)
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Fatalf("Failed to create directories for responses: %v", err)
		}
	}

	// Marshal the map of responses
	fileData, err := json.MarshalIndent(responses, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal all responses to JSON: %v", err)
	}

	// Write the file
	if err := os.WriteFile(filename, fileData, 0644); err != nil {
		log.Fatalf("Failed to write all responses to %s: %v", filename, err)
	}

	log.Printf("Successfully saved all responses to %s", filename)
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
	// Find the last separator.
	if i := lastIndex(path, "/"); i != -1 {
		return path[:i]
	}
	if i := lastIndex(path, "\\"); i != -1 {
		return path[:i]
	}
	// No separator found, must be in the current directory.
	return "."
}

// lastIndex finds the last occurrence of a separator in a string.
// We use this simple helper to avoid importing 'path/filepath' just for this.
func lastIndex(s, sep string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == sep[0] {
			return i
		}
	}
	return -1
}
