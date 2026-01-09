curl -X POST https://o-udoo03.0nezer0.com/models/load \
  -H "Content-Type: application/json" \
  -d '{"model":"gemma-2-2b-it-Q4_K_M"}'

curl -X POST https://o-udoo03.0nezer0.com/completion \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemma-2-2b-it-Q4_K_M",
    "prompt": "Hi.",
    "max_tokens": 64,
    "temperature": 0.7,
    "stream": false
  }'


  
curl -X POST https://o-udoo03.0nezer0.com/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemma-2-2b-it-Q4_K_M",
    "messages": [
      {
        "role": "user",
        "content": "Hi."
      }
    ],
    "max_tokens": 64,
    "temperature": 0.7,
    "stream": false
  }'

{
  "choices": [
    {
      "finish_reason": "stop",
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "Hi! 😊  How can I help you today? \n"
      }
    }
  ],
  "created": 1767919342,
  "model": "gemma-2-2b-it-Q4_K_M",
  "system_fingerprint": "b7634-f1768d8f0",
  "object": "chat.completion",
  "usage": {
    "completion_tokens": 14,
    "prompt_tokens": 11,
    "total_tokens": 25
  },
  "id": "chatcmpl-xgtB09W0CAf7hbpo0k0RtvshoSMHRha1",
  "timings": {
    "cache_n": 0,
    "prompt_n": 11,
    "prompt_ms": 4817.6,
    "prompt_per_token_ms": 437.9636363636364,
    "prompt_per_second": 2.2832945865161074,
    "predicted_n": 14,
    "predicted_ms": 8440.141,
    "predicted_per_token_ms": 602.8672142857142,
    "predicted_per_second": 1.6587400613330985
  }
}