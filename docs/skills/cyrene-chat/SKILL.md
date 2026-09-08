---
name: cyrene-chat
description: Chat / code generation via Cyrene Gateway using OpenAI /v1/chat/completions or Anthropic /v1/messages format with streaming + auto-fallback combos.
---

# Cyrene Gateway — Chat

Requires `CYRENE_URL` (and `CYRENE_KEY` if auth enabled).

## Endpoints

- `POST $CYRENE_URL/v1/chat/completions` — OpenAI format
- `POST $CYRENE_URL/v1/messages` — Anthropic format

## Discover

```bash
curl $CYRENE_URL/v1/models | jq '.data[].id'
```

Combos auto-fallback through multiple providers.

## OpenAI format

```bash
curl -X POST $CYRENE_URL/v1/chat/completions \
  -H "Authorization: Bearer $CYRENE_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"openai/gpt-4o","messages":[{"role":"user","content":"Hi"}],"stream":false}'
```

## Anthropic format

```bash
curl -X POST $CYRENE_URL/v1/messages \
  -H "Authorization: Bearer $CYRENE_KEY" \
  -H "anthropic-version: 2023-06-01" \
  -H "Content-Type: application/json" \
  -d '{"model":"anthropic/claude-sonnet-4-20250514","max_tokens":1024,"messages":[{"role":"user","content":"Hi"}]}'
```

## Response shape

OpenAI:
```json
{ "id": "chatcmpl-...", "object": "chat.completion", "model": "openai/gpt-4o",
  "choices": [{ "index": 0, "message": { "role": "assistant", "content": "Hello!" }, "finish_reason": "stop" }],
  "usage": { "prompt_tokens": 8, "completion_tokens": 2, "total_tokens": 10 } }
```

Streaming (`stream:true`) emits SSE: `data: {choices:[{delta:{content:"..."}}]}\n\n` ... `data: [DONE]\n\n`.
