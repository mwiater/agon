#!/bin/bash

# --- Configuration ---
# Set your hosts
# Use an array for easy scaling
HOSTS=(
  "{host-01}"
  "{host-02}"
  "{host-03}"
  "{host-04}"
)

# Create short names for file outputs, corresponding to the HOSTS array
HOST_NAMES=(
  "host1"
  "host2"
  "host3"
  "host4"
)

# Set your models
# Use an array to easily manage the list
MODELS=(
  "granite3.1-moe:1b"
  "granite3.1-moe:3b"
  "granite4:micro"
  "llama3.2:1b"
  "llama3.2:3b"
  "phi4-mini:3.8b"
  "qwen3:1.7b"
)

# Define the JSON payload template.
# The '%s' is a placeholder for the model name.
PAYLOAD_TEMPLATE='{
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
}'

# Define the output directory
OUTPUT_DIR="mcp/curls"

# --- End of Configuration ---

# --- Script Logic ---
clear

# Get the total number of models and hosts
NUM_MODELS=${#MODELS[@]}
NUM_HOSTS=${#HOSTS[@]} # This is our batch size

echo "Starting request runner..."
echo "Total Models: $NUM_MODELS"
echo "Parallel Hosts (Batch Size): $NUM_HOSTS"
echo ""

# --- Pre-run Cleanup ---
echo "Preparing output directory: $OUTPUT_DIR"
# Ensure the output directory exists
mkdir -p "$OUTPUT_DIR"

# Find and delete existing json files, logging each one
# Use find to handle cases with no files gracefully
echo "Cleaning up existing .json files in $OUTPUT_DIR..."
find "$OUTPUT_DIR" -name "*.json" -print -exec rm {} \;

# Check if any files were deleted (this is a bit tricky, find returns 0 even if no files)
# A simple message to confirm completion is usually enough.
echo "Cleanup complete. Directory is ready."
echo ""
# --- End of Cleanup ---


# Loop through the models in batches of $NUM_HOSTS
for (( i=0; i<$NUM_MODELS; i+=$NUM_HOSTS )); do

  echo "----------------------------------------"
  echo "Processing batch starting at model $((i+1))..."
  echo "----------------------------------------"

  # 1. Unload models before starting the batch
  echo "Unloading models..."
  ./dist/agon_linux_amd64_v1/agon unload models

  echo ""

  # 2. Start parallel requests for the current batch
  echo "Sending $NUM_HOSTS parallel requests (or fewer for last batch)..."
  
  # This inner loop runs for each host (0, 1, 2, 3)
  for (( j=0; j<$NUM_HOSTS; j++ )); do
    
    # Calculate the model index
    MODEL_INDEX=$((i+j))

    # Check if this model index exists
    if (( MODEL_INDEX < $NUM_MODELS )); then
      
      # Get the specific model, host, and host_name for this request
      MODEL=${MODELS[$MODEL_INDEX]}
      HOST=${HOSTS[$j]}
      HOST_NAME=${HOST_NAMES[$j]}

      # Create the output filename in the specified directory
      OUTPUT_FILE="${OUTPUT_DIR}/${MODEL}-${HOST_NAME}.json"

      # Generate the specific payload for this model
      printf -v PAYLOAD "$PAYLOAD_TEMPLATE" "$MODEL"

      # Run curl in the background (&) and save to the named file
      echo "  [STARTING] Request for $MODEL on $HOST. Output: $OUTPUT_FILE"
      curl "$HOST/api/chat" -X POST -d "$PAYLOAD" > "$OUTPUT_FILE" 2>/dev/null &

    fi
  done # End of inner loop for starting jobs

  # 3. Wait for all background jobs in this batch to complete
  echo ""
  echo "Waiting for all requests in this batch to complete..."
  wait
  echo "Batch complete."
  echo ""

  # 4. Display results for the completed batch
  echo "Displaying results for this batch:"
  
  # Loop back through the batch to display results sequentially
  for (( j=0; j<$NUM_HOSTS; j++ )); do
    MODEL_INDEX=$((i+j))

    if (( MODEL_INDEX < $NUM_MODELS )); then
      MODEL=${MODELS[$MODEL_INDEX]}
      HOST=${HOSTS[$j]}
      HOST_NAME=${HOST_NAMES[$j]}
      OUTPUT_FILE="${OUTPUT_DIR}/${MODEL}-${HOST_NAME}.json"

      echo ""
      echo "--- Results for $MODEL ($HOST) ---"
      if [ -f "$OUTPUT_FILE" ]; then
        jq . "$OUTPUT_FILE"
      else
        echo "ERROR: Output file $OUTPUT_FILE not found."
      fi
    fi
  done # End of inner loop for displaying results

  echo ""
  echo "----------------------------------------"

done # End of main loop

echo "All batches processed."
# Note: You can re-enable the 'rm' command here if you want to clean up
# echo "Cleaning up JSON files..."
# find "$OUTPUT_DIR" -name "*.json" -exec rm {} \;

