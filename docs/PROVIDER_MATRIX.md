# Provider 协议与验证状态

| 协议 | 实现 | 离线测试 | 真实厂商验收 |
|---|---|---|---|
| openai_responses | /responses、JSON Schema、SSE、usage、store=false | 本地 HTTP 模拟服务器 | 未完成 |
| openai_chat | /chat/completions、response_format、SSE、usage | 本地 HTTP 模拟服务器 | 未完成 |
| Anthropic | 不支持 | 无 | 后续范围 |

两个适配器独立，不自动切换协议。模型和兼容厂商可能不支持某种参数；创建 profile 不等于通过能力测试。admin 的 POST `/v1/providers/test` 真正请求结构化输出和流式，可能产生费用。结果为 supported / unsupported / unknown；连接失败和缺失信息保持 unknown。native_web_search、function_calling、search_citations 不未经测试宣称支持。

profile 按租户版本化；任务只保存 profile ID/version，密钥加密且不读回。删除 profile 后无法执行依赖其密钥的新调用。默认环境模型用于 HPO，变异模型通过显式 `options.provider_profile_id` 选择。

真实联调需部署方提供测试端点、模型和专用密钥，分别保存两种协议的 Schema、流式、用量、失败和不支持场景报告。报告不得包含密钥、患者原文或完整提示。当前仓库不把 mock 通过写成厂商验证通过。
