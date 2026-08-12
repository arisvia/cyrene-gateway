# Domain Model

本文定义后端、API、前端和文档必须统一使用的核心术语。

## Provider

供应商能力定义，来自 curated registry。描述品牌、区域、支持能力、认证方法、transport 和可用模型来源。Provider 不包含用户 secret。

例：`openai`、`anthropic`、`gemini`。

## Connection

用户创建的 Provider 账号或上游连接实例。包含认证方式、加密或受保护的 credential、优先级、启停状态、健康和额度状态。

一个 Provider 可以有多个 Connection。

## Model

Provider 暴露的具体模型。必须记录来源：registry、upstream fetch、custom。模型 ID 不等于客户端最终使用的路由名称。

## Alias

客户端模型名到一个目标模型或 Combo 的显式映射。

## Combo

一个有序候选路由集合及其选择策略。Combo 不直接执行协议转换，所有候选仍交由统一 upstream executor。

## Route Resolution

从客户端输入模型到最终请求尝试的纯解析结果：

```text
input model
 -> alias or combo
 -> provider/model candidate
 -> eligible connection candidates
 -> selected connection
 -> transport and target format
```

解析器应可独立测试，并能输出供 Route Inspector 使用的脱敏 trace。

## Credential

Connection 中的敏感认证材料。不得直接出现在 API response、日志、usage、trace 或前端 store 中。

## Provider Catalog 与 Connection Workspace

- Provider Catalog 用于发现 Provider 和创建 Connection。
- Connection Workspace 用于管理一个具体 Connection。
- Routing Workspace 用于 Alias、Combo 和 Route Inspector。

禁止再用“Provider Detail”指代 Connection 详情。
