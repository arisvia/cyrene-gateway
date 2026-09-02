# Changelog

所有显著变更记录于此。格式基于 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本遵循[语义化版本](https://semver.org/lang/zh-CN/)。

## [Unreleased]

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
