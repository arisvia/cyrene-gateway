# Provider compatibility workflow

Provider upstreams change continuously. Updates must follow this sequence:

1. Capture a redacted request/response or SSE fixture.
2. Add a contract test that fails against the old adapter.
3. Update only the provider adapter or translator normalization layer.
4. Classify failures as auth_expired, quota_exhausted, endpoint_unreachable, stream_broken, schema_changed, or model_catalog_stale.
5. Record the upstream source, date, license, and commit/document revision.

Do not copy whole implementations from community routers. Use them as change signals, then verify against official provider behavior and local fixtures.
