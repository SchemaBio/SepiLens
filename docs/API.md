# SepiLens API v2

统一接口为 `POST /v2/evaluations`。本地同步返回结果，LLM 联网异步返回任务 ID；四引擎使用同一结构化证据包。医学数据库由 YiJian 管理，SepiLens 不依赖本地注释数据。

- [完整对接契约](YIJIAN_INTEGRATION.md)
- [OpenAPI](openapi.yaml)，通过 `python scripts/openapi.py` 重建，`--check` 校验漂移。
- [合成请求示例](examples/v2)

全部 `/v1` 路由已移除。旧 `data import|verify`、`serve -bundle-dir`、来源状态与本地表型库入口已移除。本次升级不迁移或兼容旧任务、快照和数据库。使用新的数据目录；不要删除旧目录或用旧二进制打开新数据库。

新版本每次评估和修订仍保存加密快照与审计记录。已有部分结果的 AI 失败任务可返回 HTTP 200，调用方必须检查任务状态和 delivery_status，不能据 HTTP 200 判断成功或结算。

## AI HPO 术语提取

POST /v2/hpo/extract，使用现有 Bearer 服务密钥。请求包含 text（1–10000 个字符）和 options.provider_profile_id，可选 max_candidates（最多100）。仅使用 AI，allow_local_fallback 必须为 false。响应为同步的 HPO ExtractionResult，execution_mode=model、degraded=false，terms 含 canonical HPO ID、polarity、Unicode codepoint span_start/span_end，候选编号和原文位置经过校验；否定、疑似和家族史须单独复核。模型失败返回错误，不返回本地匹配结果。此接口不建立持久化任务，不保存原始文本。需 SEPILENS_AI_MODE=enabled 和已保存的模型 profile。

SEPILENS_HPO_KNOWLEDGE_DIR 默认 knowledge/hpo；镜像内置作者授权的 DeepHPO 术语与名称资源，仅用于候选匹配和 ID 校验。它不替代上游提供的医学注释证据。YiJian 的本地自动提取独立在浏览器完成，点击 AI 辅助提取才调用此接口。
