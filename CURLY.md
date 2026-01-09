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

---

curl -X POST https://o-udoo03.0nezer0.com/models/load \
  -H "Content-Type: application/json" \
  -d '{"model":"gemma-2-2b-it-Q4_K_M"}'
  
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

---

clear && curl -X POST http://192.168.0.33:8888/models/load \
  -H "Content-Type: application/json" \
  -d '{"model":"DeepSeek-R1-Distill-Qwen-1.5B-Q4_K_M"}'

curl http://192.168.0.33:8888/v1/chat/completions \
-H "Content-Type: application/json" \
-d '{
  "model": "DeepSeek-R1-Distill-Qwen-1.5B-Q4_K_M",
  "messages": [
    {
      "role": "system",
      "content": "You are a numeric answer engine. You may reason inside <think> tags, but your final answer must be a single integer provided after the </think> tag."
    },
    {
      "role": "user",
      "content": "A clock shows 6:00. How many degrees are between the hour and minute hand? Respond with a single integer only."
    }
  ],
  "temperature": 0.4,
  "max_tokens": 512,
  "seed": 42
}'

Example Response:

```
{"choices":[{"finish_reason":"stop","index":0,"message":{"role":"assistant","reasoning_content":"Okay, so I have this problem where a clock shows 6:00, and I need to find out how many degrees are between the hour and minute hands. Hmm, let me think about how clocks work. I remember that a clock is a circle, which has 360 degrees. And there are 12 hours on a clock, so each hour represents 30 degrees because 360 divided by 12 is 30. \n\nAlright, so at 6:00, the minute hand is pointing straight up, which is 12 o'clock. That's 0 degrees from the top. Now, where is the hour hand? At 6:00, it's exactly at the 6, which is halfway around the clock. Since each hour is 30 degrees, halfway would be 180 degrees. \n\nSo, the minute hand is at 0 degrees, and the hour hand is at 180 degrees. To find the angle between them, I subtract the smaller angle from the larger one. That would be 180 minus 0, which is 180 degrees. \n\nWait, but clocks are circular, so sometimes people might think the angle could be the other way around. But since we're just looking for the smaller angle between the two hands, 180 degrees is correct. So, the answer should be 180.","content":"The angle between the hour and minute hands at 6:00 is calculated by recognizing that each hour represents 30 degrees (360 degrees divided by 12 hours). At 6:00, the hour hand is at 180 degrees, while the minute hand is at 0 degrees. The difference is 180 degrees. \n\n**Answer:** 180"}}],"created":1767973590,"model":"DeepSeek-R1-Distill-Qwen-1.5B-Q4_K_M","system_fingerprint":"b7634-f1768d8f0","object":"chat.completion","usage":{"completion_tokens":381,"prompt_tokens":63,"total_tokens":444},"id":"chatcmpl-apbKyhgpDWbByNkuUPEffVotX6P2aLsd","timings":{"cache_n":0,"prompt_n":63,"prompt_ms":4.817,"prompt_per_token_ms":0.07646031746031746,"prompt_per_second":13078.679676146978,"predicted_n":381,"predicted_ms":10707.933,"predicted_per_token_ms":28.104811023622048,"predicted_per_second":35.58109674388138}}
```

curl http://192.168.0.33:8888/v1/chat/completions \
-H "Content-Type: application/json" \
-d '{
  "model": "DeepSeek-R1-Distill-Qwen-1.5B-Q4_K_M",
  "messages": [
    {
      "role": "system",
      "content": "You are a strict numeric answer engine. Output ONLY one line containing a single integer. No spaces, no leading or trailing text, no punctuation, no commas, no units, no explanation, no extra lines, no markdown, and no code fences. If the answer is negative, use a leading minus sign. If unsure, still output one best-guess integer only."
    },
    {
      "role": "user",
      "content": "A clock shows 6:00. How many degrees are between the hour and minute hand? Respond with a single integer only."
    }
  ],
  "temperature": 0.1,
  "top_p": 0.4,
  "top_k": 10,
  "min_p": 0.1,
  "max_tokens": 16,
  "presence_penalty": 0.0,
  "frequency_penalty": 0.0,
  "repeat_penalty": 1.0,
  "seed": 42
}'

Example Response:

```
{"choices":[{"finish_reason":"length","index":0,"message":{"role":"assistant","reasoning_content":"First, I need to determine the positions of both the hour and minute hands on","content":""}}],"created":1767973602,"model":"DeepSeek-R1-Distill-Qwen-1.5B-Q4_K_M","system_fingerprint":"b7634-f1768d8f0","object":"chat.completion","usage":{"completion_tokens":16,"prompt_tokens":106,"total_tokens":122},"id":"chatcmpl-oLECLLoHmQWFuKx1UDdWT6HUt51B1tsu","timings":{"cache_n":0,"prompt_n":106,"prompt_ms":4.584,"prompt_per_token_ms":0.043245283018867924,"prompt_per_second":23123.9092495637,"predicted_n":16,"predicted_ms":756.933,"predicted_per_token_ms":47.3083125,"predicted_per_second":21.13793426895115}}
```