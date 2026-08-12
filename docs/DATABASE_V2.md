# Database V2 Design

项目尚未投产，因此数据库可以一次性重建，不提供旧 schema 迁移或兼容读取。Phase 37F 实施时允许删除旧数据库，由应用创建 V2 schema。

## 当前问题

1. `migrate()` 只是逐条 `CREATE TABLE IF NOT EXISTS`，最后无条件写 `schema_version=1`，不具备真正版本迁移能力。
2. `providerConnections.data` 将 credential、状态、quota 和 provider-specific metadata 混在一个 JSON 中，任何字段更新都可能覆盖其他并发更新。
3. Provider API key、OAuth access/refresh token 和本地 API key 以可直接使用的值存入数据库。
4. `usageHistory.apiKey` 会记录客户端 API key，违反最小化原则。
5. usage/requestDetails 缺少主要查询索引，数据量增长后分页、时间范围和 connection 查询会退化。
6. 时间使用 RFC3339 TEXT，格式可读但范围查询、排序和统计不如统一 INTEGER epoch milliseconds 简单。
7. settings 是单行大 JSON，局部更新存在读改写覆盖风险，也难以约束敏感字段。
8. Connection、健康状态、凭据和 Provider metadata 没有清晰生命周期边界。
9. 多个删除操作没有明确 FK 与级联策略。
10. 没有 retention、vacuum/checkpoint 和数据完整性检查策略。

## 设计原则

- Provider 是静态 catalog，不存用户 secret。
- Connection 是用户配置和路由实体。
- Credential 与 Connection 分表，API 永不直接读取后序列化 credential row。
- 高频变化的 runtime state 与低频配置分离，使用窄 SQL update。
- 请求日志只记录脱敏、必要、可设 retention 的 metadata。
- 所有时间统一为 UTC epoch milliseconds `INTEGER`。
- Boolean 使用 `INTEGER NOT NULL CHECK(value IN (0,1))`。
- JSON 只用于扩展性 metadata，并通过 `CHECK(json_valid(...))`。
- 所有 migration 在事务内执行；schema version 只在成功后推进。
- V2 首次实现不迁移旧数据，检测到 V1 时明确报错并引导删除/备份数据库。

## 建议 Schema

### meta

```sql
CREATE TABLE meta (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
) STRICT;
```

保存 `schema_version=2`、`created_at_ms` 和应用 build 信息。

### settings

非敏感设置采用键值行，避免单行 JSON 全量覆盖。

```sql
CREATE TABLE settings (
  key TEXT PRIMARY KEY,
  value_json TEXT NOT NULL CHECK (json_valid(value_json)),
  updated_at_ms INTEGER NOT NULL
) STRICT;
```

认证初始化状态独立保存，但不保存密码明文或可逆值。

### connections

```sql
CREATE TABLE connections (
  id TEXT PRIMARY KEY,
  provider_id TEXT NOT NULL,
  auth_method TEXT NOT NULL,
  name TEXT NOT NULL,
  email TEXT,
  region TEXT,
  priority INTEGER NOT NULL DEFAULT 0,
  enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0,1)),
  base_url TEXT,
  config_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(config_json)),
  created_at_ms INTEGER NOT NULL,
  updated_at_ms INTEGER NOT NULL
) STRICT;

CREATE INDEX idx_connections_provider_enabled_priority
  ON connections(provider_id, enabled, priority, created_at_ms);
```

`config_json` 只能放非敏感、低频、provider-specific 配置。

### connection_credentials

```sql
CREATE TABLE connection_credentials (
  connection_id TEXT PRIMARY KEY
    REFERENCES connections(id) ON DELETE CASCADE,
  kind TEXT NOT NULL,
  ciphertext BLOB NOT NULL,
  nonce BLOB NOT NULL,
  key_version INTEGER NOT NULL,
  hint TEXT,
  expires_at_ms INTEGER,
  updated_at_ms INTEGER NOT NULL
) STRICT;
```

Credential payload 在应用层采用 AEAD 加密。应用 master key 存于固定数据目录的 `0600` 文件，并支持 `key_version` 轮换。

这只能降低数据库文件单独泄漏的风险，不能抵御已控制本机账号且同时读取 master key 的攻击者。不要将其描述为主机被攻破后的完整保护。

本地客户端 API key 不应可逆保存：

```sql
CREATE TABLE client_api_keys (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  key_prefix TEXT NOT NULL,
  key_hash BLOB NOT NULL UNIQUE,
  enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0,1)),
  created_at_ms INTEGER NOT NULL,
  last_used_at_ms INTEGER
) STRICT;
```

只在创建时返回一次完整 key；验证时比较 hash。使用带应用 pepper 的 HMAC-SHA-256 即可，因为 key 是高熵随机 token，不是用户密码。

### connection_runtime_state

```sql
CREATE TABLE connection_runtime_state (
  connection_id TEXT PRIMARY KEY
    REFERENCES connections(id) ON DELETE CASCADE,
  health TEXT NOT NULL DEFAULT 'unknown',
  last_error_code TEXT,
  last_error_message TEXT,
  rate_limited_until_ms INTEGER,
  backoff_level INTEGER NOT NULL DEFAULT 0,
  consecutive_failures INTEGER NOT NULL DEFAULT 0,
  last_success_at_ms INTEGER,
  last_failure_at_ms INTEGER,
  latency_ms INTEGER,
  state_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(state_json)),
  updated_at_ms INTEGER NOT NULL
) STRICT;
```

Token refresh、健康检查和 routing update 使用窄字段更新，避免覆盖 credential 或 Connection 配置。

模型级锁可独立表：

```sql
CREATE TABLE connection_model_locks (
  connection_id TEXT NOT NULL REFERENCES connections(id) ON DELETE CASCADE,
  model_id TEXT NOT NULL,
  locked_until_ms INTEGER NOT NULL,
  reason TEXT,
  PRIMARY KEY (connection_id, model_id)
) STRICT;
```

### routing

```sql
CREATE TABLE aliases (
  alias TEXT PRIMARY KEY,
  target TEXT NOT NULL,
  updated_at_ms INTEGER NOT NULL
) STRICT;

CREATE TABLE combos (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  strategy TEXT NOT NULL CHECK (strategy IN ('fallback','round-robin')),
  created_at_ms INTEGER NOT NULL,
  updated_at_ms INTEGER NOT NULL
) STRICT;

CREATE TABLE combo_members (
  combo_id TEXT NOT NULL REFERENCES combos(id) ON DELETE CASCADE,
  position INTEGER NOT NULL,
  target TEXT NOT NULL,
  PRIMARY KEY (combo_id, position)
) STRICT;
```

不再把 Combo members 放在 JSON 字符串中。

### proxy pools

Proxy 与 endpoint secret 按 credential 同等处理。非敏感配置可留在 JSON，含用户名/密码的 proxy URL 必须拆出并加密。

### request_records

```sql
CREATE TABLE request_records (
  id TEXT PRIMARY KEY,
  started_at_ms INTEGER NOT NULL,
  completed_at_ms INTEGER,
  provider_id TEXT,
  connection_id TEXT REFERENCES connections(id) ON DELETE SET NULL,
  input_model TEXT NOT NULL,
  resolved_model TEXT,
  endpoint TEXT NOT NULL,
  status TEXT NOT NULL,
  error_code TEXT,
  http_status INTEGER,
  prompt_tokens INTEGER NOT NULL DEFAULT 0,
  completion_tokens INTEGER NOT NULL DEFAULT 0,
  cost_micros INTEGER NOT NULL DEFAULT 0,
  first_token_ms INTEGER,
  duration_ms INTEGER,
  route_trace_json TEXT CHECK (route_trace_json IS NULL OR json_valid(route_trace_json)),
  metadata_json TEXT CHECK (metadata_json IS NULL OR json_valid(metadata_json))
) STRICT;

CREATE INDEX idx_request_records_started ON request_records(started_at_ms DESC);
CREATE INDEX idx_request_records_connection_started ON request_records(connection_id, started_at_ms DESC);
CREATE INDEX idx_request_records_provider_started ON request_records(provider_id, started_at_ms DESC);
CREATE INDEX idx_request_records_status_started ON request_records(status, started_at_ms DESC);
```

禁止记录完整客户端 API key、Provider credential、Authorization、Cookie 或默认保存 prompt/response 正文。若未来增加内容采样，必须显式 opt-in、脱敏、限长并有 retention。

每日统计可作为派生缓存：

```sql
CREATE TABLE usage_daily (
  day_utc TEXT NOT NULL,
  provider_id TEXT NOT NULL,
  model_id TEXT NOT NULL,
  requests INTEGER NOT NULL,
  prompt_tokens INTEGER NOT NULL,
  completion_tokens INTEGER NOT NULL,
  cost_micros INTEGER NOT NULL,
  PRIMARY KEY(day_utc, provider_id, model_id)
) STRICT;
```

## DB API 结构

建议将 `internal/db` 拆为：

```text
internal/db/
  open.go
  migrations.go
  tx.go
  connections.go
  credentials.go
  runtime_state.go
  routing.go
  api_keys.go
  requests.go
  retention.go
```

- 不再公开 `Conn() *sql.DB`。
- 所有方法第一个参数使用 `context.Context`。
- 组合写操作通过 `WithTx(ctx, func(*Queries) error)`。
- Update 返回 `ErrNotFound`，检查 `RowsAffected`。
- List 使用稳定排序与 cursor/keyset pagination。
- `sql.DB` 的连接数由负载测试决定。SQLite WAL 下可保留单 writer，但不应默认把所有 read 永久限制为一个连接。

## SQLite 运行参数

启动时显式验证：

```text
foreign_keys=ON
journal_mode=WAL
busy_timeout=5000
synchronous=NORMAL
journal_size_limit
```

补充：

- 启动执行 `PRAGMA quick_check`，失败进入只读诊断或拒绝启动。
- 定期或关闭时做受控 WAL checkpoint。
- retention 删除后按阈值执行 incremental vacuum，不在请求路径执行全量 VACUUM。
- 设置数据库和 WAL 文件权限。

## Retention

默认建议：

- request records：30 天或 100,000 条，先达到者触发清理
- runtime logs：内存 ring buffer，不长期入库
- usage daily：长期保留
- OAuth in-flight session：只在内存，带 TTL，不写数据库

Retention 必须可在 Settings 中调整，并有 UI 显示预计空间。

## Phase 37F 验收

- [ ] V2 schema 在事务中创建，schema version 正确
- [ ] 发现 V1 时明确拒绝并给出重建提示，不静默误标版本
- [ ] Provider credential AEAD round-trip 与错误 key 测试
- [ ] Client API key 只保存 hash，创建后只返回一次完整 key
- [ ] usage/request record 不保存完整 API key 或 secret
- [ ] FK、cascade、unique、check 和主要索引测试通过
- [ ]并发窄更新不会覆盖 credential/runtime state
- [ ] retention、quick_check 和 WAL checkpoint 有测试或可验证接口
- [ ] 所有 repository 使用 context，业务层无法获取裸 `*sql.DB`
