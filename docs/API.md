# SepiLens API v2

统一接口为 `POST /v2/evaluations`。本地同步返回结果，LLM 联网异步返回任务 ID；四引擎使用同一结构化证据包。医学数据库由 YiJian 管理，SepiLens 不依赖本地注释数据。

- [完整对接契约](YIJIAN_INTEGRATION.md)
- [OpenAPI](openapi.yaml)，通过 `python scripts/openapi.py` 重建，`--check` 校验漂移。
- [合成请求示例](examples/v2)

全部 `/v1` 路由已移除。旧 `data import|verify`、`serve -bundle-dir`、来源状态与本地表型库入口已移除。本次升级不迁移或兼容旧任务、快照和数据库。使用新的数据目录；不要删除旧目录或用旧二进制打开新数据库。

新版本每次评估和修订仍保存加密快照与审计记录。已有部分结果的 AI 失败任务可返回 HTTP 200，调用方必须检查任务状态和 delivery_status，不能据 HTTP 200 判断成功或结算。
