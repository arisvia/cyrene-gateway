---
name: cyrene-image
description: Generate images via Cyrene Gateway /v1/images/generations using OpenAI / Gemini / FLUX / Stability models.
---

# Cyrene Gateway — Image Generation

Requires `CYRENE_URL` (and `CYRENE_KEY` if auth enabled).

## Endpoint

`POST $CYRENE_URL/v1/images/generations`

| Field | Required | Notes |
|---|---|---|
| `model` | yes | e.g. `openai/dall-e-3`, `bfl/flux-pro` |
| `prompt` | yes | image description |
| `n` | no | count |
| `size` | no | `1024x1024`, `1792x1024`, ... |
| `response_format` | no | `url` (default) or `b64_json` |

## Example

```bash
curl -X POST $CYRENE_URL/v1/images/generations \
  -H "Authorization: Bearer $CYRENE_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"openai/dall-e-3","prompt":"A cat in a hat","size":"1024x1024"}'
```
