---
name: cyrene-stt
description: Speech-to-text via Cyrene Gateway /v1/audio/transcriptions using OpenAI Whisper / Deepgram.
---

# Cyrene Gateway — Speech-to-Text

Requires `CYRENE_URL` (and `CYRENE_KEY` if auth enabled).

## Endpoint

`POST $CYRENE_URL/v1/audio/transcriptions` (multipart/form-data)

| Field | Required | Notes |
|---|---|---|
| `model` | yes | e.g. `openai/whisper-1`, `deepgram/nova-2` |
| `file` | yes | audio file (mp3, wav, m4a, ...) |
| `language` | no | ISO 639-1 code |

## Example

```bash
curl -X POST $CYRENE_URL/v1/audio/transcriptions \
  -H "Authorization: Bearer $CYRENE_KEY" \
  -F "model=openai/whisper-1" \
  -F "file=@audio.mp3"
```
