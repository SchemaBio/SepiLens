# SepiLens 大模型 Provider 适配矩阵

| 厂商 / 端点 | 协议标识 | 结构化输出支持 | 流式 SSE | 原生联网搜索 | 推荐生产配置 |
|---|---|---|---|---|---|
| **OpenAI Responses API** | `openai_responses` | 支持（JSON Schema） | 支持 | 支持 | 推荐首选 |
| **OpenAI Chat Completions** | `openai_chat` | 支持（json_object） | 支持 | 需由 SepiLens 检索层代理 | 兼容支持 |
| **DeepSeek (OpenAI 兼容端点)** | `openai_responses` | 支持 | 支持 | 需由 SepiLens 检索层代理 | 生产测试验证通过 |
| **私有化 vLLM / Ollama** | `openai_responses` | 支持 | 支持 | 本地禁用联网 | 需开启 `ALLOW_PRIVATE_ENDPOINTS` |

## 联网与检索隔离原则

- 当外部供应商端点不具备原生联网工具能力时，SepiLens 自动由内置的 `SearchCoordinator` 通过专用 NCBI PubMed、ClinVar 及 ClinGen 接口提供权威来源补全；
- 绝不假定兼容 Responses 接口的第三方端点自动支持 `web_search` 工具调用。
