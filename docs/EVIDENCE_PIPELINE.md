# SepiLens 证据生命周期与多通道检索管道

本文档定义 SepiLens 中变异解释证据链的生命周期、多通道检索协调策略与安全核验规范。

---

## 1. 统一证据模型 (`EvidenceItem`)

评估任务中产出的每一条证据均遵循统一的审计结构：

```json
{
  "id": "EVI-PVS1-001",
  "engine_id": "acmg_amp_2015",
  "guideline_version": "2015",
  "rule_code": "PVS1",
  "status": "proposed_met",
  "suggested_score": "Very Strong",
  "score_applied": null,
  "reason": "CFTR 基因无义突变，功能丧失为已知致病机制",
  "source_ids": ["SRC-PMID-25741868"],
  "source_validation": "verified",
  "review_required": true,
  "rule_conflicts": [],
  "missing_information": []
}
```

- `status` 枚举：`proposed_met`（提议适用）、`met`（已满足）、`not_met`（不满足）、`not_applicable`（不适用）、`insufficient_data`（证据不足）、`conflicting`（证据冲突）、`pending_review`（待人工审核）。
- `suggested_score`：智能体根据规则提议的候选强度或分值。
- `score_applied`：仅当确定性规则引擎通过约束校验后方可填入正式分值。

---

## 2. 来源实体与核验机制 (`Source`)

每个来源具有全局唯一稳定标识并记录核验状态：

- `pubmed`：验证 PMID 与 DOI 数字格式；
- `clinvar`：验证 VCV / RCV 或变异等位基因标识符；
- `clingen_dosage`：校验官方剂量敏感性评分（HI/TS）；
- `web_search`：原生网页检索结果，需经净化与交叉比对，默认标记为待确认。

### 核心医学不变量

1. **未核验来源禁止产出正式赋分**：若证据项引用的任何来源未通过核验，证据项状态强制降级为 `proposed_met`，不得标记为 `met`，并追加 `missing_information: ["source verification pending"]`。
2. **无文献绝不转化为良性分值**：检索结果为 0 时，系统严格标记为 `insufficient_data`，禁止以“未检索到致病报道”为由赋予良性（Benign）加分。
3. **病例重复报告去重**：识别多篇论文对同一患者家系或登记库的重复报道，避免在共分离或病例计数中重复累加。

---

## 3. 多通道检索的三档模式

| 模式 | 标识 | 请求预算 | 超时时间 | 适用场景 |
|---|---|---|---|---|
| **标准（纯离线）** | `off` | 0 次外网请求 | 0s | 纯本地 HPO 提取、本地知识库快照初筛 |
| **增强（受限定向）** | `targeted` | 最多 3 次网络请求 | 15s | 用户主动点击的单变异定向证据补全 |
| **深度（多轮追踪）** | `deep` | 最多 10 次网络请求 | 30s | 复杂重排、冲突变异文献追踪 |

检索协调器（`SearchCoordinator`）配置硬性熔断机制，当请求数或超时达到上限时自动截断并优雅降级。

---

## 4. 证据快照与 SHA-256 锁定 (`EvidenceSnapshot`)

每次评估生成的完整来源集合、证据项集合与溯源元数据（`ProvenanceMetadata`）会被序列化并通过 SHA-256 散列锁定，持久化于 SQLite 的 `evidence_snapshots` 表中，支持跨租户隔离审计与后续版本比对。
