# SepiLens API 接口文档（v1）

## 认证与通用请求头

- `Authorization: Bearer <API_KEY>` 或 `X-API-Key: <API_KEY>`
- `Idempotency-Key`: 客户端指定的幂等请求键（可选但推荐在创建评估任务时传入）
- `X-Request-ID`: 请求跟踪标识（可选，缺失时由网关自动分配）

所有响应均附带 `X-Request-ID` 响应头。

---

## 探针接口

### `GET /healthz`
- **说明**：服务存活性检查。
- **认证**：无需认证。
- **返回**：`200 OK`
```json
{ "status": "ok" }
```

### `GET /readyz`
- **说明**：服务就绪性检查（含 SQLite 存储连通性）。
- **认证**：无需认证。
- **返回**：`200 OK` 或 `503 Service Unavailable`。

---

## 元数据与能力接口

### `GET /v1/capabilities`
- **说明**：获取系统当前支持的引擎、检索模式及状态矩阵。
- **认证**：必需。

### `GET /v1/rulesets`
- **说明**：获取注册的规则集列表、版本及散列值。
- **认证**：必需。

### `POST /v1/providers/test`
- **说明**：测试自定义大模型 Base URL 与协议连通性，自动执行 SSRF 校验。
- **请求体**：
```json
{
  "protocol": "openai_responses",
  "base_url": "https://api.openai.com/v1",
  "model": "gpt-4o",
  "api_key": "sk-..."
}
```

---

## 评估任务接口

### `POST /v1/variants/snv/evaluations`
- **说明**：创建 SNV/InDel 评估任务（`engine_id=acmg_amp_2015` 或 `svc_v4`）。
- **请求配置参数 (`options`)**：
  - `web_search`: 布尔值（`true` / `false`），可选开启网络文献与数据库检索（NCBI PubMed / ClinVar / ClinGen）；
  - `retrieval_mode`: 检索模式字符串，可选 `"off"`（纯离线）、`"targeted"`（定向增强，默认最多 3 次请求）、`"deep"`（深度多轮，最多 10 次请求）。
- **校验拦截**：
  - `svc_v4` 永久返回 `409 RULESET_NOT_AVAILABLE`；
  - 缺失 MOI 或疾病时返回 `422 NEEDS_CLINICAL_CONTEXT`；
  - 坐标或参考碱基无效时返回 `422 INVALID_VARIANT`；
  - 基因组版本不支持时返回 `422 ASSEMBLY_MISMATCH`。

### `POST /v1/variants/cnv/evaluations`
- **说明**：创建 Constitutional CNV 评估任务（`cnv_loss_2020` 或 `cnv_gain_2020`）。
- **请求配置参数 (`options`)**：支持与 SNV 相同的 `web_search` 及 `retrieval_mode` 参数。
- **校验拦截**：
  - 引擎与变异类型（loss/gain）不匹配时返回 `422 INVALID_VARIANT`；
  - 坐标系统缺失时返回 `422 INVALID_COORDINATE_SYSTEM`；
  - 变异起源标记为 `somatic_confirmed` 时返回 `422 UNSUPPORTED_VARIANT_ORIGIN`。

### `GET /v1/jobs/{job_id}`
- **说明**：查询任务状态与执行进度。

### `GET /v1/jobs/{job_id}/events`
- **说明**：以 Server-Sent Events (`text/event-stream`) 方式实时推送执行阶段进度。

### `GET /v1/jobs/{job_id}/result`
- **说明**：获取结构化评估结论与审核记录。

### `POST /v1/jobs/{job_id}/cancel`
- **说明**：取消运行中的任务。

### `GET /v1/evaluations/{id}/evidence`
- **说明**：获取评估任务的不可篡改证据快照（SHA-256 散列锁定）、引用的来源条目与核验状态。
- **返回**：`200 OK`
```json
{
  "snapshot_id": "snp-job-123",
  "evaluation_id": "job-123",
  "hash": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
  "sources": [
    {
      "id": "SRC-PMID-25741868",
      "type": "pubmed",
      "title": "Standards and guidelines for sequence variant interpretation",
      "pmid": "25741868",
      "validation_status": "verified"
    }
  ],
  "items": [],
  "provenance": {
    "retrieval_mode": "targeted",
    "requests_count": 1,
    "duration_ms": 15
  }
}
```

### `POST /v1/evaluations/{id}/reviews`
- **说明**：追加人工复核记录。当规则引擎未产生有效结论时拒绝采纳引擎结果（409）；支持携带理由的人工覆盖判定（`human_override: true`）。
