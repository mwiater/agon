#!/bin/bash

# --- Configuration ---
# Set your hosts
HOST1="host01"
HOST2="host02"
HOST3="host03"
HOST4="host04"

# Set your models
MODEL1="phi4-mini:3.8b"
MODEL2="granite3.1-moe:1b"
MODEL3="granite3.1-moe:3b"
MODEL4="granite4:micro"

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
# --- End of Configuration ---

# Clear screen and unload models sequentially
clear && \
./dist/agon_windows_amd64_v1/agon.exe unload models

# Create the specific JSON payloads for each request
printf -v PAYLOAD1 "$PAYLOAD_TEMPLATE" "$MODEL1"
printf -v PAYLOAD2 "$PAYLOAD_TEMPLATE" "$MODEL2"
printf -v PAYLOAD3 "$PAYLOAD_TEMPLATE" "$MODEL3"
printf -v PAYLOAD4 "$PAYLOAD_TEMPLATE" "$MODEL4"

echo "Sending 4 requests in parallel..."

# Run all 4 curl commands in the background (&)
# and save their output to temporary files.
curl $HOST1/api/chat -X POST -d "$PAYLOAD1" > out1.json &
curl $HOST2/api/chat -X POST -d "$PAYLOAD2" > out2.json &
curl $HOST3/api/chat -X POST -d "$PAYLOAD3" > out3.json &
curl $HOST4/api/chat -X POST -d "$PAYLOAD4" > out4.json &

# Wait for all background jobs to complete
wait

echo "All requests complete. Displaying results:"

# Now, print the results sequentially and formatted
echo "--- Results for $MODEL1 ($HOST1) ---"
jq . out1.json

echo "--- Results for $MODEL2 ($HOST2) ---"
jq . out2.json

echo "--- Results for $MODEL3 ($HOST3) ---"
jq . out3.json

echo "--- Results for $MODEL4 ($HOST4) ---"
jq . out4.json

# Clean up the temporary files
rm out1.json out2.json out3.json out4.json