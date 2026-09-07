# Cyrene Gateway

Cyrene Gateway 是一个自托管的 **LLM API 网关**:把众多 AI 提供商(OpenAI / Anthropic / Gemini / GitHub Copilot / Qoder 等数十家)统一收敛到一套 OpenAI 兼容 API 之后,并为它们补上连接池、故障回退(fallback)、配额冷却、用量统计与一块内置管理面板。

单二进制、零外部依赖(SQLite 内嵌)、Go + Solid.js 实现。

## 功能总览

- **统一 API 入口**:OpenAI 兼容(`/v1/chat/completions`、`/v1/embeddings`、`/v1/images/generations`、`/v1/audio/speech`、`/v1/models`、`/v1/responses`)与 Anthropic 兼容(`/v1/messages` 直通)。
- **全能力提供商市场**:
  - 覆盖 LLM 对话、图像生成、语音合成/识别、视频生成、文本向量与联网搜索/网页抓取 8 大能力标签;
  - 支持 API Key、OAuth(授权码 + PKCE、设备码、Token 导入)、免密公共提供商与 Web Cookie 接入;
  - 纯媒体服务(ElevenLabs、Stability AI、Tavily、Exa 等)与大模型上游在统一市场聚合与授权;
  - 多模态媒体工作台(Media)提供即时交互调试与图像结果图库。
- **思考推理能力映射**:下游标准化暴露 OpenAI `reasoning_effort`(`low`/`medium`/`high`)与 Anthropic `thinking`，自动映射至上游提供商(Gemini 3.8/3.7/3.6 Flash、Claude 3.7 Sonnet 等)。
- **组合与回退**:Combo(多模型组合)按 `fallback` / `round-robin` / sticky 策略轮转;错误分类规则(`ErrorRules`)驱动指数退避冷却(2s 起步、上限 5 分钟)与模型级锁定。
- **凭证调度**:`SelectCredentialWithQuota` 按 priority 排序、冷却状态、模型锁、配额上限挑选连接,OAuth 优先于 API Key。
- **全链路出站安全与 SSRF 防护**: 所有出站 HTTP 请求（上游直通、文本向量、后台模型探测、凭证测试与外部面板下载）统一经 `SafeHTTPClient` 强化，实施基于 dial-time IP 解析的私网/环回/链路本地/云元数据/CGNAT 地址阻断与防跳转逃逸(可用 `-allow-private-networks` 放开,仅限本地测试)。
- **令牌节省**:RTK 压缩、Caveman、Ponytail 三档 token saver,支持按提供商排除。
- **循环防护(loopguard)**:检测重复工具调用与文本复读,注入提示打断死循环。
- **用量观测**:逐请求 token 记账(含 cached/reasoning tokens)、每日聚合、成本估算、请求详情、SSE 实时事件流(`/api/usage/stream`)。
- **MITM 调试代理**(仅 localhost):本地 TLS 拦截配合 DNS 劫持,观察 CLI 工具的 LLM 流量,帮助编写适配器。
- **CLI 工具一键接入**:为 Claude Code / Codex / OpenCode / Cline / Copilot CLI 等十余款工具写配置。
- **Tailscale 隧道**:检测/安装/启用 Funnel,把本地网关暴露到公网。
- **内置管理面板**: 基于 Solid.js + Vite + Tailwind CSS v4 构建的现代化单页控制台，具备完整深色玻璃拟物风格、统一矢量图标库、无内联 `any` 严格类型安全与流畅动画。
## 快速开始

### 要求

- Go 1.26+
- Node.js 20+(仅构建面板时需要)

### 运行

```bash
# 1. 构建前端面板(产物嵌入二进制)
cd webui && npm ci && npm run build && cd ..

# 2. 构建并运行网关
go build -o cyrene-gateway ./cmd/gateway
./cyrene-gateway
```

启动后:

- 管理面板:<http://127.0.0.1:20128>
- OpenAI 兼容端点:`http://127.0.0.1:20128/v1/chat/completions`
- 健康检查:`GET /api/health`

数据目录为 `~/.cyrene-gateway/`(SQLite 数据库 `data.sqlite`)。

### 命令行参数与环境变量

| Flag | 环境变量 | 默认值 | 说明 |
|---|---|---|---|
| `-host` | `CYRENE_HOST` | `127.0.0.1` | 绑定地址(默认仅本机) |
| `-port` | `CYRENE_PORT` | `20128` | 网关端口 |
| `-data-dir` | `CYRENE_DATA_DIR` | `~/.cyrene-gateway` | 数据目录(数据库、密钥与面板缓存) |
| `-secret` | `CYRENE_SECRET` | 空 | 面板访问密码;空则自动生成并持久化 |
| `-dashboard` | `CYRENE_DASHBOARD` | 空 | 本地面板目录(开发用),空则用嵌入式面板 |
| `-panel-url` | `CYRENE_PANEL_URL` | 空 | 面板更新包 URL(dist.zip 或单 HTML),空则用嵌入式 |
| `-mitm` | — | `false` | 启用 MITM 代理(强制要求 localhost 绑定) |
| `-mitm-port` | `CYRENE_MITM_PORT` | `443` | MITM 代理端口 |
| `-allow-private-networks` | `CYRENE_ALLOW_PRIVATE_NETWORKS` | `false` | 允许出站访问私网地址(本地 mock 测试用) |
### 使用示例

```bash
curl http://127.0.0.1:20128/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-5",
    "messages": [{"role": "user", "content": "hello"}],
    "stream": false
  }'
```

在面板(或 `/api/providers`)添加提供商连接后,`model` 字段支持:

- 具体模型名(如 `claude-sonnet-4-5`)→ 按模型前缀推断提供商;
- Combo 名 → 按 combo 策略在多个模型间回退/轮转;
- 模型别名(面板中配置)。

## 开发

```bash
# 后端
go build ./...
go vet ./...
go test ./...          # CI 另会跑 go test -race ./...

# 前端
cd webui
pnpm install
pnpm typecheck        # tsc --noEmit
pnpm test             # vitest
pnpm build            # vite 构建生产产物
pnpm dev              # dev server :5173，自动代理 /api 到 :20128

### 目录结构

```
cmd/gateway/        # 入口:配置、数据库、HTTP server、优雅停机
internal/
  handler/          # HTTP 处理器(chat/embeddings/messages、管理 API、面板)
  middleware/       # Recovery / Logging / 请求体限制 / CORS / API Key / 面板会话
  auth/             # HMAC 密钥管理、API Key 签发、Argon2id 密码、登录限速
  provider/         # 提供商注册表、OAuth 流程、凭证调度、fallback、SSRF、代理池
  model/            # 连接/Combo 等领域模型、模型目录、live 模型发现与缓存
  db/               # SQLite 存储层(连接、组合、用量、KV、设置)
  translator/       # OpenAI ↔ Anthropic ↔ Gemini 响应格式互转
  usage/            # 各格式响应的 token 用量提取
  media/            # embeddings / TTS / STT / image / video / web 媒体端点
  mitm/             # TLS 拦截代理、根 CA、DNS 劫持、流量环形日志
  tunnel/           # Tailscale 隧道管理
  cli/              # CLI 工具适配器(claude/codex/opencode/cline/…)
  skills/           # cyrene-* 技能清单(chat/search/fetch/tts/stt/image/embeddings)
  loopguard/        # 对话死循环检测
webui/              # Solid.js + Vite 管理面板（构建产物由 CI 嵌入二进制，源码不提交 dist）
schema.sql          # 数据库 schema 参考(实际迁移在 internal/db/db.go)
```

### 发布

打 `v*` tag 触发 GitHub Actions,多平台交叉编译(linux/darwin/windows × amd64/arm64)并附到 Release。版本号通过 `-ldflags -X .../internal/handler.version=…` 注入,未注入时从 git build info 读取,回退 `dev`。

## 安全要点

- 默认只绑 `127.0.0.1`;管理 API(`/api/*`)对非环回来源**始终**要求会话认证。
- `/v1/*` 可通过设置开启 API Key 强制校验(HMAC 签名 + 数据库白名单)。
- 出站请求默认启用 SSRF 防护(解析期 + 拨号期双重校验,含重定向校验)。
- MITM 代理仅在 `-mitm` 且 localhost 绑定时可用。
- 登录失败按 IP 指数锁定(30s → 30m);密码使用 Argon2id(兼容旧 HMAC 哈希自动迁移)。

## License

以 [MIT](LICENSE) 许可证开源。
