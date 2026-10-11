# API 与 alpha 迁移

完整路径及结构见 `openapi.yaml`（OpenAPI 3.1，JSON 是合法 YAML 子集）。通过 `python scripts/openapi.py` 从 Go 公共结构及路径说明重建；错误统一为 `error.code,message,request_id,retryable`。JSON 请求拒绝未知字段、重复顶层对象及超过 2 MiB 的输入。

认证：`Authorization: Bearer TOKEN`。Token 决定 tenant/actor/role，不接受客户端冒充审核人员。HPO 为同步候选提取；变异评估在事务保存输入后返回 202，而不是等待模型。首次响应 `job_id,status,result_kind,is_replayed`；GET jobs/result 在待执行时 202、失败 409、取消 410、交付完成 200。

## 最小请求

```json
{"engine_id":"acmg_amp_2015","variant":{"assembly":"GRCh38","chrom":"1","pos":100,"ref":"A","alt":"G","gene":"TEST"},"clinical_context":{"disease_id":"MONDO:TEST","inheritance":"AD","variant_origin":"unknown","sample_type":"blood","hpo_terms":[]},"options":{"retrieval_mode":"off"}}
```

POST `/v1/variants/snv/evaluations` 并设置 `Idempotency-Key`。unknown 来源只生成 evidence_only，没有正式胚系分类且不可结算。改为 germline_confirmed 时未批准引擎返回 409；somatic / somatic_confirmed 返回 422。

```json
{"text":"患者否认癫痫，家族有听力下降。","options":{"allow_local_fallback":true}}
```

POST `/v1/hpo/extract`。返回候选集内 HPO、Unicode 字符跨度 `[span_start,span_end)`、原文症状、present/absent/uncertain/family_history/inferred 与来源。local_fallback 必须显式允许，始终需要专业复核。原文不持久化。

CNV 支持 GRCh37/38；`coordinate_system` 只允许两种枚举，half-open 转换为 inclusive 后持久化。Loss/Gain 类型必须匹配引擎。AF 必须在 [0,1]；ref/alt 为不同的大写 ACGTN 序列。gene、disease、inheritance 和 sample_type 必须提供适用上下文。省略或未确认的结构材料不自动满足规则。

## Profile 与审核

admin PUT `/v1/providers/{profile_id}` 提交 `base_url,api_key,model,protocol`；每次保存递增版本；GET 不含密钥；DELETE 删除该租户配置。protocol 明确选择 openai_responses/openai_chat。POST `/providers/test` 执行真实调用并返回能力状态，可能产生费用。

reviewer/admin POST `/v1/evaluations/{id}/reviews`，传 `revision,decision,reason`，reviewer_id 省略时采用当前 actor，提供时必须一致。decision：`accept_engine_result`、`reject`、`override`、`revise_evidence`；override 自动标记 human_override=true，必须提供有效 reviewed_classification。采纳仅针对已完成 production 的有效规则结果。revision 过期返回 409。

revise_evidence 提交 `corrections`（EvidenceItem）、`verified_sources`（已解析来源 ID）和核验理由，专业人员确认对象与材料后重新确定性计算；生成新结果 revision、新快照，旧版本不修改。核验来源需已证明 identifier_valid 与 record_exists，不能仅凭 ID 格式计分。facts 列表见导出的规则包；CNV suggested_score 是两位小数字符串（如 `0.45`），整数 rubric 不使用浮点。

## SSE、结算和迁移

GET `/v1/jobs/{job_id}/events` 是持续 SSE，支持 Last-Event-ID 和 15 秒心跳；终态关闭。断开订阅不会取消任务。显式 POST cancel 才取消。

只在结果、证据、终态、完成事件事务提交后 delivered。平台必须同时检查 production、settlement_eligible、delivery_status；模拟、失败、取消、历史及 evidence_only 均不可结算。insufficient_data 只有启用引擎正常生产交付时可结算。已有结果读取免费；需要重新模型生成时使用新幂等键。同键同规范化输入及相同锁定 profile/规则/数据版本返回旧任务；版本改变可能返回 409。

alpha 升级后五引擎默认禁用；旧 verified 不再可信，旧结果 wrapped 为 legacy_unverified，不可采纳或结算。迁移保留重复历史任务，只移除冲突活动幂等键。旧活动键保持原形式，新键散列保存，不保证旧 alpha 请求的幂等重放；调用方应先按旧 job_id 读取，显式重新生成才创建新键。审核请求增加 revision，profile 引用替代任务内密钥，结果增加版本、执行模式、缺口、来源和审核状态。先备份再迁移，详见 DEPLOYMENT。

## SVC v4 实验接口

SNV 请求使用 svc_v4、draft_assessment 和 options.allow_experimental=true。结果仅位于 agent_proposal.experimental_result，禁止临床审核采纳与结算；见 [详细说明](SVC_V4_EXPERIMENTAL.md)。
