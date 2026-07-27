---
name: cyrene-embeddings
description: Text embeddings via Cyrene Gateway /v1/embeddings using OpenAI / Voyage / Jina / NVIDIA NIM models.
---

# Cyrene Gateway — Embeddings

Requires `CYRENE_URL` (and `CYRENE_KEY` if auth enabled).

## Endpoint

`POST $CYRENE_URL/v1/embeddings`

| Field | Required | Notes |
|---|---|---|
| `model` | yes | e.g. `openai/text-embedding-3-small`, `voyage/voyage-3` |
| `input` | yes | string or array of strings |

## Example

```bash
curl -X POST $CYRENE_URL/v1/embeddings \
  -H "Authorization: Bearer $CYRENE_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"openai/text-embedding-3-small","input":"Hello world"}'
```

## Response

```json
{ "object": "list", "data": [{ "object": "embedding", "index": 0, "embedding": [0.1, ...] }],
  "usage": { "prompt_tokens": 2, "total_tokens": 2 } }
```
