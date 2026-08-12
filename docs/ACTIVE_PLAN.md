# Active Plan

## 当前目标

在首次公开部署前完成安全加固、核心代理可靠性重构与 WebUI Provider 工作流重做。

## 执行顺序

1. Phase 37A：默认安全边界、secret DTO、登录 limiter、密码与 secret 生命周期
2. Phase 37B：统一 upstream executor，修正 combo/messages/translation/fallback
3. Phase 37C：SSE、timeout、request/response limits
4. Phase 37D：SSRF、panel 下载、原子文件写入
5. Phase 37E：Go race/vet/test/E2E 发布门禁
6. Phase 38A：API contract 与前端状态层
7. Phase 38B：Provider Catalog 与连接向导
8. Phase 38C：Connection Workspace、Route Inspector、流式 tester
9. Phase 38D：设计系统和浏览器可用性验收

## 必读文件

- `docs/ARCHITECTURE_DECISIONS.md`
- `docs/SECURITY_AUDIT_2026-08-12.md`
- `docs/FRONTEND_AUDIT_2026-08-12.md`
- `progress.json`

## 禁止事项

- 不为未投产旧版本保留兼容层
- 不在 Phase 37 完成前新增 Provider 或大规模视觉功能
- 不直接把持久化模型序列化为管理 API 响应
- 不在多个 handler 复制上游执行逻辑
- 不用全量 `loadCore()` 掩盖前端状态问题
