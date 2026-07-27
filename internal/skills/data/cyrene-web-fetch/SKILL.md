---
name: cyrene-web-fetch
description: Fetch URL content as markdown via Cyrene Gateway /v1/web/fetch using Jina / Firecrawl providers.
---

# Cyrene Gateway — Web Fetch

Requires `CYRENE_URL` (and `CYRENE_KEY` if auth enabled).

## Endpoint

`POST $CYRENE_URL/v1/web/fetch`

| Field | Required | Notes |
|---|---|---|
| `model` | yes | e.g. `jina/reader`, `firecrawl/scrape` |
| `url` | yes | URL to fetch |

## Example

```bash
curl -X POST $CYRENE_URL/v1/web/fetch \
  -H "Authorization: Bearer $CYRENE_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"jina/reader","url":"https://example.com"}'
```
