# Product Roadmap

本文描述 Phase 37-38 之后的产品方向。它不是当前任务清单，未进入 `progress.json.active_phases` 的内容不得提前开发。

## 产品定位

Cyrene Gateway 的近期目标是成为可靠、可解释、适合本机和小团队使用的 AI Provider Gateway：

- 向客户端提供稳定的 OpenAI-compatible API
- 将 Provider、Connection、Model、Routing 四个领域概念清晰分离
- 支持多账号、fallback、转换、额度和健康管理
- 让用户能解释每次请求为什么选择某个上游
- 保持单二进制、低依赖、可本地运行

近期不追求：企业级多租户、集群调度、复杂 RBAC、云控制平面或无限扩张 Provider 数量。

## Release 0：可安全开发

对应 Phase 37。

结果：默认安全、核心代理链路一致、SSE 正确、资源边界明确、动态测试通过。

## Release 1：管理面板产品可用

对应 Phase 38。

结果：Provider Catalog、Connection Workspace、统一连接向导、Route Inspector 和可靠状态管理完成。

## Release 2：路由与可观测性闭环

候选 Phase 39，必须在 Phase 38 完成后细化。

- 请求详情显示完整解析链，但不记录 secret 和敏感 prompt
- Alias、Combo、Connection priority 在统一 Routing Workspace 管理
- 明确错误 taxonomy：validation、auth、quota、rate-limit、transport、translation、upstream
- 健康状态基于近期请求和主动测试，不只依赖单次 test
- 支持导出脱敏诊断包
- 用量、成本和配额数据建立一致口径

## Release 3：Provider Conformance

候选 Phase 40。

- 建立 Provider contract 和 conformance test harness
- 每个 curated Provider 声明认证、格式、stream、tools、vision、models、quota 能力
- 使用 mock upstream 验证 URL、headers、body、translation、SSE 和错误分类
- Provider 只有通过 contract tests 才能进入 curated registry
- 删除未深度适配或长期不可验证的 Provider

## Release 4：配置、备份与恢复

候选 Phase 41。

- 导出不含 secret 的配置模板
- 可选加密备份，明确密钥与恢复流程
- 数据库 schema version 与破坏性迁移策略
- Reset application、清理缓存、重建索引
- 诊断与恢复模式

## Release 5：首次公开发布

候选 Phase 42。

- 安装、升级、卸载与数据目录说明
- Windows/macOS/Linux 安装包或可信二进制
- SBOM、校验和、依赖漏洞扫描
- Release notes、known limitations、security reporting
- 新用户完整 onboarding
- 性能基线与容量说明

## 延后决策

以下内容只有出现真实需求后再评估：

- 多用户与 RBAC
- Postgres 和多实例部署
- 云端控制平面
- 插件市场
- 远程第三方面板分发
- 自动执行高权限 MITM/DNS 操作
- 大规模 Provider 数量扩张
