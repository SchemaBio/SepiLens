# SepiLens 部署与运维指南

## 1. 运行环境要求

- **操作系统**：Linux (amd64 / arm64) 或 Windows
- **运行时环境**：单个无外部依赖的静态 Go 二进制文件，或 Docker 容器
- **存储**：本地持久化卷供 SQLite 数据库（WAL 模式）使用

---

## 2. 环境变量配置表

| 变量名 | 默认值 | 作用说明 |
|---|---|---|
| `SEPILENS_ADDR` | `:8080` | HTTP 服务监听地址与端口 |
| `SEPILENS_DATA_DIR` | `./data` | SQLite 数据库存储目录 |
| `SEPILENS_AUTH_MODE` | `required` | 认证模式：`required` 或 `disabled` |
| `SEPILENS_API_KEYS` | 空 | 允许的 API 密钥及租户映射，格式：`key1:tenant1,key2:tenant2` |
| `SEPILENS_SIMULATION` | `false` | 启用纯模拟任务测试模式 |
| `SEPILENS_LLM_BASE_URL` | `https://api.openai.com/v1` | 默认大模型 Base URL（亦支持 `OPENAI_BASE_URL`） |
| `SEPILENS_LLM_API_KEY` | 空 | 默认大模型 API Key（亦支持 `OPENAI_API_KEY`） |
| `SEPILENS_LLM_MODEL` | `gpt-4o` | 默认大模型名称（亦支持 `OPENAI_MODEL`，如 `deepseek-chat`、`gpt-4o`） |
| `SEPILENS_ALLOW_PRIVATE_ENDPOINTS` | `false` | 允许大模型连接本地/私有网络地址（用于内网部署私有模型） |
| `SEPILENS_LOG_FORMAT` | `json` | 日志输出格式：`json` 或 `text` |

---

## 3. Docker 容器化运行

### 3.1 构建镜像

```bash
docker build -t sepilens:v0.1.0 .
```

### 3.2 运行容器

```bash
docker run -d \
  --name sepilens \
  -p 8080:8080 \
  -v /var/data/sepilens:/app/data \
  -e SEPILENS_AUTH_MODE=required \
  -e SEPILENS_API_KEYS=prod-secret-key:cloud-platform \
  -e SEPILENS_LLM_BASE_URL=https://api.deepseek.com/v1 \
  -e SEPILENS_LLM_API_KEY=sk-your-deepseek-key \
  -e SEPILENS_LLM_MODEL=deepseek-chat \
  sepilens:v0.1.0
```

---

## 4. 本地二进制启动与环境配置

二进制启动时会自动检查并加载同目录下的 `.env` 文件。支持以下三种配置方式：

### 4.1 方式一：同目录下放置 `.env` 文件（推荐）

只需在编译出的二进制文件同级目录下放置 `.env` 文件，程序启动时自动识别并加载：

```env
# 在可执行文件同目录下创建 .env 文件
SEPILENS_ADDR=:8080
SEPILENS_DATA_DIR=./data
SEPILENS_AUTH_MODE=disabled

# 大模型配置（端点、Key、模型）
SEPILENS_LLM_BASE_URL=https://api.deepseek.com/v1
SEPILENS_LLM_API_KEY=sk-your-api-key
SEPILENS_LLM_MODEL=deepseek-chat
```

启动程序：
```bash
./bin/sepilens
```

如需显式指定其他路径的配置文件，可使用 `-env-file` 参数：
```bash
./bin/sepilens -env-file /etc/sepilens/prod.env
```

### 4.2 方式二：环境变量接入

```bash
export SEPILENS_LLM_BASE_URL="https://api.deepseek.com/v1"
export SEPILENS_LLM_API_KEY="sk-..."
export SEPILENS_LLM_MODEL="deepseek-chat"
./bin/sepilens -addr ":8080" -data-dir "./data"
```

### 4.3 方式三：命令行参数直接指定

```bash
./bin/sepilens \
  -addr ":8080" \
  -data-dir "./data" \
  -llm-base-url "https://api.openai.com/v1" \
  -llm-key "sk-..." \
  -llm-model "gpt-4o"
```

### 4.4 方式四：接入本地/私有化 Ollama 或 vLLM

若模型端点在本地或局域网内，需在 `.env` 或环境变量中开启私网放行：

```env
SEPILENS_ALLOW_PRIVATE_ENDPOINTS=true
SEPILENS_LLM_BASE_URL=http://127.0.0.1:11434/v1
SEPILENS_LLM_API_KEY=ollama
SEPILENS_LLM_MODEL=qwen2.5:14b
```

---

## 5. SQLite 备份与恢复

系统默认开启 SQLite WAL（Write-Ahead Logging）模式与并发安全保障：

- **热备份**：使用 `sqlite3 /app/data/sepilens.db ".backup /app/data/backup_$(date +%Y%m%d).db"`
- **数据恢复**：停止服务，替换 `sepilens.db` 文件后重启即可。
