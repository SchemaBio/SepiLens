# SepiLens

SepiLens 是供 YiJian 调用的 Go 变异评估服务。YiJian 提交结构化注释、剂量参考和临床证据 JSON，SepiLens 返回条款、证据强度、缺口和结论。不内置或查询医学注释数据库；任务、审核和审计使用加密 SQLite 业务存储。

支持 ACMG/AMP 2015、SVC v4 锁定草案、CNV Gain 和 CNV Loss 的本地／LLM 联网入口。本地同步返回，联网异步执行。SVC v4 始终为实验用途，仍有上游草案的适用性缺口；其他医学引擎需独立病例验证和签名批准。所有医学引擎及 AI 默认禁用。模型输出经校验后由规则引擎重算，工程交付不代表医学批准。

## 启动

需要 Go 1.26。构建 `go build -o sepilens.exe ./cmd/sepilens`（Linux 输出 sepilens），复制 `.env.example` 为 `.env`，配置 API Key、32字节主密钥及来源策略，然后运行 `sepilens serve -env-file .env`。公网部署使用 HTTPS 反向代理。无需医学数据库或资源下载。

`GET /healthz`、`GET /readyz` 用于探针；带 `Authorization: Bearer KEY` 调用 `GET /v2/capabilities` 查看固定五个引擎状态。POST `/v2/evaluations` 的 schema_version 固定 `sepilens-evaluation-2.0`；API Key 绑定租户、服务身份和提交权限。

## 对接与验证

- [YiJian 对接契约](docs/YIJIAN_INTEGRATION.md)、[API](docs/API.md)、[OpenAPI](docs/openapi.yaml)、[合成示例](docs/examples/v2)
- [开发进度](docs/DEVELOPMENT_PLAN_NEXT.md)、[部署与不兼容迁移](docs/DEPLOYMENT.md)、[安全](docs/SECURITY.md)
- [验证状态](docs/VALIDATION.md)、[规则批准](docs/RULESET_POLICY.md)

运行 `go test ./...`、`go vet ./...` 和 `python scripts/openapi.py --check`；Linux 额外运行 `go test -race ./...`。双平台使用 `python scripts/smoke.py --binary PATH --output REPORT`，不挂载医学数据库。真实厂商联网验收和独立临床黄金案例仍需分别完成。

本次 v2 是不兼容升级：旧 API、医学数据导入／索引及历史兼容层已移除。新存储默认 `data/sepilens-v2.db`，旧数据库拒绝打开且不自动删除。旧 RC 安装包和历史验收报告不代表当前版本通过同样验收。
