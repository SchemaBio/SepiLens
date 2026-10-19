# 结构化注释与表型驱动评估开发计划

> 版本：v1.1（进度更新）  
> 编制日期：2026-10-19  
> 依据：[评估架构设计](ASSESSMENT_ARCHITECTURE.md)、[规则政策](RULESET_POLICY.md)、RC2 代码基线 `3262e17`  
> 目标：在保持医学门禁关闭的前提下，交付“结构化注释 + 表型驱动 + 本地确定性 + AI 覆盖”的工程闭环。

---

## 0. 当前进度总览

| 里程碑 | 内容 | 状态 |
|---|---|---|
| M0 | 基线整备：版本单一来源、文档一致性、OpenAPI 清单 | ✅ 已完成 |
| M1 | HPO 本体图：`is_a`、祖先/后代、信息含量、废弃映射 | ✅ 已完成 |
| M2a | 注释层：五类模型、校验、冲突标记、注释 API | ✅ 已完成 |
| M2b | 注释快照与索引（官方数据导入、SQLite 索引、快照 v2） | ⬜ 未开始 |
| M3 | 本地确定性派生：12 条条款、PVS1 分支、SVI 点数 | ✅ 已完成 |
| M4 | 表型评分与 PP4 | ⬜ 未开始 |
| M5 | AI 评估与条款覆盖 | ⬜ 未开始 |
| M6 | 审核、审计与发布 | ⬜ 未开始 |

已完成部分已通过 `go test ./...`（24 个包）、`go vet ./...`、OpenAPI 重建与二进制构建验证。

---

## 1. 目标与范围

把现有“调用方提交证据条目 → 规则引擎复核”的窄路径，扩展为两条完整评估路径：

| 路径 | 输入 | 处理 | 输出 |
|---|---|---|---|
| `local` | 位点 + 疾病/MOI + HPO + 五类注释 | 注释派生 → 表型评分 → 确定性组合 | `rule_engine_result` |
| `ai_assisted` | 同上 + provider profile | 本地基线 → LLM 复核/检索 → **覆盖条款** → 服务端重算 | 覆盖后的 `rule_engine_result` + `local_baseline` + 覆盖审计 |

四个新增能力：

1. **注释层**：ClinVar、gnomAD、AlphaMissense、Pangolin、GenCC。
2. **HPO 本体图**：补齐 `is_a`，支持祖先/后代与信息含量。
3. **表型层**：SepiLens Phenotype Score（参考 LIRICAL 方法，clean-room 实现）。
4. **AI 层**：工具调用、引用核验、条款覆盖、服务端确定性重算。

**不变的门禁**：

- 五个引擎默认 `disabled`。
- AI 模式不绕过批准门禁。
- CNV 与 SVC v4 本轮不动。
- 调用方提交的注释可评估，但不视为已核验证据。

---

## 2. 基线核对

| 检查 | 结果 |
|---|---|
| `go test ./...` | 24 个包全部通过 |
| `go vet ./...` | 通过 |
| `python scripts/openapi.py` | 22 操作 / 56 schema |
| `go build ./cmd/sepilens` | 成功 |
| HPO 资源 | 20,384 条，原始数据无 `is_a`，需依赖官方 OBO |
| CNV/SVC 回归 | 保持全绿 |

---

## 3. 架构落点（含实际实现状态）

```text
internal/
  hpo/
    model.go              ✅ 扩：Parents / AltIDs
    graph.go              ✅ 新：祖先闭包、共同祖先、深度、信息含量、废弃映射
    graph_test.go         ✅ 新：环检测、单根、扁平图、IC 单调性
  annotation/
    model.go              ✅ 新：五类记录、状态机、覆盖率与阈值助手
    resolver.go           ✅ 新：校验、归一化、冲突汇总、完整性
    protein.go            ✅ 新：三字母/单字母互认、同氨基酸/同残基
    model_test.go         ✅ 新
    protein_test.go       ✅ 新
    index.go              ⬜ M2b：大文件 SQLite 索引
  rules/acmg2015/
    derive.go             ✅ 新：注释 → 条款候选
    derive_test.go        ✅ 新
    points.go             ✅ 新：SVI 点数组合
    points_test.go        ✅ 新
    criteria.go           ✅ 扩：insufficient_data / conflicting 状态
  api/
    admission.go          ✅ 扩：mode / phenotype / annotations / options
    evaluate.go           ✅ 扩：注释派生接入、来源核验门禁
    annotation_handler.go ✅ 新：validate 与 sources 端点
    annotation_evaluation_test.go ✅ 新
    annotation_handler_test.go    ✅ 新
  phenotype/              ⬜ M4
  agent/                  ⬜ M5
```

---

## 4. 已完成里程碑

### M0 · 基线整备 ✅

| ID | 任务 | 结果 |
|---|---|---|
| M0.1 | 版本单一来源 | ✅ `main.version` → `config.Version` → `/v1/capabilities`；实测评测注入 `9.9.9-test` 后返回一致 |
| M0.2 | 文档一致性 | ✅ 修正 `CHANGELOG.md` 标题顺序，新增 Unreleased；修正 `VALIDATION.md` 过期基线描述 |
| M0.3 | 行尾策略 | ⚠️ 未归一化。仓库当前 CRLF 检出（100 个 Go 文件 CRLF、23 个 LF），需要单独决策 |
| M0.4 | OpenAPI 解析清单 | ✅ 改为按文件存在性解析，新增包不会中断生成 |

**验收**：全部通过。

### M1 · HPO 本体图 ✅

| ID | 任务 | 结果 |
|---|---|---|
| M1.1 | 扩展 `Term` | ✅ 新增 `Parents`、`AltIDs`，旧 JSON 仍可反序列化 |
| M1.2 | OBO 捕获 `is_a`、`alt_id` | ✅ 支持 `HP:0000001 ! Name` 限定符 |
| M1.3 | 图结构与校验 | ✅ 祖先/后代、共同祖先、深度、IC、单根、迭代 DFS 环检测 |
| M1.4 | 废弃/替代映射 | ✅ 先判断废弃再映射，避免返回已撤回术语 |
| M1.5 | 版本入快照 | ✅ `HPOVersions` 记录本体版本 |
| M1.6 | 扁平图保护 | ✅ 纯翻译数据返回 `Complete()==false`，不产生虚假祖先匹配 |

**验收**：全部通过；6 项图结构测试覆盖。

### M2a · 注释层（调用方提交）✅

| ID | 任务 | 结果 |
|---|---|---|
| M2a.1 | 注释模型与状态机 | ✅ `found/not_found/conflicting/unavailable/stale/caller_supplied` |
| M2a.2 | 五类记录与归一化 | ✅ ClinVar、gnomAD、AlphaMissense、Pangolin、GenCC |
| M2a.3 | 合并与校验 | ✅ assembly 一致、版本必填、频率/分数范围、冲突标记 |
| M2a.4 | 请求契约 | ✅ `mode`、`phenotype`、`annotations`、`combination_method`、`max_searches` |
| M2a.5 | 注释校验端点 | ✅ `POST /v1/annotations/validate`（原计划的 `/snv` 改为 `/validate`，语义更准确） |
| M2a.6 | 来源端点 | ✅ `GET /v1/annotations/sources` |

**关键设计**：

- `not_found` 与 `unavailable` 严格区分，两者都不推出良性。
- 调用方注释标记 `caller_supplied`，来源不进入 `verified`，结果强制人工复核。
- 空注释集合被拒绝，避免产生空派生结果。

### M3 · 本地确定性派生 ✅

| ID | 任务 | 结果 |
|---|---|---|
| M3.1 | 派生器骨架 | ✅ `Candidate`、`Derivation`、`Gaps` |
| M3.2 | 群体条款 | ✅ PM2（FAF95 + 覆盖 + Supporting）、BA1（>5%） |
| M3.3 | 计算条款 | ✅ PP3/BP4，多预测器去重为一条 |
| M3.4 | 同义/剪接 | ✅ BP7 需 Pangolin + 保守性；PVS1 剪接分支封顶 Strong |
| M3.5 | ClinVar 条款 | ✅ PS1/PM5，≥2 星门槛，同氨基酸/同残基 |
| M3.6 | 长度改变 | ✅ PM4 骨架，缺剪接评估时 `insufficient_data` |
| M3.7 | PVS1 决策树 | ✅ 需机制与转录本显式声明；GenCC Refuted 阻断 |
| M3.8 | SVI 点数组合 | ✅ 边界、冲突、BA1 支配、重复代码去重 |
| M3.9 | 组合器强度 | ✅ 使用条款实际强度，PVS1 不再固定 VeryStrong |
| M3.10 | 接入 `api.evaluate` | ✅ 有注释走派生，无注释保持旧路径 |

**已实现的可派生条款（12 条）**：PVS1、PS1、PM2、PM4、PM5、PP3、PP5、BA1、BP1、BP4、BP6、BP7。

**已定义但未实现**：PP4（依赖 M4 的表型评分）、BS1（需疾病频率模型）、PS2/PS3/PS4/PM1/PM3/PM6/PP1/PP2/BS2/BS3/BS4/BP2/BP3/BP5（需文献、家系或人工材料）。

**边界语义**：

- `not_met`：已评估但未满足（例如 AF 恰好 5%）。
- `not_applicable`：变异类型不适用。
- `insufficient_data`：前提缺失。
- `conflicting`：多源矛盾。

---

## 5. 待完成里程碑

### M2b · 注释快照与索引（1.5 周）

| ID | 任务 | 文件 | 验收 |
|---|---|---|---|
| M2b.1 | `knowledge` 新增资产：`hpoa`、`gencc_tsv`、`alphamissense_tsv`、`gnomad_tsv`、`clinvar_snapshot` | `internal/knowledge/bundle.go` | manifest 校验与散列 |
| M2b.2 | 官方解析器与许可字段 | 同上 | 合成夹具通过 |
| M2b.3 | 大文件索引：SQLite 按基因/位点查询 | `internal/annotation/index.go` | 查询 < 50ms |
| M2b.4 | 过期与不可用策略 | `resolver.go` | 不把缺失当良性 |
| M2b.5 | 注释快照并入证据快照 | `internal/evidence/model.go` | `hash_version` v2 兼容 |

**退出条件**：同一请求在 `caller_supplied` 与 `snapshot` 下得到相同归一化结果。

### M4 · 表型评分与 PP4（2.5 周）

| ID | 任务 | 文件 | 验收 |
|---|---|---|---|
| M4.0 | 引入官方 `hp.json` / `hp.obo` 并与中文翻译层合并 | `internal/hpo`、`internal/knowledge` | 图 `Complete()` 为 true |
| M4.1 | `phenotype.hpoa` 解析 | `internal/phenotype/hpoa.go` | 频率映射可配置 |
| M4.2 | 背景频率与信息含量预计算 | `background.go` | 加载一次并缓存 |
| M4.3 | 六种匹配关系的单术语 LR | `lr.go` | 每类关系单测 |
| M4.4 | 聚合、排名、margin、特异性 | `aggregate.go` | 单调性测试 |
| M4.5 | PP4 十项门禁 | `pp4.go` | 任一不满足不得 `met` |
| M4.6 | 参数版本 `sepi-pheno-0.1` | `spec.go` | 改参新建版本 |
| M4.7 | `POST /v1/phenotype/match` | `phenotype_handler.go` | 不产生分类 |
| M4.8 | 留一法校准脚本 | `scripts/calibrate_phenotype.py` | 输出排名命中率报告 |

**退出条件**：表型评分可复现；PP4 在表型不足/冲突/未确认时不误判。

### M5 · AI 评估与覆盖（3 周）

| ID | 任务 | 文件 | 验收 |
|---|---|---|---|
| M5.1 | Provider 扩展：`Tools`、`ToolCall`、多轮 | `internal/llm/provider.go` | 接口测试 |
| M5.2 | Responses 工具调用 + Chat `tool_calls` | `internal/llm/responses/client.go` | 本地 mock 服务器 |
| M5.3 | 工具注册与预算 | `internal/agent/tools.go` | 超预算结构化失败 |
| M5.4 | 输入组装 | `input.go` | 无患者原文/标识符 |
| M5.5 | 输出 Schema 与严格解析 | `schema.go`、`parse.go` | 未知字段拒绝 |
| M5.6 | 引用核验 | `verify.go` | 伪造/错配引用被拒 |
| M5.7 | 覆盖执行：六种 `change`、`ai_omitted`、`unsupported_override` | `override.go` | 与服务端裁决一致 |
| M5.8 | 服务端重算与 `major_override` | `override.go`、`api/runtime.go` | 模型自报分类被忽略 |
| M5.9 | 提示词嵌入与版本 | `prompts/acmg_ai_system.md` | `prompt_version` 入结果 |
| M5.10 | 请求/响应契约 | `admission.go`、`jobs_handler.go` | `local_baseline`/`reconciliation` |
| M5.11 | 结算门禁 | `runtime.go` | 未复核覆盖不可结算 |

**退出条件**：mock LLM 全流程通过；AI 无法修改注释或表型分数；分类由服务端重算。

**覆盖执行顺序**（严格按此顺序）：

```text
parse → schema → 代码去重 → 引用存在性 → 对象匹配 → facts 完整性
→ 强度上限 → change 合法性（非 confirmed 需 reason）
→ 覆盖合并（保留 ai_omitted）→ 组合器重算 → major_override → 结算标记
```

### M6 · 审核、审计与发布（1.5 周）

| ID | 任务 | 文件 | 验收 |
|---|---|---|---|
| M6.1 | `revise_evidence` 接受 AI 条款并重算 | `internal/api/revision.go` | revision 递增、旧版不变 |
| M6.2 | 审计导出：注释、表型、agent、工具、覆盖 | `audit.go` | 加密、租户隔离 |
| M6.3 | 删除与保留覆盖新数据 | `tenant.go`、`workflow/queue.go` | 租户删除彻底 |
| M6.4 | 负向安全测试扩展 | `tests/negative_security` | 注入/伪造/越权 |
| M6.5 | OpenAPI 与文档同步 | `docs/openapi.yaml`、`API.md` | CI 无漂移 |
| M6.6 | smoke 脚本扩展 | `scripts/smoke.py` | 双平台通过 |

---

## 6. 接口契约

### 6.1 请求字段

```json
{
  "engine_id": "acmg_amp_2015",
  "mode": "local | ai_assisted",
  "variant": { "assembly": "GRCh38", "chrom": "7", "pos": 100, "ref": "A", "alt": "G", "gene": "CFTR", "transcript": "NM_000492.4", "consequence": "missense_variant" },
  "clinical_context": { "disease_id": "MONDO:0009061", "inheritance": "AR", "variant_origin": "germline_confirmed", "sample_type": "blood" },
  "phenotype": {
    "hpo_terms": [{ "id": "HP:0001945", "polarity": "present", "onset": "childhood" }],
    "source": "clinician_verified | extracted_pending_review | caller_supplied"
  },
  "annotations": {
    "source": "caller_supplied | sepilens_snapshot | sepilens_api",
    "clinvar": {}, "gnomad": {}, "alphamissense": {}, "pangolin": {}, "gencc": {}
  },
  "options": {
    "combination_method": "acmg_2015_table | svi_points",
    "allow_pp5_bp6": false,
    "max_searches": 12
  }
}
```

兼容：旧 `clinical_context.hpo_terms: string[]` 按 `present` + `caller_supplied` 解析，不能支撑 PP4。

### 6.2 已实现响应结构

`local` 模式下 `rule_engine_result` 新增：

```json
{
  "classification": "Likely pathogenic",
  "combination_method": "svi_points",
  "derivation": {
    "candidates": [
      {
        "code": "PM2",
        "status": "met",
        "strength": "Supporting",
        "reason": "gnomAD FAF95 is rare with adequate coverage; SVI default strength is Supporting",
        "source_ids": ["gnomad:v4.1"],
        "missing_information": []
      }
    ],
    "gaps": []
  },
  "points": { "classification": "Likely pathogenic", "pathogenic_points": 7, "benign_points": 0, "spec_version": "svi-points-0.1" }
}
```

### 6.3 注释辅助端点

| 方法 | 路径 | 作用 |
|---|---|---|
| `POST` | `/v1/annotations/validate` | 归一化与校验，不分类 |
| `GET` | `/v1/annotations/sources` | 列出来源与结算状态 |

### 6.4 计划新增错误码

`ANNOTATION_UNAVAILABLE`、`ANNOTATION_STALE`、`PHENOTYPE_UNVERIFIED`、`PHENOTYPE_INCONSISTENT`、`GENE_DISEASE_REFUTED`、`AI_TOOL_BUDGET_EXCEEDED`、`AI_OUTPUT_INVALID`。

---

## 7. 数据与快照兼容

**风险点**：`EvidenceSnapshot.hash` 覆盖 `sources/items/provenance`。新增字段若进入规范载荷，会使旧快照校验失败。

**方案**：

1. 新增字段一律 `omitempty`，空值不参与 JSON。
2. 新字段：`annotations`、`phenotype`、`agent`、`overrides`。
3. 读取按 `hash_version`（缺省 v1）分支校验；新快照写 v2。
4. 旧结果不原位改写；`legacy_unverified` 逻辑保持。
5. 默认不新增表；数据随加密 JSON 存储。

---

## 8. 测试与验收门禁

### 8.1 每阶段必须通过

```powershell
go test ./... -count=1
go vet ./...
python scripts/openapi.py; git diff --exit-code docs/openapi.yaml
go build -trimpath -o tmp\sepilens.exe ./cmd/sepilens
```

Linux 额外：`go test -race ./...`。

### 8.2 测试矩阵

| 层 | 必测 | 状态 |
|---|---|---|
| 注释 | 五类源、assembly、版本、`not_found` vs `unavailable`、冲突、空集合 | ✅ 已覆盖 |
| 派生 | PM2/BA1 边界、PP3/BP4 去重、PS1/PM5 星级、PVS1 声明 | ✅ 已覆盖 |
| 点数 | 分类边界、冲突、BA1 支配、重复代码 | ✅ 已覆盖 |
| 图 | 环、单根、扁平图、IC 单调性、废弃映射 | ✅ 已覆盖 |
| 表型 | 六种关系、单调性、排除术语、平局 | ⬜ M4 |
| PP4 | 十项门禁逐项失败用例 | ⬜ M4 |
| AI | 覆盖六种 `change`、无依据拒绝、遗漏、伪造引用 | ⬜ M5 |
| 安全 | 跨租户、密钥、原文不落日志、注入 | ✅ 现有；⬜ 扩展 |

### 8.3 发布门禁（不变）

- 医学引擎仍 `disabled`，除非有签名批准包。
- AI 模式不绕过批准。
- 未人工复核的 AI 覆盖结果不可结算。
- 数据源许可未确认时默认 `disabled`。

---

## 9. 风险与待决策

### 9.1 风险

| 风险 | 缓解 | 回滚 |
|---|---|---|
| HPO 图加载拖慢启动 | 预计算缓存、基准门禁 | 保留旧索引路径 |
| AlphaMissense 数据过大 | SQLite 索引/按需加载 | 关闭该源，条款降级 |
| 表型 LR 阈值不成熟 | 参数版本化、默认保守 | 回退 `sepi-pheno-0.x` |
| AI 覆盖误改本地结论 | 硬前提、引用核验、`major_override` | 关闭 `ai_assisted` |
| 工具调用成本失控 | 预算 + 硬上限 | 降低 `max_searches` |
| 快照散列不兼容 | `omitempty` + `hash_version` | 读旧格式分支 |

**总开关**：`SEPILENS_AI_MODE=disabled|enabled`，默认 `disabled`。

### 9.2 待确认决策

| # | 决策 | 建议默认 | 状态 |
|---|---|---|---|
| 1 | 行尾策略：是否归一化为 LF 并加 CI 检查 | 归一化 | ⬜ 待确认 |
| 2 | 候选疾病集合 | 只接受调用方 `disease_id` | ⬜ M4 前确认 |
| 3 | 组合方法默认值 | 2015 表默认，点数并列返回 | ✅ 已实现双方法 |
| 4 | PP5/BP6 | 默认禁用 | ✅ 已按默认禁用实现 |
| 5 | AI 覆盖结果位置 | 作为 `rule_engine_result` 生效 | ⬜ M5 前确认 |
| 6 | 无依据覆盖 | 保留本地值 + `unsupported_override` | ⬜ M5 前确认 |
| 7 | AI 遗漏本地条款 | 保留本地值 + `ai_omitted` | ⬜ M5 前确认 |
| 8 | 注释来源 | 生产快照、开发 API | ⬜ M2b 前确认 |
| 9 | `absent` 表型参与 PP4 | 参与，仅限频率明确时 | ⬜ M4 前确认 |
| 10 | 首轮范围 | 常染色体 SNV/InDel | ✅ 当前实现范围 |

---

## 10. 医学与合规边界

1. 本轮不产生任何医学批准声明。
2. 所有阈值标注“工程默认，待独立校准”。
3. 不复制 LIRICAL 代码、数据、常量；方法引用 PMID 32755546。
4. 数据源许可未确认时默认关闭，不随发行包分发完整官方数据。
5. AI 提示词版本化，模型与供应商固定，变更需回归案例。
6. 调用方提交的注释不视为已核验证据，不可结算。

---

## 11. 工期估算

| 阶段 | 工期 | 累计 |
|---|---|---|
| M0 + M1 + M2a + M3 | 已完成 | — |
| M2b | 1.5 周 | 1.5 周 |
| M4 | 2.5 周 | 4 周 |
| M5 | 3 周 | 7 周 |
| M6 | 1.5 周 | 8.5 周 |

医学验证（独立黄金案例、专家批准、数据许可）独立于工程排期。

---

## 12. 任务依赖

```text
M0 ✅ ─ M1 ✅ ─ M2a ✅ ─ M3 ✅
                    │
                    ├─ M2b ─┐
                    │       ├─ M4 ─ M5 ─ M6
                    └───────┘
```

M4 依赖 M1 的图结构与 M2b 的 HPOA 数据；M5 依赖 M3 的派生结果与 M4 的 PP4。
