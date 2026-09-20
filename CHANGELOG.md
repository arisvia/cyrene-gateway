# Changelog

所有显著变更记录于此。格式基于 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本遵循[语义化版本](https://semver.org/lang/zh-CN/)。

## [1.2.4] - 2026-09-20

### Fixed
- **Antigravity 地域与网络限制根治**：彻底剥离诱发 Google 判定为受限 Cloud Shell 容器的 `X-Goog-Api-Client` 脏头，规范透传会话标识（`sessionId`），彻底解决 `User location is not supported` 阻断。
- **Responses API 双向流式转换加固**：重构工具调用索引跟踪为状态化精准映射，消除并行函数调用与图文混排下的索引漂移及 `finish_reason` 丢失。

### Added
- **无头服务器 OAuth 手动回调粘贴模式**：针对 Antigravity 官方 Client ID 严格限定本地回环回调的特性，新增浏览器地址栏完整回调 URL/Code 手工直粘与前端解析换票机制，保障远程 VPS 部署无障碍授权。
- **Anthropic Messages 协议全格式兼容增强**：补齐非流式与流式数据行提取防御性兼容，保障任意客户端向各上游模型无缝调用。

## [1.2.3] - 2026-09-18

### Security
- **外网智能识别与登录自适应防护**：智能探测客户端 IP 与请求 Host，非局域网/本机访问时默认强制开启管理面板登录，杜绝远程未授权接管；支持 `-admin-password` 参数或安全随机密码首启。

### Fixed
- **管理会话拦截与死锁修复**：修复未配置密码时 401 拦截导致的登录弹窗失效，更新密码后即时签发会话凭据并支持动态引导初始化。
- **全站模型友好展示名双显**：用量明细、实时请求流、钻取详情与模型组合全量支持「友好名称 + 原始 ID」双显与智能兜底。

### Added
- **端点当前访问感知**：首页统一接入端点动态感知当前请求 Host 与协议头，并标明「当前访问」状态。

### Refactored
- **网络层国际化与死键清理**：API 与网络拦截层全量接入 Strict i18n 规范，彻底消除硬编码字符串并物理清理 22 个无引用孤立死键。

## [1.2.2] - 2026-09-17
### Added
- **模型静态目录全厂商 2026 对齐**：依据 `models.dev` 权威数据库，全面增补与对齐 OpenAI（含 GPT-6 Astra、GPT-OSS 系列）、xAI Grok（含 4.20、4.6、4.2-Fast、Grok Build）、通义千问 Qwen 3.8 全系、Mistral Large 3/Small 4、Meta Llama 4、Cohere Command A+、Perplexity Sonar、Amazon Nova、百度文心 ERNIE 5.x、字节跳动 Seed 2.0、小米 MiMo、阶跃星辰 Step 等主流现役模型。
- **Antigravity 周度与共享配额支持**：支持 `retrieveUserQuotaSummary` 提取 5 小时与每周额度窗口，合并 Claude 与 GPT 共享配额池。

### Fixed
- **Qoder 模型来源透传修复**：完整透传 `model_config.source` 至请求头 `X-Model-Source`，消除退回兜底值。
- **Antigravity 动态模型自适应命名**：自动格式化 `gemini-3.7-flash-tiered` 等自适应端点展示名，并淘汰清理过期实验模型。

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

[1.2.4]: https://github.com/arisvia/cyrene-gateway/compare/v1.2.3...v1.2.4
[1.2.3]: https://github.com/arisvia/cyrene-gateway/compare/v1.2.2...v1.2.3
[1.2.2]: https://github.com/arisvia/cyrene-gateway/compare/v1.2.0...v1.2.2
[1.2.0]: https://github.com/arisvia/cyrene-gateway/compare/v1.1.0...v1.2.0
[1.1.0]: https://github.com/arisvia/cyrene-gateway/compare/v1.0.0...v1.1.0
[1.0.0]: https://github.com/arisvia/cyrene-gateway/releases/tag/v1.0.0
