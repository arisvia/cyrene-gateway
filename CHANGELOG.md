# Changelog

所有显著变更记录于此。格式基于 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本遵循[语义化版本](https://semver.org/lang/zh-CN/)。

## [1.2.0] - 2026-09-17

### Added
- **OpenCode 双端点适配**：新增 `opencode` 与 `opencode-go`（Zen）接入支持及免密模型目录。
- **系统状态与在线运维**：支持系统实时指标监控、SQLite WAL Checkpoint/VACUUM 以及在线平滑重启与热升级。

### Fixed
- **OpenCode 客户端指纹加固**：对齐规范化 UA 及 30 位会话/请求 ID，消除免费档 403 阻断。
- **演练场（Playground）工作台重构**：固定视口布局防抖动，优化空状态排版与参数面板联动。
- **用量看板与容器防抖**：修复 Token 趋势柱悬浮抖动（CLS），补齐滚动容器呼吸内边距。

### Refactored
- **凭据与模型级路由收口**：统一由 `provider.ResolveCredentials` 管理免密与付费路由，清理散落逻辑。

## [1.1.0] - 2026-09-16

### Security
- **OAuth 出站流 SSRF 防护增强**：统一经 SafeHTTPClient 代理校验，阻断私网与元数据探测。

### Added
- **`-dashboard` 本地开发模式**：支持直接挂载静态目录进行热开发，并规范化结构化 CLI 帮助文案。

### Refactored
- **核心算法与工具函数收口**：统一 Base64URL、PKCE、SSE 解析及 UUID 生成；规范前端统一图标库与剪贴板工具。

### Testing
- **配置契约测试**：引入 CLI 单/双横杠参数解析契约测试，清理冗余测试辅助逻辑。

## [1.0.0] - 2026-09-15

### Security
- **DNS Rebinding 与本地跨域隔离（P0）**：引入可信回环校验，防范跨域劫持；实施全链路 SSRF 与暴力破解防护。

### Added
- **主流模型协议深度兼容**：支持 Claude Code 1M 标记清洗、Codex 工具正则过滤及并行工具结果自动聚合。
- **高性能响应缓存与多模态支持**：无锁 LRU 缓存零延迟回放；内置 Prometheus 指标端点与多阶段 Docker 容器化。

### Changed
- **统一能力标签与提供商市场重构**：打通多模态媒体与文本提供商统一市场，接入 Google Antigravity 深度协议。
- **架构极致瘦身**：移除边缘非核心模块，精简净削减 3,500+ 行代码。

### Fixed
- **全站视觉与稳定性治理**：统一空状态组件与暗色毛玻璃规范，修复面板白屏（P0）与定时器内存泄漏。

[1.2.0]: https://github.com/arisvia/cyrene-gateway/compare/v1.1.0...v1.2.0
[1.1.0]: https://github.com/arisvia/cyrene-gateway/compare/v1.0.0...v1.1.0
[1.0.0]: https://github.com/arisvia/cyrene-gateway/releases/tag/v1.0.0
