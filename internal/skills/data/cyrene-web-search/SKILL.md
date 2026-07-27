---
name: cyrene-web-search
description: Web search via Cyrene Gateway /v1/search using Tavily / Exa / Brave providers.
---

# Cyrene Gateway — Web Search

Requires `CYRENE_URL` (and `CYRENE_KEY` if auth enabled).

## Endpoint

`POST $CYRENE_URL/v1/search`

| Field | Required | Notes |
|---|---|---|
| `model` | yes | e.g. `tavily/search`, `exa/search` |
| `query` | yes | search query |
| `max_results` | no | number of results (default 5) |

## Example

```bash
curl -X POST $CYRENE_URL/v1/search \
  -H "Authorization: Bearer $CYRENE_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"tavily/search","query":"latest Go release"}'
```
