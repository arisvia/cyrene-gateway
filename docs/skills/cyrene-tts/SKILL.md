---
name: cyrene-tts
description: Text-to-speech via Cyrene Gateway /v1/audio/speech using OpenAI / ElevenLabs / Deepgram voices.
---

# Cyrene Gateway — Text-to-Speech

Requires `CYRENE_URL` (and `CYRENE_KEY` if auth enabled).

## Endpoint

`POST $CYRENE_URL/v1/audio/speech`

| Field | Required | Notes |
|---|---|---|
| `model` | yes | e.g. `openai/tts-1`, `el/eleven_multilingual_v2` |
| `input` | yes | text to speak |
| `voice` | no | voice name (provider-dependent) |
| `response_format` | no | `mp3` (default), `wav`, `opus` |

## Example

```bash
curl -X POST $CYRENE_URL/v1/audio/speech \
  -H "Authorization: Bearer $CYRENE_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"openai/tts-1","input":"Hello world","voice":"alloy"}' \
  -o output.mp3
```
