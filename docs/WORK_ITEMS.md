# Active Work Items

每次 Session 只完成一个 work item。顺序不可跳过，除非在 `progress.json` 记录阻塞原因和新的依赖关系。

## 37A Secure Boundary and Credentials

### Scope

- 默认只监听 loopback，远程监听要求显式配置并完成认证初始化
- 管理 API 使用脱敏 DTO，不返回任何 Provider credential
- 高权限管理接口统一授权
- 登录 limiter 并发安全、有容量上限和清理策略
- 删除默认密码，采用适合密码的哈希方案
- auth secret 在 config 解析后显式初始化，文件位于固定应用数据目录

### Acceptance

- 默认配置从非 loopback 来源无法匿名访问管理能力
- list/detail/mutation response 均通过 secret contract test
- `go test -race` 下 limiter 无 race
- 首次设置密码、登录、退出、session 失效测试通过
- 前端编译通过，并适配新的脱敏 Connection DTO

### Out of scope

不重构 upstream executor，不重做 Provider 页面。

## 37B Unified Upstream Executor

### Scope

- 将 single、combo、messages 和 tester 的公共执行逻辑抽为统一 executor
- Route Resolve 与 Execute 分离
- 统一 translation、auth、refresh、fallback、connection state、usage 和错误 taxonomy
- 明确哪些错误可 fallback

### Acceptance

- OpenAI、Anthropic、Gemini 的 single/combo 非流式测试通过
- 400/422 不 fallback，429/受控 transient error fallback
- 成功、失败、refresh 状态持久化一致
- tester 与生产请求使用相同解析与执行管线

## 37C Streaming and Resource Limits

### Scope

- 事件级 SSE parser 与 writer
- 正确处理多行 data、event、id、comment、DONE、EOF、scanner/read error 和 cancel
- 流式与非流式 timeout 分离
- request/header/multipart/upstream-error/non-stream response 大小限制

### Acceptance

- 超长或损坏事件不伪装成 `[DONE]`
- 客户端取消立即中止上游
- 长连接不被固定 5 分钟总 timeout 错误切断
- 各端点超限返回稳定错误 code

## 37D Active Network and File Safety

### Scope

- 自定义 BaseURL 与 panel-url URL policy
- redirect 逐跳验证和明确的 private-network 开关策略
- panel size、zip entry、zip-slip 与完整性检查
- secret/config 原子写入、权限和错误传播

### Acceptance

- IPv4/IPv6 loopback、link-local、private、redirect 绕过测试覆盖
- 超限 panel 或 entry 被拒绝而非截断
- 失败更新不会破坏旧配置
- Linux/macOS 敏感文件权限为 0600，目录为 0700

## 37F Database V2 and Persistence Boundaries

### Scope

- 按 `docs/DATABASE_V2.md` 重建未投产数据库 schema
- Connection、Credential、Runtime State、Routing 和 Request Record 分离
- Provider credential 加密存储，本地客户端 API key 只保存 hash
- 真正的 transaction migration、FK、索引、retention 和 integrity policy
- Repository 使用 context、窄 update 与事务，不公开裸 `*sql.DB`

### Acceptance

- `docs/DATABASE_V2.md` 的 Phase 37F 验收全部通过
- V1 数据库被明确拒绝并提示重建，不提供兼容迁移
- 前后端与新 Connection DTO、API key 一次性展示行为一致
- DB 并发、约束、索引和 secret contract tests 通过

## 37E Release Gate

### Scope

- 修复前四项剩余回归
- CI 加入 fmt/vet/test/race/vulnerability/frontend tests/build
- 核心后端 E2E

### Acceptance

- `docs/TEST_STRATEGY.md` 中发布命令全部通过
- P0/P1 finding 有测试或明确处置
- Phase 37 文档由“整改计划”转为“验证报告”

## 38A Contract and State Layer

### Scope

- 新 API resource naming 与 typed DTO
- Provider Catalog、Connection、Routing、Usage、Settings 状态拆分
- request cancellation、stale response protection、local mutation update
- 统一 API error model

### Acceptance

- 核心前端不再使用 `Record<string, any>` 承载 Provider/Connection 主契约
- mutation 不调用全量 `loadCore()`
- loading/error/empty 可被页面独立表达
- contract tests 通过

## 38B Provider Catalog and Connection Wizard

### Scope

- Connections 与 Provider Catalog 分页
- schema-driven 三步向导
- API Key、OAuth、Device Code、NoAuth、Custom endpoint 统一流程
- test-before-save

### Acceptance

- 用户不离开向导即可完成选择、认证、测试和保存
- 双认证 Provider 可明确选择方法
- 失败可重试、取消，不产生半成品 Connection
- 桌面与 375px 浏览器 E2E 通过

## 38C Connection Workspace and Route Inspector

### Scope

- Connection 详情、健康、额度、优先级、模型与错误
- Provider 多 Connection 管理
- Routing Workspace 和 Route Inspector
- 共享、可取消的 SSE tester

### Acceptance

- 页面术语与 `DOMAIN_MODEL.md` 一致
- Route Inspector 输出与实际请求 trace 一致
- tester 显示首 token、总耗时、usage 和结构化错误
- 不显示 secret 或敏感 header

## 38D Design System and Usability

### Scope

- Button、Card、Dialog、Field、Select、Segmented、Table、Confirm 和 Inline Alert
- 键盘、焦点、reduced motion、dark/light、移动端
- 清理交互卡片内嵌按钮冲突和只靠 toast 的反馈

### Acceptance

- 核心流程键盘可完成
- dark/light 对比度与状态可辨识
- 所有危险操作有一致确认和行级结果
- 浏览器视觉回归与关键组件测试通过
