# Changelog

所有显著变更记录于此。格式基于 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本遵循[语义化版本](https://semver.org/lang/zh-CN/)。

## [1.7.0] - 2026-10-16

### Added
- **轮次感知智能令牌瘦身（Turn-Aware Tool Slimming）**：升级 RTK 压缩引擎，全量保护最新活跃轮次以确保 Agent 完整读取代码与报错，对前序过期轮次实施安全折叠与摘要，并提供 `X-Cyrene-Tool-Slimmer` 动态请求头。
- **Anthropic 官方规范 Token 计数端点（count_tokens）**：提供 `POST /v1/messages/count_tokens` 原生接口，支持本地多语言 BPE 快速分词估算与官方上游透明转发，并集成 SSRF 安全防护。
- **Combo 组合路由多维智能选路**：新增成本优先（`cost-first`）与时延感知（`latency-first`）调度策略，支持按模型价格因子调度廉价节点及基于 EWMA 滑动窗口与动态故障惩罚自适应选路。
- **Anthropic 提示词缓存自动打点（Auto Prompt Caching）**：在转发 Anthropic/DeepSeek 请求时自动在 System Prompt、工具集与前序轮次注入 `ephemeral` 缓存断点，强行激活 90% 输入费用打折并严格约束 4 个标记上限。
- **演练场思考过程独立解构与折叠展示**：解构并分离 `reasoning_content` 与回答正文，以独立折叠卡片实时流式呈现思考链路，杜绝思考内容污染回答与后续上下文。
- **用量详情与拓扑实时流式联动（Real-Time SSE Sync）**：用量明细与拓扑图全量接入 SSE 事件驱动自动刷新，消除请求结束或失败后界面滞后或需手动刷新的体验问题。
- **多色谱贯通激光动效（Chromatic Spectral Laser）**：重构拓扑能量通路为单线多色阶贯通导管、白热核心丝与多级彩色光子波束，带来极具质感的实时传输流动感知。

### Fixed
- **CodeBuddy 会话渠道校验合规补齐（X-Conversation-* Headers）**：自动提取与规范化会话标识（`session_id`、`prompt_cache_key`），注入渠道验证所需的 `X-Conversation-Id` 与 `X-Conversation-Request-Id`，修复 11128 鉴权校验失败拦截。

## [1.6.1] - 2026-10-16

### Fixed
- **上下游链路长流防截断与超时重构**：解除流式长连接的硬性总时长 Client.Timeout，引入每分块 5 分钟空闲停滞检测（Stall Timeout）并规范 `no-cache, no-transform` 响应头，彻底消除超长思考模型（o1/o3-mini/R1/Claude 3.7）在 3~5 分钟被误杀截断的问题。
- **Claude 思考流保真与开源模型 `<think>` 标签解构**：补齐 `translateClaude` 与非流式中的 `thinking_delta` 映射，消除思考过程丢失；引入流式标签状态机（`ThinkTagState`）自动将开源模型内嵌的 `<think>` 标签解构为标准思考块，防止提示词污染。
- **Antigravity 媒体聚合异常透传**：为图片生成流式聚合补齐 HTTP 状态码拦截与双格式 Candidate 兼容，消除失败时静默下发空数据的假成功假象。
- **多模态媒体模型注册表多退少补**：全面更新生图、语音合成、语音识别、视频生成与向量模型目录，剔除废弃与未适配占位，补齐 Google Imagen 3、FLUX 1.1 Pro、DALL·E 3、Groq Whisper Turbo 与 ElevenLabs Flash 等官方主力模型规格。
- **媒体搜索与抓取提供商智能推断**：支持自动从 `model` 参数前缀推断检索/抓取 Provider（如 `model: "tavily"` 或 `model: "brave-search"`），无缝兼容限制传参格式的第三方 Agent 框架。

## [1.6.0] - 2026-10-16

### Security
- **控制台严格 CSP 隔离与容器最小特权加固**：针对管理页面实施细粒度内容安全策略并外置引导脚本；Docker 默认启用只读系统与特权剥离（cap_drop: ALL），并约束 Argon2 哈希解密资源上限防范 DoS。

### Added
- **端到端请求链路追踪与有序事件推送**：贯通网关生成之 `X-Cyrene-Request-ID` 与 `sequence` 序列号至全链路生命周期；心跳改用单一定时器降低内存开销并防止长连接空闲断流。
- **网关拓扑均布椭圆环与 S 形旋涡连线**：重构拓扑排布为等角自适应椭圆环，配合 1.3 倍摆尾的三次贝塞尔旋转等变 S 曲线，大幅提升连线对称美感并支持容器动态视窗适配。

### Fixed
- **Google Gemini 与 Antigravity 工具参数清洗**：自动将联合类型数组展平成标量并剥离不支持的 Schema 属性，杜绝 Protobuf 解析器 HTTP 400 报错。
- **Qoder 隐式额度耗尽熔断与指数退避**：精准识别 HTTP 200 SSE 流内嵌的 403 额度用尽错误，自动持久化健康状态并触发冷却退避，阻断无效轮询。

## [1.5.1] - 2026-10-10

### Fixed
- **Claude 5.x 自适应思考与双向深度保真**：严格校准 Claude 5.x 主版本识别，防止 3.5 模型被误触发；支持 `output_config.effort` 与双向往返映射，修复显式禁用推理时误删温度参数。
- **Gemini Schema 清洗与图片模型扩展**：工具 Schema 自动过滤 `errorMessage` 关键字；图像目录加入 `gemini-nano-banana-2.1`。
- **控制台交互防抖与多语言体验修复**：统一网关端点全量国际化，复制状态容器锁定宽度与平滑过渡消除 CLS 抖动，并同步首屏 `html` 语言属性。

## [1.5.0] - 2026-10-09

### Security
- **鉴权与密钥生命周期 Fail-Closed 防护**：Settings 异常时阻断开放网关降级并返回 503，强制校验 HMAC 密钥长度（>=32 字节）并实施原子写入。
- **脱敏备份全链路闭环与凭据保护**：脱敏备份彻底擦除代理 URL 用户凭证、敏感 KV 项、用量元数据及请求明细载荷；合并导入保护库内已有真实凭据。

### Added
- **多账户连接优先级与模型倍率调度**：支持同级负载均衡与跨级故障转移，用量统计透传价格倍率并支持动态配额解析。
- **循环防御门禁全局可控（LoopGuard）**：支持全链路开关循环防御，在保持请求结构完整的同时防御多轮循环死锁。

### Changed
- **区域型提供商规范统一**：拆分 Qoder 国际版与国内版独立生命周期，收口别名解析为规范 Provider ID。

### Fixed
- **上游提供商连接写入校验**：创建与更新上游连接时严格拦截未知 `authType`，防止脏数据入库。
- **前端测试多实例警告消除**：状态存储导出测试重置钩子，彻底消除 Solid 多实例重复加载警告。

## [1.4.0] - 2026-09-23

### Added
- **动态模型目录同步与 SQLite 本地持久化**：深度接入 `models.dev` 3660+ 权威模型库并于网关冷启动预热，模型规格多级自适应探测且离线 0 延时兜底。
- **Qoder 协议完整代理与非流式聚合**：支持 Qoder COSY 签名鉴权、双向流式 SSE 信封解包与非流式调用自聚合，原生兼容 OpenAI 客户端交互。
- **全链路用量追踪与请求明细贯通**：端到端捕获 Token 消耗、耗时、成本与双向提示词，支持取消状态捕获与客户端连接感知。

### Changed
- **静态模型目录重构与全面瘦身**：彻底剔除虚构臆测与历史遗留模型，兜底目录精准收敛至近两年各主流厂商官方现役主力系列。
- **全站严格国际化与文案统一对齐**：全量覆盖中英文本地化，消除全部硬编码字符串并通过 `NestedKeyOf` 静态类型约束。
- **设计系统现代性与布局防抖升级**：全站应用 Tailwind CSS v4 现代化质感调优，锁定滚动容器水平内边距以彻底消除布局抖动与焦点环裁剪。

## [1.3.0] - 2026-09-21

### Added
- **Google Gemini 入站协议全栈兼容**：全面开放 `/v1beta/models/...` 与 `/v1/models/...` REST/SDK 端点，支持多轮对话、流式增量、多工具调用与思考链互转。
- **演练场多协议矩阵与多语言代码生成**：支持在 WebUI 演练场中自由切换 Chat、Responses、Messages、Gemini 四大协议，并支持导出 cURL、Python 与 Node.js 客户端代码。
- **强制流式模型非流式工具调用自动聚合**：彻底攻克 OpenAI Codex 等 `ForceStream` 专属模型在非流式调用下丢失 `tool_calls` 的问题，完整保真工具名与参数。

### Changed
- **有状态跨协议流式状态机（SSETranslator）重构**：统一管理 Anthropic、Gemini、Responses 和 OpenAI 间的流式生命周期、工具索引分配与异常流快速失败。
- **OpenAI Responses API 双向规范深度对齐**：全面接入 `response.completed` 等标准终态事件与 `fc_` 规范前缀，实现 Token 详情与结构化参数无缝流转。
- **模型组合（Combo）全协议上游打通与装配收口**：收口全站上游请求预处理，模型组合全面支持路由至 Claude、Gemini、Codex 等异构提供商。

### Security
- **代理池与出站 SSRF 校验加固**：`SafeHTTPClient` 适配环境代理白名单机制，并阻断对伪造内部路由头的越权注入。

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

[1.7.0]: https://github.com/arisvia/cyrene-gateway/compare/v1.6.1...v1.7.0
[1.6.1]: https://github.com/arisvia/cyrene-gateway/compare/v1.6.0...v1.6.1
[1.6.0]: https://github.com/arisvia/cyrene-gateway/compare/v1.5.1...v1.6.0
[1.5.1]: https://github.com/arisvia/cyrene-gateway/compare/v1.5.0...v1.5.1
[1.5.0]: https://github.com/arisvia/cyrene-gateway/compare/v1.4.0...v1.5.0
[1.4.0]: https://github.com/arisvia/cyrene-gateway/compare/v1.3.0...v1.4.0
[1.3.0]: https://github.com/arisvia/cyrene-gateway/compare/v1.2.4...v1.3.0
[1.2.4]: https://github.com/arisvia/cyrene-gateway/compare/v1.2.3...v1.2.4
[1.2.3]: https://github.com/arisvia/cyrene-gateway/compare/v1.2.2...v1.2.3
[1.2.2]: https://github.com/arisvia/cyrene-gateway/compare/v1.2.0...v1.2.2
[1.2.0]: https://github.com/arisvia/cyrene-gateway/compare/v1.1.0...v1.2.0
[1.1.0]: https://github.com/arisvia/cyrene-gateway/compare/v1.0.0...v1.1.0
[1.0.0]: https://github.com/arisvia/cyrene-gateway/releases/tag/v1.0.0
