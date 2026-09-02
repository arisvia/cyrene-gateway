# Changelog

所有显著变更记录于此。格式基于 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本遵循[语义化版本](https://semver.org/lang/zh-CN/)。

## [Unreleased]

### Added
- **数据目录灵活配置**：新增 `-data-dir` 命令行参数与 `CYRENE_DATA_DIR` 环境变量，允许指定数据库、密钥与面板缓存所在目录，实现测试与容器环境的彻底隔离。
- **提供商创建接口防线**：POST `/api/providers` 强化校验，强制要求 `provider` ID 必填（400）、`api-key` 类型密钥必填（400），并在同 provider+authType 已存在活跃连接时拦截重复创建（409）。
- **OAuth 回调 CSRF 防御**：GET `/api/oauth/{provider}/callback` 强制校验 `state` 必填且会话未过期，彻底杜绝无 state 绕过 PKCE 校验的安全风险。
- **CI / Release 职责解耦**：PR 与主干推送走 `build.yml` 门禁（类型检查、单元测试 `-race` 与单二进制冒烟测试）；Tag 推送专走 `release.yml`（多架构二进制交叉编译与 Docker 镜像推送）。

### Fixed
- **前端提供商连接类型错乱**：修复添加提供商表单未显式传递 `authType` 导致 OAuth/免费提供商被错误创建为 `api-key` 类型的逻辑缺陷；修复类型过滤与状态徽章枚举混用问题。
- **前端状态刷新机制优化**：`addProvider` 由本地数组拼接改为调用 `loadProvidersOnly` 服务端权威拉取，确保后端生成的连接 ID 与元数据完整展现。
- **构建产物断层治理**：从 Git 索引中解绑临时构建产物（`webui/dist/`），由 CI 在 Go 编译前重新构建最新 WebUI 产物嵌入，消除本地与历史提交中散落的幽灵哈希资产。
- **CSS 注释解析警告**：重写 `app.css` 中含斜杠星号的注释文本，消除 Lightning CSS 的 `Unexpected token Delim('*')` 构建警告。

### Added
- **MIT LICENSE**：项目正式以 MIT 许可证开源。
- **Prometheus 指标端点** `GET /metrics`（公开可抓取）：
  - `cyrene_requests_total{provider,model,endpoint,status}` 请求计数
  - `cyrene_request_duration_seconds{provider,endpoint}` 上游延迟直方图
  - `cyrene_tokens_total{provider,type}` token 计量（prompt/completion/cached/reasoning）
  - `cyrene_credentials_in_cooldown{provider}` 冷却中凭证数
  - `cyrene_build_info{version}` 构建版本
- **入站限流**：新增 `settings.apiKeyRpm`（0 = 关闭，默认关闭），对 `/v1/*` 按 API Key 每分钟固定窗口限流，超限返回 429 + `Retry-After`；设置即改即生效，无需重启。
- **Docker 支持**：多阶段 `Dockerfile`（面板构建 → Go 静态编译 → Alpine 运行镜像，非 root，含 HEALTHCHECK）、`docker-compose.yml`，CI 在 `v*` tag 时构建多架构镜像（amd64/arm64）并推送 ghcr.io。

### Fixed
- **面板白屏（P0）**：干净克隆构建出的二进制，管理面板因 `webui/dist/assets` 未纳入版本控制而 100% 白屏（JS 请求被 SPA 回退吞掉，返回 200 + text/html）。现构建产物已跟踪，且 `/assets/*` 缺失时正确返回 404（磁盘模式与嵌入模式双路修复），新增回归测试 `TestDashboardAssetMissIs404NotSPA`。
- **前端构建失败（P0）**：`vue-tsc 3.3.11` 与 `typescript 7`（Go 重写版）不兼容，`npm run build` 必然失败（CI 同样红）。`typescript` 钉回 `~5.9.3`。
- `.gitignore` 全局 `dist/` 规则误伤 `webui/dist/`（锚定为 `/dist/`）；`webui/.gitignore` 移除对 `dist/assets|providers|i18n` 的忽略。

## [历史版本]

未打 tag 前的开发史见 [git log](https://github.com/arisvia/cyrene-gateway/commits/main)（自 2026-07-21 起）。
