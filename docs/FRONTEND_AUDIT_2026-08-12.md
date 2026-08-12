# WebUI 产品逻辑与体验审计

- 日期：2026-08-12
- 范围：Vue 3 WebUI 的信息架构、状态管理、Provider 添加/管理、模型转换与测试链路、组件视觉基础
- 结论：视觉质感确实有提升空间，但当前更大的问题是领域模型和页面任务不匹配。应先重构 Provider 工作流，再统一组件视觉。
- 动态验证：Node 可用，但压缩包未包含 `node_modules`，当前环境无法直接运行 Vitest 与 Vite build。Phase 38 完成前必须执行 `npm ci && npm test && npm run build`，并浏览器走查。

## 一、核心问题

### 1. “Provider 类型”和“连接账号”混在同一页面

`ProvidersView` 同时展示：

- 已创建的 connection
- 可选 Provider registry
- 品牌与区域切换
- 免费 Provider 启用
- API Key Provider 添加
- OAuth Provider 连接
- 自定义 OpenAI-compatible endpoint
- Test All

这使用户无法快速回答三个问题：

1. 我已经连接了哪些账号，它们是否健康？
2. 我还能添加哪些供应商？
3. 某个品牌有多少账号、区域、可用模型和路由优先级？

**建议**：拆成两个明确任务。

```text
Connections
  管理已连接账号、健康状态、优先级、额度、模型和失败原因

Provider Catalog
  发现并添加供应商，按能力、认证方式、区域和免费属性筛选
```

首页默认进入 Connections。没有连接时再显示 Catalog onboarding。

### 2. Add Provider 对话框逻辑断裂

顶部“Add Provider”打开 Registry tab，但弹窗本身没有可选择列表，只提示用户“Pick a provider from the list above”。用户需要关闭或绕开弹窗，再回页面卡片选择，流程不闭环。

**建议**：改为三步向导。

```text
Step 1 选择 Provider
  搜索、能力、认证方式、区域、免费筛选

Step 2 选择连接方式
  API Key / OAuth / Device Code / NoAuth / Custom Endpoint

Step 3 配置与验证
  名称、凭据、Base URL、可用模型 -> Test Connection -> Save
```

向导必须由注册表 schema 驱动，不在 Vue 页面里用 category 分支猜测行为。

### 3. authType 与 authModes 没有成为真正的产品状态机

当前添加逻辑大多使用 `addSelected.authType`，对 `authModes`、device code、OAuth callback、freeTier 和 noAuth 没有形成统一流程。Provider 卡片只按 category 决定按钮文案，不能准确表达双认证 Provider。

**建议**：后端输出统一的 connection schema：

```text
connectionMethods[]
  id
  label
  kind: apiKey | oauth | deviceCode | noAuth | custom
  fields[]
  action
  availability
  helpUrl
```

前端只渲染 schema 和调用统一 action。Provider 特例留在后端适配层。

### 4. Store 是全局“大仓库”，刷新粒度过大

每次添加、删除、开关 Provider 后都会 `loadCore()`，它并行请求七个无关接口，然后又不等待地加载 Keys 和 Proxy Pools。结果是：

- 页面局部操作触发全局刷新
- 无统一 loading/error 状态
- 请求竞态可能让旧响应覆盖新状态
- 错误大多只写 console 或被静默吞掉
- 页面无法显示“局部失败但其他数据可用”

**建议**：按领域拆 store，或至少拆 query state。

```text
connectionStore
providerCatalogStore
modelStore
routingStore
usageStore
settingsStore
```

每个资源保留 `data/loading/error/loadedAt/requestId`，mutation 成功后局部更新或刷新对应资源，不再调用全量 `loadCore()`。

### 5. 前后端契约大量使用 any 和 Record<string, any>

Provider data、settings、usage、quota 和 registry 中心字段缺少判别联合类型。页面通过可选链猜测 payload，契约错误只能在运行时出现。

**建议**：由后端维护 API contract，并生成 TypeScript 类型，至少先手工建立 discriminated union。Provider secret DTO 与前端响应 DTO 必须同时重构，前端不应再接收完整 token。

### 6. Provider Detail 的概念层级错误

路由参数是 connection ID，但页面叫 Provider Detail；“Connections” Tab 实际只展示当前一个 connection；其中“Add Key”会创建一个兄弟 connection，操作完成后仍停留在旧 connection 页面，用户难以理解层级。

**建议**：明确两个层级。

```text
/provider-types/:providerId
  品牌、区域、支持模型、连接方式、文档

/connections/:connectionId
  单个账号的凭据状态、额度、模型覆盖、优先级、代理、错误与测试
```

若要在 Provider 页面管理多个账号，则页面路由应以 provider ID 为主，左侧列出账号，右侧编辑选中账号。

### 7. “模型转换”和“实际请求模型”不透明

当前模型列表混合 registry 与 custom model，但用户看不到：

- 客户端发送的模型名
- alias 解析结果
- combo 解析结果
- 最终 provider/model
- transport format
- 是否经过 OpenAI/Anthropic/Gemini 转换
- 使用哪个 connection 和 fallback 顺序

**建议**：增加 Route Inspector。

```text
Input model
 -> alias/combo
 -> provider model
 -> connection
 -> transport format
 -> target endpoint
```

测试器应显示这条解析链、首 token 延迟、总耗时、token usage 与失败分类，而不只是输出文本。

### 8. 页面内 SSE 测试器再次手写了脆弱解析逻辑

Tester 按换行拆 `data:`，与后端 SSE 的多行事件问题相似。应抽出共享 SSE client，支持 AbortController、事件级解析、超时、取消和结构化错误。

### 9. 危险操作和批量操作缺乏产品约束

- Test All 最多并发 10 个，缺少进度、取消和重试策略。
- Disable、Reset Cooldown 与删除的反馈层级不统一。
- Provider 首页点击卡片与点击按钮存在双重交互语义。
- 免费 Provider 的“启用”直接执行，API Key/OAuth 却进入不同流程。

建议为所有异步 action 建立统一状态：idle/running/success/error/canceled，并提供行级反馈，不依赖全局 toast 作为唯一结果。

## 二、目标信息架构

```text
Overview
  健康状态、请求量、异常连接、最近失败

Connections
  已连接账号列表
  筛选：Provider / Auth / Region / Health
  批量测试、启停、优先级排序

Provider Catalog
  发现与添加供应商
  能力、认证、地区、免费属性筛选
  统一连接向导

Routing
  Aliases
  Combos
  Route Inspector

Models
  按 Provider/Connection 查看
  registry / fetched / custom 来源
  enable/disable 与 alias

Observability
  Requests
  Usage
  Logs

System
  API Keys
  Proxy Pools
  Tunnel / MITM
  CLI Tools
  Settings
```

## 三、组件与视觉整改

视觉放在逻辑重构后进行，但可以同步建立基础组件：

- `GButton`：solid / soft / outline / ghost / danger，统一 hover、pressed、focus ring、loading 宽度
- `GCard`：interactive 与 static 分开，避免整卡可点击时内部又有按钮
- `GModal` 升级为 `GDialog`：焦点锁定、Esc、返回焦点、滚动锁、描述绑定
- 新增 `GField`、`GSelect`、`GSegmented`、`GDataTable`、`GConfirmDialog`、`GInlineAlert`
- 减少大面积玻璃拟态，数据密集页面优先清晰边界与层级
- 统一 4/8px spacing、按钮高度、圆角和阴影层级
- 所有交互态满足键盘操作与 `prefers-reduced-motion`

## 四、Phase 38 推荐拆分

### 38A API Contract 与状态层

- Provider/Connection DTO 分离并脱敏
- 将前端 `any` 收敛为判别联合
- 拆分 store 和局部 query state
- 统一 error/loading/cancel

### 38B Provider Catalog 与连接向导

- Connections 与 Catalog 分页
- schema-driven 三步向导
- API Key/OAuth/Device Code/NoAuth/Custom 统一状态机
- 保存前必须 Test Connection

### 38C Connection Workspace 与 Route Inspector

- 明确 Provider 与 Connection 层级
- 多账号优先级、健康、额度和错误
- 模型解析/协议转换/fallback 可视化
- 统一流式 tester

### 38D Design System 与可用性

- 重做 Button/Card/Dialog/Form/Table
- 危险操作确认、行级反馈、键盘与移动端
- dark/light 浏览器视觉验收

## 五、完成标准

- [ ] Add Provider 无需离开弹窗即可完成选择、认证、测试和保存
- [ ] 所有认证模式均由同一 schema 状态机驱动
- [ ] Provider、Connection、Model 和 Routing 概念在路由与页面上清晰分离
- [ ] mutation 不再触发全量 `loadCore()`
- [ ] 前端不接收完整 provider secret
- [ ] Route Inspector 能解释模型如何到达最终上游
- [ ] SSE tester 可取消且正确处理事件与错误
- [ ] `npm test` 和 `npm run build` 通过
- [ ] 核心用户路径有组件测试和浏览器 E2E
