# 部署、存储和不兼容升级

本版本不需要医学注释数据库、bundle 或启动下载；HPO AI 提取使用镜像内置的术语字典。YiJian 在 API 请求中提供材料。启动前设置 `.env.example` 中的主密钥、API Key、提交权限和来源策略，参见 [对接契约](YIJIAN_INTEGRATION.md)。`serve -rules-dir DIR` 加载独立验证报告和签名批准；默认不启用医学引擎。AI 默认 disabled，启用后还需版本化 provider profile。

## 业务存储

默认 data/sepilens-v2.db，格式标识 PRAGMA application_id=0x53455032。已有非该格式数据库通过只读探测拒绝，错误提示使用新目录；不迁移、不覆盖、不自动删除旧文件。旧版 data/sepilens.db 不会被默认选中。回滚使用旧二进制及旧目录，不用旧二进制打开新存储。

新版本任务、输入、结果、快照、审核、修订、工具调用和 profile 密钥继续加密保存，按租户隔离。默认两个 worker，任务上限300秒；LLM循环上限180秒。单实例写 SQLite，不允许多个实例同时挂载。请求断开不删除已提交异步任务。

## 备份、保留和密钥

停止服务并确认进程退出后备份整个业务数据目录、对应程序和来源策略／批准配置，主密钥单独安全保管。恢复到空目录，用相同版本和主密钥启动。不要只复制正在使用的主数据库文件而遗漏 WAL。

保留期默认90天，每小时清理到期终态任务。admin 可 DELETE /v2/jobs/{id} 或 /v2/tenant/data；审核需要 reviewer/admin。删除不保证物理介质擦除，应保护 WAL 和备份。外部模型的数据保留另由合同和配置约束。

轮换密钥：停止服务、备份后设置旧 SEPILENS_MASTER_KEY（或 MASTER_KEY_FILE）和 SEPILENS_NEW_MASTER_KEY，执行 `sepilens rotate-key`；该命令不自动加载 .env。事务重加密所有敏感负载，成功后更新运行密钥并清除临时环境变量。

## 网络与镜像

探针 /healthz、/readyz 不要求 API Key，其他路径需要认证。HTTPS 反向代理由部署者提供。模型默认仅公网 HTTPS，显式配置才允许私网模型；官方检索保留默认 SSRF 防护。日志不含患者内容、Key 或完整外部错误。

Dockerfile 和 scripts/images.ps1 不再复制医学参考资源；镜像以非 root 用户运行，仅挂载业务数据和受保护的密钥／配置。使用新命名卷部署，保留旧卷作为独立归档。双平台 smoke 使用 scripts/smoke.py；OCI 构建和容器运行需本机 Docker，未运行时不声称容器验收通过。

## 仅接入 AI 与可选规则批准

设置 SEPILENS_AI_MODE=enabled、SEPILENS_AI_ONLY=true，仅接受 mode=ai_assisted，拒绝 mode=local。默认 SEPILENS_REQUIRE_RULE_APPROVAL=true 保持签名批准门禁；自部署 AI 证据整理可设为 false，不加载批准文件也能提交 AI 任务。未批准引擎的结果为 evidence_only、clinical_use=forbidden、settlement_eligible=false，引擎 enabled 仍为 false；能力中的 ai_available 独立表示 AI 入口可用。来源校验、模型校验和人工审核仍执行；失败不回退为本地评估。SVC 始终为实验模式。
