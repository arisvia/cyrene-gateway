# Changelog

所有显著变更记录于此。格式基于 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本遵循[语义化版本](https://semver.org/lang/zh-CN/)。

## [1.8.3] - 2026-10-29

### Added
- **会话亲和覆盖无会话字段的 agent 客户端**：显式字段之外新增具名会话头白名单（`X-Session-Id`/`X-Conversation-Id`/`X-Grok-Conv-Id`，值经字符集校验）与「API 密钥作用域 + system + 首条用户消息」内容指纹兜底，pi/opencode/aider 等不携带任何会话标识的多轮对话从此也能钉定上游账号提升缓存命中；指纹跨租户隔离、仅 SHA-1 进入内存表。
- **媒体管线容灾与响应契约统一**：图像/TTS/STT/视频/网页抓取/搜索全部接入账号级故障转移走查（429/配额/5xx/网络错误自动换号并冷却，请求级 4xx 仍立即终止），且 Gemini 图像与 TTS、Stability 图像、Deepgram/Gemini STT 的上游原生响应统一归一为 OpenAI 契约（`b64_json` 图像数组、裸音频字节配 `audio/*` Content-Type、`{"text":...}` 转写），此前这些提供商会把 generateContent JSON 信封原样透传、下游 SDK 解析即失败；媒体出站并入代理池保证（含强制代理永不直连），媒体调用计入用量历史与图表，视频 job 注册到创建账号使状态轮询免 provider 参数。

### Changed
- **antigravity 思考档位解析去除死家族硬编码**：`gemini-3.5-flash`/`gemini-3.1-pro` 的逐家族特判（含 low→extra-low 错档映射）删除——线上目录已无这两个家族，特判只会把请求钉到不存在的模型 ID；档位解析统一为「模型后缀优先 → 目录精确命中 → 目录内 tier 探测 → 通用后缀兜底」，对 claude 系 tier 模型同样生效；`extra-low` 保留为合法 tier 词表（后缀解析与防错档需要）。

### Fixed
- **用量界面模型名显示不统一**：聊天行记录裸模型 ID 而模型目录以 `provider/model` 全名发布，明细日志/多维用量/实时事件里同一模型有的显示友好名有的只剩裸 ID，媒体搜索行还因写入全名在用量分析里分裂成独立桶；展示名解析现按行内提供商优先精确匹配全名（跨提供商同名裸 ID 也能正确定名）、裸 ID 镜像兜底，副行统一显示规范 `provider/model` 全名，媒体用量写入剥离提供商前缀与聊天对齐。
- **Web 搜索面因引擎被禁用而整体消失**：antigravity 搜索的硬编码引擎（gemini-2.5-flash/pro）一旦在聊天侧被禁用（成本管控常见操作），媒体列表与默认引擎双双落空、搜索页签空白；搜索引擎现从提供商实时模型目录动态匹配（Gemini 家族 chat 引擎、排除图像型号）并保留静态活跃档兜底，无模型请求自动落到首个未禁用引擎、全禁时返回明确 503；禁用门禁本身保留（显式点名被禁引擎仍 403）。
- **Combo 链路的会话亲和补全**：命中 Combo 模型的 Anthropic Messages 请求原本先于会话标识解析就移交聊天管线（翻译后的 OpenAI 请求体不携带 `prompt_cache_key`/`session_id`/`metadata.user_id`），多轮对话被轮转打散、上游 KV 缓存保护失效；标识解析现前置到移交之前并经请求头传递。
- **客户端断连不再误伤账号池**：Anthropic Messages 直通在客户端中途断开时，传输层取消错误不再计入账号冷却并级联故障转移——此前单个用户挂断可把该提供商全部账号逐一切入冷却。
- **Embeddings 成功后清除错误状态**：`/v1/embeddings` 在 4xx/5xx 持久化限流与退避状态后，后续成功不再清除（对仅提供嵌入流量的账号是永久隔离）；成功现在与聊天管线同步重置。
- **提供商详情页顶部开关水合窗口期空操作**：`/api/providers` 列表尚在加载时点击顶部组开关只触发刷新、不改变任何账号状态；现回退为直接切换当前已加载连接。

## [1.8.2] - 2026-10-28

### Added
- **会话亲和负载均衡(上游 KV 缓存保护)**:携带显式会话标识的请求(`prompt_cache_key`/`session_id`/`conversation_id`、Anthropic `metadata.user_id`,或 `X-Cyrene-Session-ID` 头)在多轮对话中钉住同一账号,保证上游前缀缓存高命中,长会话不再逐轮全价重付 prompt token;绑定账号进入冷却、模型锁或配额上限时无缝迁移并重绑,故障转移与配额语义零改动。跨会话仍按优先级等权均衡;无会话标识的请求完全不受影响——刻意不以消息内容猜测会话,避免无关对话被误钉同一账号。
- **全路径账号级容灾统一**:Anthropic Messages 原生直通与 Combo 步骤现与聊天管线共享同账号换号语义——账号级故障(429/配额/5xx/网络)自动改试下一账号,Combo 会先耗尽同模型全部账号再降级到下一模型,请求级 4xx 仍立即跳模型;单账号被限流不再直接打穿到客户端,也不再误伤健康模型的轮换计数。

### Changed
- **历史兼容垫片彻底移除**：登录口令校验严格化 Argon2id（删除旧 HMAC 回退与就地升级分支，现库已全量 Argon2id 核实）；`organization_name` 蛇形键的双读与 CAS 条件一并拆除（现行写入方唯一使用驼峰键且现库零命中）；前端废弃 IndexedDB 库的一次性清理逻辑随批次退役。
- **KV 模型别名层整体拆除（命名统一规范全名）**：删除 `ResolveModel` 的别名解析层级、`/v1/models` 别名陈列、`/api/models/alias` 三条管理路由与前端悬空接线（现库存量 0 行、控制台无入口）；客户端点名与用量统计自此只有一套名字——`provider/model` 全名，提供商短别名维持既有"仅输入糖、入口即规范化"契约不扩大。

### Fixed
- **Qoder 与 CodeBuddy (Intl) 测试连接 404 误报**：Qoder OAuth 设备令牌改为沿用真实聊天链路的 COSY 签名探测模型列表端点，不再在 SSE 聊天地址后拼接不存在的路径；CodeBuddy Intl 改探与模型同步同源的 `/v3/config` 账户配置端点。
- **提供商详情页顶部开关改为组级批量控制**：一键同时启用或停用该提供商名下全部账号，顶部徽标按全启用/部分启用/全停用三态显示，消除多账号场景下仅影响单条连接的歧义。
- **"限流冷却中"徽标到期不灭**：账号行徽标改为与后端调度一致的"截止时间晚于当前时刻"判据并随时钟自动熄灭；此前只要字符串残留（未成功调用过或未点重置冷却）就永久显示，误导为账号仍在轮换隔离。
- **上游客户端指纹版本升级**：CodeBuddy CLI/IDE 指纹对齐官方 `2.177.1`（原 `2.108.1` 落后约 70 个 minor），Qoder CLI 请求 UA 升至 `qodercli/1.1.80`，降低陈旧客户端指纹触发上游风控误判的概率。
- **Embeddings 端点鉴权收口**：`/v1/embeddings` 统一走 transport 与 `ResolveCredentials` 注入并支持令牌预刷新;此前 OAuth 型账号会携带空 Authorization 裸奔上游。
- **RTK 当前轮报错保护扩列**：活跃轮工具输出的失败识别从 `error:`/traceback/`exit code 1` 扩展至 `panic:`、`fatal:`、任意非零 `exit status/code`、`command not found`、`permission denied`、`core dumped`、pytest `FAILED` 等形态——超限的当前轮失败输出不再被中段截断;设置页 RTK 提示同步披露当前轮/历史轮双阈值真实行为,不再以"无损"概括截断。
- **代理池语义真实化**：`强制代理` 开关落地为"永不直连"——强制池只要存在(即使已停用),经代理池出站的全部请求(聊天/组合/直通/嵌入/模型同步/用量/自更新)在无可用代理时将显式失败而非静默回落直连,删除该池即恢复直连;OAuth 令牌交换与 Qoder 签名链沿用独立直连客户端,不在池保证范围内。代理类型由 URL scheme 自动派生(SOCKS5 池不再误标 http);移除零消费方的"排除地址"字段与永不生效的"绑定提供商"删除守卫,相关文案全部改写为与真实行为一致。

## [1.8.0] - 2026-10-27

### Added
- **同账号微退避重试（In-place Micro-Retry）与 KV Cache 保护**：在单模型路径中引入 200ms 短延时原地重试，对瞬态网络故障或 5xx 优先同凭证重试，避免过早跨账号轮替击穿数十万 Token 的上游 KV Cache。
- **Prompt 级内联指令（In-turn Directives）无痕解析**：支持在对话末尾用户消息中内联指令（如 `@cg-model:openai/gpt-4o`、`@cg-effort:high`、`@ocr-effort-up`、`@ocr-effort-down`），网关自动提取并剥离指令文本，动态重载当轮模型与思考深度。
- **OpenAI 双鉴权（Dual-Auth）与 Codex 提供商彻底整合**：将独立的 `codex` 移出注册表，由 `openai` 提供商统一承接 API Key 直连与 ChatGPT/Codex responses 官方 OAuth，根据授权类型自动分流。
- **Tool Result 多模态图像转换支持**：`openAIToClaude` 工具调用结果支持嵌套 `image_url` 与 Base64 图像块，对齐 Claude 官方多模态工具输入规范。

### Fixed
- **API Key 提取与严格鉴权模式兼容性修复**：修复 `ExtractAPIKey` 对带有首尾空白或大小写 `bearer` 前缀的 Bearer Header 解析失败问题，统一返回标准 OpenAI 格式的鉴权错误体。
- **免密模式下前端控制台获取 `/v1/models` 被误拦截**：支持同源（`Sec-Fetch-Site: same-origin`）请求在未开启强制登录时免鉴权拉取模型列表。
- **Combo 模型非终态请求级 4xx 错误容灾流转**：当组合内前序模型遇到请求级错误（如上下文超长）时，自动尝试下一个可用模型而非直接向客户端返回失败。
- **Antigravity 客户端指纹对齐**：更新 User-Agent 指纹至 `antigravity/hub/2.24.0 darwin/arm64`。

## [1.7.7] - 2026-10-26

### Changed
- **设置面板重构为即时无感自动保存（Auto-Save）**：移除本地临时草稿状态（`local()`）、脏检查（`dirty()`）、切页弹窗确认与底部浮动保存条。所有开关与下拉配置变动即时通过 `PATCH /api/settings` 持久化，数字输入框经 400ms 防抖自动提交；在页面右上角提供轻量级的已保存脉冲指示。
- **后端配置缓存失效精细化**：`handlePutSettings` 与 `handlePatchSettings` 原先无条件清空所有响应缓存，现精细化为仅当影响模型输出或缓存策略的字段（如 `RTK`、`Caveman`、`AutoPromptCache`、`ResponseCache*`、`TokenSaverExclude`）发生实际变更时才触发 `Cache.Clear()`，避免修改 RPM 等元数据时误清空生产热点缓存。
- **升级 Go 运行时与核心依赖**：升级 Go 编译器与运行时工具链至 `go1.27.3`；依赖 `modernc.org/sqlite` 升级至 `v1.61.0`（包含 `libc v1.79.0`、`go-strftime v1.1.0`）、`github.com/prometheus/common` 升级至 `v0.72.1`。

### Fixed
- **Prometheus `/metrics` 端点公网越权暴露加固**：通过 `middleware.MetricsAuth` 实施安全管控，本机回环自由抓取，外部公网访问强制要求控制台有效 Session 或 Bearer AuthSecret，杜绝内部运行指标、模型名与用量计数外泄。
- **上游错误日志镜像至 Web 日志流的敏感数据脱敏**：在 `BroadcastLogHandler` 中引入 `sanitizeLogAttr`，对入库广播的 `attrs` 中包含 token、secret、password、authorization 等敏感属性自动打码脱敏，并对超过 1024 字符的异常长文本自动截断，防止上游回显的用户提示词或内部诊断溢出至前端日志面板。
- **RTK 对 OpenAI 与 Gemini 工具输出的报错保护增强**：在 `internal/rtk/rtk.go` 中引入 `isToolOutputError`，统一识别各协议中包含 `error:`、`Traceback`、`Exception`、`exit code 1` 等报错堆栈的工具输出，在当前活跃轮次中 100% 保留不截断，确保智能体能获得完整的报错排查上下文。
- **Antigravity 单模型白名单 `TokenSaverExclude` 匹配修复**：此前 Antigravity 请求未在 `innerRequest` 中透传 `model` 字段，导致单模型白名单排除规则失效。现已在 TokenSaver 过滤前补充模型 ID 传递。
- **Anthropic 原生直通请求（`/v1/messages`）遗漏 TokenSaver 与 RTK 压缩管线**：`handleMessages` 原先仅对 `handleMessagesViaChat` 转发路径生效 TokenSaver，原生 Anthropic 直通请求未调用 `applyTokenSaver`，导致通过 Claude 官方端点调用的会话无法享受 RTK 工具压缩与 Terse 提示词注入。现已补齐调用。
- **RTK 对 Gemini `functionResponse` 仅匹配 `content` 键、忽略 `result` 键**：Antigravity 等转换器生成的工具响应结构为 `{"result": ...}`，此前未被压缩。现已扩展匹配 `resp["result"]` 字段。
- **公网/隧道下保存设置误报“cannot disable login requirement”阻断保存**：后端防降级校验原先写成了 `if !settings.RequireLogin && IsPublicWANRequest(r)`，当数据库中历史初始状态 `RequireLogin` 为 `false` 且用户经 Cloudflare 隧道访问时，前端保存任何设置（如开启 API Key 鉴权）都会无辜触发该拦截。现修复后端为仅在“原先开启且尝试关闭”时拦截（`current.RequireLogin && !updated.RequireLogin`），保留 Localhost 免密与公网安全约束的同时，支持公网正常修改其它系统设置。
- **Anthropic 协议适配端点（`/v1/messages`）转发非 Anthropic 供应商时轮询计数器步进两次导致多账号负载不均衡**：`handleMessages` 原先在委托给 `handleMessagesViaChat` 之前先调用了 `selectAvailableConnection` 推进了一次轮询游标；随后在 `handleSingleModelChat` 中又推进了一次。对于 2 个相同优先级的账号（如 Antigravity），每次请求游标增加 2，模 2 结果恒等于 0，导致流量 100% 压在第一个账号上，第二账号始终 0 消耗。现改为在 `handleMessages` 前期直接基于供应商特征（非 `anthropic` APIType 或 Qoder / Antigravity 等专用协议）委托，避免二次选择凭证。
- **Antigravity 消息转换对空文本生成空 text part 导致 Vertex 400 报错**：`convertOpenAIToGeminiContents` 原先无条件构造 `[]map[string]any{{"text": text}}`。当 assistant 轮次仅包含 thinking 或 tool_calls 时，`text` 为空字符串，下游 Vertex 判定为缺少字段并返回 `messages.N.content.0.text.text: Field required` (HTTP 400)。现改为仅在 `text != ""` 时追加 text part，system 轮次同步增加非空检查。
- **代理池配置接受无效 URL 导致运行时拨号失败**：`handleCreateProxyPool` 与 `handleUpdateProxyPool` 原先仅校验字符串非空，未检查 scheme 与主机端口合法性。现新增 `provider.ValidateProxyURL`，创建和更新时对 scheme（仅允许 http/https/socks5/socks5h）及 endpoint 格式进行校验，非法 URL 返回 400。

## [1.7.6] - 2026-10-23

### Fixed
- **测试数据竞争**：`handleUpdateProvider` 等处以 `go s.syncConnectionModels(...)` 起后台目录同步，goroutine 会活过测试函数，在下一个测试改写 `provider.Registry` 时并发读该 map，`go test -race` 偶发失败。现统一经 `Server.goBackground` 登记，测试清理阶段 `waitBackground` 等待其结束。

## [1.7.5] - 2026-10-23

### Fixed
- **账号额度耗尽不会切到下一个账号，客户端直接失败**：单模型路径（`handleSingleModelChat`）选完连接只发一次请求，上游报错就直接回给客户端，从不重试其它账号——`provider.ExecuteAttempt` 的 `ShouldFallback` 分类器早已存在却没有任何调用方。现改为在连接池内循环重试：每次失败按 `CheckFallbackError` 判定，账号级失败（额度耗尽、限流、传输错误、缺凭据）换下一个账号，请求级 4xx（上下文超长、参数错误）立即返回不扩散。Qoder 与 Antigravity 两个专用处理器改为返回 `retryable` 而非直接写错误体。
- **额度耗尽只冷却 30 秒，等于没生效**：错误文本规则只有 `quota exceeded` 一条，Antigravity 实际返回 `RESOURCE_EXHAUSTED` / `RATE_LIMIT_EXCEEDED`，CodeBuddy 返回 `usage limit`，全部落到默认的 30 秒瞬时冷却，账号几秒后又回到轮转里继续失败。现新增 `CooldownQuota = 15m` 并优先匹配真实配额耗尽文案（`resource_exhausted`、`ratelimit_exceeded`、`rate_limit_exceeded`、`usage limit`、`insufficient balance/credit`、`daily/weekly/5-hour limit`）；普通 `rate limit` 仍走 2s 起的指数退避，避免把瞬时限流误判成额度耗尽。
- **上游忽略 `stream:true` 时流式请求丢答案**：客户端要流式但上游回普通 JSON 时，`proxyResponse` 按 `Content-Type` 判定仍走 `proxyStreaming`，把 JSON 当 SSE 解析，最终吐出 `upstream stream ended before completion: EOF`，上游完整回答被丢弃。现新增 `streamFromJSONResponse`，把非 SSE 响应合成为单帧 chunk + usage 帧 + `[DONE]`，三条适配端点都能拿到内容。
- **非流式 Qoder 排队必然 524**：注释帧保活只在 `req.Stream` 为真时生效，非流式请求排队期间零字节输出，Cloudflare 100 秒后返回 524。现非流式路径遇到 `10605` 立即返回 `503` 并带 `Retry-After`，让客户端自行重试而不是空等到边缘超时。10 分钟 / 40 次预算对流式路径保持不变。
- **Gemini 端点把 `:countTokens`、`:embedContent` 当生成请求转发**：两者被 `handleGeminiGenerateContent` 统一送进生成链路，返回 `candidates` 而非 `totalTokens` / embeddings，SDK 无法解析。现按 action 分流：`countTokens` 走本地估算返回 `{"totalTokens":N}`，`embedContent` / `batchEmbedContents` 转 `/v1/embeddings` 后转回 Gemini 形状，未知 action 返回 400。
- **SSE 单行超限导致流静默中断**：`/v1/messages` 原生透传、Qoder 流式与非流式三处 `bufio.Scanner` 上限 1MB，超大 tool 参数或 reasoning 帧会触发 `token too long`，循环静默结束，客户端只看到流突然断开。上限提至 8MB（`maxSSELineBytes`），且 scanner 报错时补发 `event: error` 终止帧。
- **`/v1/messages` 上游错误缺少可操作提示**：与 `/v1/chat/completions` 不一致，现统一经 `wrapErrorWithHint` 附加 `hint` 字段。
- **Qoder 排队期间经协议适配端点调用被 Cloudflare 判 524**：`handleQoderChat` 在排队等待时每秒写 `: ping` 注释帧保活，但 `/v1/responses`、`/v1/messages`、`/v1beta/models/*`（Gemini）三个适配器只解析 `data:` 行，注释帧被直接丢弃且不 flush，下游 100 秒收不到任何字节，边缘返回 524。Antigravity 流式转发同样丢弃注释帧。现统一经 `forwardSSEComment` 原样转发注释帧并立即 flush（自动补终止空行），四处流式路径的响应头也改为写出即 flush。
- **同优先级账号并不负载均衡，流量全压在第一个账号**：`SelectCredentialWithQuota` 只按 OAuth 优先做稳定排序后取首个可用连接，同优先级时列表顺序固定，导致同池第二个账号几乎零消耗（表现为一个账号 100%、另一个 0%）。现改为按优先级分层，仅在最高可用层内做进程内轮询，同层多账号按请求均摊；某层全部冷却/配额耗尽时自动落到下一层。

## [1.7.4] - 2026-10-21

### Performance
- **用量面板 SSE 刷新合并（每事件 4 个请求 → 每秒最多 4 个）**：一个请求会发出 accepted → routing → streaming → completed 多个事件，每个事件都触发 `loadRequestDetails` 加 3 个 `/api/usage` 调用，繁忙时整页每秒重渲染数十次。现合并为 1 秒一次，标签页隐藏时跳过，切回前台立即补刷。
- **用量统计聚合 2 秒 TTL 缓存**：`/api/usage/stats` 每次都要扫 `usageHistory` 并解析多维聚合，多开面板即多份重复计算。按周期缓存序列化结果，响应头 `X-Cyrene-Cache: hit` 可验证。
- **非详情页不再刷新明细表**：概览页不渲染请求明细，原实现仍在刷新；现仅在详情页且停留在第一页时刷新，切回详情页主动拉取以免看到陈旧数据。
- **实时事件按 `requestId` 去重**：同一请求的多个生命周期阶段原本各占一行，现原地更新，减少 DOM 行数。
- **日志行格式化移至入库时**：300 行每次刷新都重跑 `toLocaleTimeString` 与 attrs 拼接，是刷新时的主要开销。现于入库时计算一次。
- **日志页后台暂停渲染**：标签页隐藏时不再每 100ms 重建列表，改为回到前台一次性补刷，并给待渲染缓冲加上限。
- **日志环形缓冲加固**：单锁内完成广播（原实现解锁后重新加读锁，存在 records 与 subs 不一致窗口）；订阅者积压时保留最新而非丢弃（日志尾部只有追到当前才有价值）；淘汰改为原地 `copy` 避免每次重分配。
- **messages 透传流跳过冗余 JSON 解析**：每条 SSE 行原本完整解析两次，其中 usage 提取对最常见的 `content_block_delta` 毫无收获。现以字节扫描门控，长流下每行省一次完整 unmarshal；行输出改用 `w.Write` 避免格式解析。
- **请求体按 `Content-Length` 预分配**：新增 `readRequestBody`，避免 `io.ReadAll` 对多 MB 会话反复扩容拷贝。长度仅作提示，chunked、虚假长度、超限声明均回退到逐字节读到 EOF。

### Fixed
- **日志页启动窗口丢日志**：原实现先取历史再建 SSE，中间产生的日志完全丢失。现先订阅后取历史，按 `时间|级别|消息` 去重合并。
- **日志 SSE 心跳定时器泄漏**：循环内 `time.After` 每轮新建定时器且到期前无法回收，日志密集时每行泄漏一个；改用复用的 `time.Ticker`，与用量流一致。
- **日志自动滚动打断阅读**：原为需手动开关的模式，上翻历史时新日志仍把视图拽到底部。现由滚动位置驱动，上翻自动暂停、回到底部自动恢复。
- **实时事件列表徽章错位**：模型 ID 长短不一导致状态徽章与延迟漂移。改为固定列宽 grid，模型名是唯一弹性轨道。

### Changed
- **`chat.go` 按职责拆分（2291 → 939 行）**：拆出 `chat_combo.go`、`chat_auth.go`、`chat_tokensaver.go`、`chat_usage.go`、`chat_messages.go`、`chat_embeddings.go`。函数数量与拆分前完全一致。
- **`server.go` 按资源拆分（1956 → 395 行）**：拆出 `settings.go`、`providers.go`、`combos.go`、`apikeys.go`、`proxypools.go`、`models.go`。

### Known Limitations
- **Cloudflare Quick Tunnel 下 SSE 仍不可用**：实测 8KB 预填充发送完毕后 6 秒仍收不到首字节（`gotFirstChunk: false`），该模式对 `text/event-stream` 为整包缓冲而非阈值缓冲，任何预填充量都无法击穿。需改用 Named Tunnel、DNS Only 或内网直连。API 调用不受影响。

## [1.7.3] - 2026-10-21

### Fixed
- **大请求体上传被 30 秒读超时截断（ReadTimeout）**：`ReadTimeout` 覆盖整个请求体读取，经 Cloudflare 隧道上传 1.6MB 会话（1063 条消息 + 14 个 tools）必然超过 30 秒，客户端收到 `400 failed to read request body`。读超时放宽至 5 分钟，`ReadHeaderTimeout` 仍约束 Slowloris，`MaxBytesReader` 仍限制体积。
- **长流被 5 分钟写超时截断（WriteTimeout）**：排队数分钟后才开始生成的长请求会撞上固定的写超时。改为不设绝对上限，改由 `NewStallTimeoutReader` 按无数据静默判定，避免误杀正常长流。
- **Qoder 排队预算与上游节拍不匹配**：上游 `retryAfterSeconds=30`，而预算仅 120 秒，20 次重试上限永远无法达到，第 4 次等待后即放弃。预算提升至 10 分钟、上限 40 次。
- **演练场吞掉流中错误帧（`event: error`）**：SSE 读取只解析 `data:` 行，忽略 `event:` 名称，排队超时等中途失败被静默丢弃，界面表现为长时间等待后空白。现正确识别错误事件并抛出，JSON 解析失败也不再掩盖该错误。

## [1.7.2] - 2026-10-20

### Fixed
- **Qoder 上游排队全自动透明代排（In-Stream Queue Polling）**：修正 1.7.1 中排队帧被静默跳过后上游断连导致的 502 空流。现精准提取 `10605` / `isQueued` 的 `retryAfterSeconds` 与队列位置，保持下游长连接并周期性注入 SSE 保活注释帧，原地透明轮询至出字，每次重试刷新 `request_id` 规避 103 Duplicate。
- **SSE 实时流穿透反向代理缓冲（8KB 分块预填充）**：实时用量拓扑与系统日志 SSE 握手升级为 8KB 注释帧分块冲刷，击穿 Cloudflare Tunnel / Nginx 的 4-8KB 写缓冲阈值，修复公网域名下 EventStream 空白挂起、界面不实时刷新的问题。

### Removed
- **移除独立数据迁移工具（`cyrene-migrate-*`）**：提供商 ID 规范化自 `v1.5.0` 起已完成，该一次性工具不再有使用场景。Release 资产由 12 个精简为 6 个纯网关二进制，`scripts/` 源码与相关文档一并清理。

## [1.7.1] - 2026-10-20

### Fixed
- **Qoder 上游排队控制帧穿透与防误杀（isQueued Bypass）**：精准识别并安全跳过 Qoder `isQueued: true`（错误码 10605）排队等待握手信封，解绑非欠费误判，保障高峰期排队后正常接收流式生成结果。
- **上游动态倍率与限免自动感知（CodeBuddy & OpenRouter）**：彻底摒弃静态猜测表，直连解析 CodeBuddy `credits` 字段（如 `x0.79 credits`、`x3.31`）与 `modelPromotions` 活动折扣，并联动识别 OpenRouter `pricing.prompt == 0`，全自动呈现 `Free` 与价格乘数。
- **Combo 零有效内容假成功自动容灾（Zero-Content Fallback）**：针对上游异常返回 HTTP 200 但内容为空且无工具调用的假死响应（PR #4750 经验），自动平滑触发 Fallback 切换至候补模型，彻底杜绝空白回答。
- **Google Gemini 4 (Argon) 协议参数清洗与工具响应末尾定序**：针对 Gemini 4 自动剥离外部采样参数（`temperature`/`top_p`）防范 400 报错，并在多轮用户消息合并中保障 `functionResponse` 置于末尾。

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

[1.8.3]: https://github.com/arisvia/cyrene-gateway/compare/v1.8.2...v1.8.3
[1.8.2]: https://github.com/arisvia/cyrene-gateway/compare/v1.8.0...v1.8.2
[1.8.0]: https://github.com/arisvia/cyrene-gateway/compare/v1.7.7...v1.8.0
[1.7.7]: https://github.com/arisvia/cyrene-gateway/compare/v1.7.6...v1.7.7
[1.7.6]: https://github.com/arisvia/cyrene-gateway/compare/v1.7.5...v1.7.6
[1.7.5]: https://github.com/arisvia/cyrene-gateway/compare/v1.7.4...v1.7.5
[1.7.4]: https://github.com/arisvia/cyrene-gateway/compare/v1.7.3...v1.7.4
[1.7.3]: https://github.com/arisvia/cyrene-gateway/compare/v1.7.2...v1.7.3
[1.7.2]: https://github.com/arisvia/cyrene-gateway/compare/v1.7.1...v1.7.2
[1.7.1]: https://github.com/arisvia/cyrene-gateway/compare/v1.7.0...v1.7.1
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
