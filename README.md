# SepiLens

SepiLens 是单机私有部署的 Go 医学证据 API。首版工程候选提供 SQLite 加密任务、HPO 候选提取、两个真实 OpenAI 协议适配器、官方接口检索、确定性规则、证据审计和人工审核。

**当前发布状态：工程候选，医学引擎默认全部禁用。** SVC v4 已提供[锁定草案实验评分](docs/SVC_V4_EXPERIMENTAL.md)，需显式 allow_experimental；禁止临床用途与结算，普通请求仍返回 409。HPO 候选提取可用，但不能视为已获医学批准的自动诊断服务。ACMG、CNV Loss/Gain 只在独立案例、规则版本、可执行文件散列和人工签名批准同时有效时启用。模型建议永远不直接成为正式计分证据。

## 快速启动

需要 Go 1.26。在 PowerShell 中：

```powershell
go build -trimpath -o sepilens.exe ./cmd/sepilens
New-Item -ItemType Directory -Force secrets | Out-Null
[Convert]::ToBase64String([Security.Cryptography.RandomNumberGenerator]::GetBytes(32)) | Set-Content -Encoding ascii secrets/master.key
Copy-Item .env.example .env
# 编辑 .env，替换三个认证 Token，并限制 .env 和 secrets/master.key 的访问权限。
./sepilens.exe serve -env-file .env
```

Linux：用 `openssl rand -base64 32 > secrets/master.key` 生成密钥，`chmod 600` 保护密钥及配置，随后执行 `./sepilens serve -env-file .env`。Linux 文件名区分大小写。默认服务仅在示例配置的回环地址监听；公网接入由部署方提供 HTTPS 反向代理。

```sh
curl http://127.0.0.1:8080/healthz
curl -H 'Authorization: Bearer YOUR_TOKEN' http://127.0.0.1:8080/v1/capabilities
```

不提供模型时，HPO 只有显式 `options.allow_local_fallback=true` 才会使用本地字面匹配，并返回降级标记。正式使用应加载许可已确认的官方锁定数据包；仓库原有中文资源许可尚未确认，不能作为发布批准材料。

## 交付资料

- [安装、镜像、备份、恢复和密钥轮换](docs/DEPLOYMENT.md)
- [API 与 alpha 迁移说明](docs/API.md)、[OpenAPI](docs/openapi.yaml)
- [数据导入和版本锁定](docs/DATA_IMPORT.md)
- [验证方法与未完成验收项](docs/VALIDATION.md)
- [14 项复核关闭记录](docs/REVIEW_CLOSURE.md)
- [安全与保留策略](docs/SECURITY.md)、[规则批准门禁](docs/RULESET_POLICY.md)

开发检查：`go test ./...`、`go vet ./...`；Linux 额外执行 `go test -race ./...`。`scripts/release.ps1` 构建 Windows amd64、Linux amd64/arm64 二进制及依赖清单。Dockerfile 支持两个 Linux 架构，使用非 root 用户。真实厂商联调和专家验收独立于离线 CI。

不在本轮交付范围：Anthropic、PostgreSQL 集群、Web 控制台和通用网页搜索。详见 [交付进度](docs/DEVELOPMENT_PLAN.md)。

当前 RC2 安装包与双架构镜像位于 dist/rc2，SHA256SUMS 与 release-manifest.json 同目录；RC1 保留在 dist。草案评分示例及临床验收待办见 docs/SVC_V4_EXPERIMENTAL.md。
