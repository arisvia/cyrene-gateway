# Active Architecture Decisions

本文只记录当前有效决策。历史阶段与旧决策见 `docs/progress-full-before-phase37.json`。

## AD-001 不保留未投产版本兼容

项目尚未投产，Phase 37-38 可以修改 API、数据库 schema、配置字段、路由和 WebUI 状态模型。禁止为旧接口增加双写、双字段或长期 compatibility shim。修改后同步更新调用方、测试和文档。

## AD-002 删除 CYRENE_DATA_DIR 外部配置

删除 `CYRENE_DATA_DIR` 环境变量。运行数据固定在当前用户的 `~/.cyrene-gateway/`。代码内部保留 `Config.DataDir` 作为依赖注入点，供测试和组件构造使用，但它不再是用户配置面。

如未来需要 portable mode，应单独设计明确的 `--portable` 或实例 profile，而不是恢复隐式环境变量覆盖。

## AD-003 安全优先于新增功能

Phase 37 完成前不新增 Provider、媒体能力或视觉特效。P0 安全问题和代理可靠性是发布门禁。

## AD-004 Provider 与 Connection 分离

Provider 是注册表中的供应商能力定义；Connection 是用户创建的账号/凭据实例。API、DTO、路由、Store 和页面必须使用一致术语。

## AD-005 Schema-driven Connection Flow

API Key、OAuth、Device Code、NoAuth 与 Custom endpoint 使用后端提供的连接方法 schema，前端不再按 category 硬编码流程。

## AD-006 统一 Upstream Executor

single model、combo、messages、tester 共用请求执行管线，统一 translation、auth、refresh、fallback、SSE、usage 与状态更新。
