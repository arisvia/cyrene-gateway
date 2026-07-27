---
name: cyrene
description: Entry point for Cyrene Gateway — local/remote AI gateway with OpenAI-compatible REST for chat, image, TTS, embeddings, web search, web fetch. Use when the user mentions Cyrene Gateway, CYRENE_URL, or wants AI without writing provider boilerplate. This skill covers setup + indexes capability skills.
---

# Cyrene Gateway

Local/remote AI gateway exposing OpenAI-compatible REST. One key, many providers, auto-fallback.

## Setup

```bash
export CYRENE_URL="http://localhost:20128"      # or VPS / tunnel URL
export CYRENE_KEY="sk-..."                      # from Dashboard → Keys (only if requireApiKey=true)
```

All requests: `${CYRENE_URL}/v1/...` with header `Authorization: Bearer ${CYRENE_KEY}` (omit if auth disabled).

Verify: `curl $CYRENE_URL/api/health` → `{"ok":true}`

## Discover models

```bash
curl $CYRENE_URL/v1/models                  # chat/LLM (default)
curl $CYRENE_URL/v1/models/image            # image-gen
curl $CYRENE_URL/v1/models/tts              # text-to-speech
curl $CYRENE_URL/v1/models/embedding        # embeddings
curl $CYRENE_URL/v1/models/web              # web search + fetch
curl $CYRENE_URL/v1/models/stt              # speech-to-text
```

Use `data[].id` as `model` field in requests. Combos appear with `owned_by:"combo"`.

## Capability skills

| Capability | Description |
|---|---|
| Chat / code-gen | OpenAI /v1/chat/completions or Anthropic /v1/messages |
| Image generation | /v1/images/generations |
| Text-to-speech | /v1/audio/speech |
| Speech-to-text | /v1/audio/transcriptions |
| Embeddings | /v1/embeddings |
| Web search | /v1/search |
| Web fetch | /v1/web/fetch |

## Errors

- 401 → set/refresh `CYRENE_KEY` (Dashboard → Keys)
- 400 `Invalid model format` → check `model` exists in `/v1/models`
