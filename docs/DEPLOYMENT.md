# 部署与运维

## 安装

二进制名称包含版本、OS 和架构；验收前核对 SHA256SUMS。运行目录包含二进制、`knowledge/hpo`、配置和可写 `data`；官方数据可放到独立只读目录并通过 `serve -bundle-dir PATH` 加载。服务启动不下载或更新数据。

配置参考 `.env.example`。主密钥必须为 32 个随机字节的 base64，通过 `SEPILENS_MASTER_KEY` 或 `SEPILENS_MASTER_KEY_FILE` 提供；文件配置优先。密钥不入库，遗失后无法恢复数据。部署方限制密钥、配置、数据库和备份文件 ACL。认证使用 `SEPILENS_PRINCIPALS`，分别给 submitter、reviewer、admin 配置独立 Token。旧 `SEPILENS_API_KEYS=token:tenant` 仅得到 submitter 权限。`auth_mode=disabled` 仅用于隔离开发环境。

```sh
./sepilens serve -env-file .env -bundle-dir /opt/sepilens/bundle
```

`/healthz` 检查存活，`/readyz` 检查数据库可访问。SIGTERM/Ctrl-C 停止接受请求、终止 worker、释放连接。默认 2 个 worker、任务上限 300 秒、租约 30 秒；请求断开不取消已接受任务。单实例写 SQLite，禁止同时挂载到多副本。

## Docker

```sh
docker buildx build --platform linux/amd64,linux/arm64 -t sepilens:0.2.0-rc2 --output type=oci,dest=sepilens-images.tar .
docker load -i sepilens-images.tar
docker run --rm -p 127.0.0.1:8080:8080 --env-file .env \
  -e SEPILENS_ADDR=:8080 -e SEPILENS_MASTER_KEY_FILE=/run/secrets/master.key \
  --mount type=bind,src=/secure/master.key,dst=/run/secrets/master.key,readonly \
  --mount type=volume,src=sepilens-data,dst=/app/data sepilens:0.2.0-rc2
```

镜像为非 root 用户。绑定宿主目录时先给该用户读写数据目录权限；推荐命名 volume。官方 bundle 另作只读挂载并在启动参数中指定。不要把密钥编入镜像。首次 import 需要网络和已确认的锁定清单；正常运行可以完全离线。

本轮实际交付的 `sepilens-0.2.0-rc2-runtime-images.tar` 使用 `scripts/images.ps1` 将已测试的 Linux 二进制打包，两种架构镜像内的 binary SHA-256 与交付二进制相同。执行 `docker load -i dist/rc2/sepilens-0.2.0-rc2-runtime-images.tar` 后得到 `sepilens:0.2.0-rc2`。完整源码构建则使用上面的 Dockerfile 命令。发布步骤按顺序执行：`scripts/release.ps1` → `scripts/images.ps1` → 验收 → `python scripts/package.py`；最后一步统一计算所有归档校验和，避免对正在写入的归档取散列。

## 备份与恢复

1. 停止服务并确认进程退出，使 SQLite WAL 写入完成。
2. 备份整个 `data` 目录（数据库及可能存在的 `-wal`、`-shm` 文件），同时保留对应版本的二进制、bundle、批准包和配置。密钥单独安全备份。
3. 恢复到空目录，使用相同主密钥和同版本程序启动；确认健康检查、历史结果和审计读取。恢复后再执行升级。

禁止只复制正在使用的数据库文件。迁移工具会在旧 alpha 数据库进入 v3 前生成 `.pre-v3.bak`；该旧备份可能包含明文患者数据，需要立即按敏感备份保护或安全销毁。新的数据库敏感负载加密，索引元数据（tenant、任务 ID、状态、时间和散列）保持可查询。

## 密钥轮换

停止服务并备份后，设置旧 `SEPILENS_MASTER_KEY` 与新 `SEPILENS_NEW_MASTER_KEY`，执行：

```sh
./sepilens rotate-key
```

该子命令读取环境变量，**不会自动加载 .env**；`SEPILENS_DATA_DIR` 和认证环境也须配置。若设置 MASTER_KEY_FILE，其内容仍用于旧密钥。轮换在一个事务中重加密任务、结果历史、证据、审核及 profile；任一认证失败全部回滚。成功后替换挂载密钥，清除新密钥环境变量并以新密钥启动。旧密钥无法读取已轮换数据库，旧备份仍需要旧密钥。

## 保留与删除

`SEPILENS_RETENTION_DAYS` 默认 90，正整数；每小时清理超过保留期的终态任务及关联记录。活跃任务不会按时间自动删除。单任务 DELETE 可清理全部关联记录；admin 的 `DELETE /v1/tenant/data` 删除当前认证租户的任务、证据、审核和 profiles，保留防止标识重复利用的墓碑。备份、访问日志和外部模型方的数据由部署方按同一保留策略处理。

## 升级

先执行离线备份，再启动新版本，迁移按 `schema_migrations` 顺序执行。历史任务不静默删除；重复幂等键保留任务，只有一个活动键。旧来源不会自动变成 verified，旧结果不可结算。数据库迁移不能通过简单替换旧二进制回滚，必须恢复整个旧版本备份。
