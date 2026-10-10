# SepiLens（薛冷）完整开发计划

> **文档版本**：v1.0（2026-10-10）  
> **项目代号**：SepiLens  
> **项目定位**：独立、可部署、可集成的医学 AI 增强解读 API 服务  
> **主要语言**：Go  
> **LLM 首选接口**：OpenAI Responses API  
> **交付目标**：2026 年完成可验证的核心服务；2027 年接入生物信息分析云平台并具备正式发布条件。  
> **边界声明**：本计划是工程实施方案，不代替 ACMG/AMP、ClinGen 或其他机构发布的临床技术规范；涉及临床使用的规则、算法和结果须经适用范围确认、性能验证和授权人员审核。

---

## 0. 执行摘要

SepiLens 将已有的 [DeepHPO](https://github.com/pzweuj/DeepHPO) 能力抽离并扩展为 Go 编写的独立医学 AI 服务。**第一代产品只聚焦 HPO 提取与胚系变异解释，但按五条独立能力线组织开发：**

| 产品能力 | 引擎标识（固定、可审计） | 输入对象 | 标准 / 方法 | 交付状态 |
|---|---|---|---|---|
| HPO 术语提取 | `hpo_extract` | 临床文本 | 本地 HPO 本体 + LLM | 首版完整上线 |
| SNV/InDel 致病性评估 | `acmg_amp_2015` | 胚系小变异及疾病关系 | ACMG/AMP 2015 + 明确选定的 ClinGen SVI/VCEP 细则 | 首版完整上线 |
| SNV/InDel 新版致病性评估 | `svc_v4` | 胚系小变异及疾病关系 | SVC v4 正式指南 + CSpec Method Model | 先建接口及模型；**正式规则发布并验证后启用** |
| CNV 缺失致病性评估 | `cnv_loss_2020` | Constitutional CNV loss / deletion | ACMG/ClinGen CNV Loss Rubric（2020） | 首版完整上线 |
| CNV 增益致病性评估 | `cnv_gain_2020` | Constitutional CNV gain / duplication 等 | ACMG/ClinGen CNV Gain Rubric（2020） | 首版完整上线 |

**关键决定：**

1. **HPO + 三套已发表的评估规则**（ACMG/AMP 2015、CNV Loss 2020、CNV Gain 2020）优先达到生产可验证状态。
2. **SVC v4 与 ACMG/AMP 2015 相互独立**：不通过替换参数或覆盖旧数据来“升级”；在正式规范确定之前不得宣称完成临床判读能力。
3. **CNV Loss 与 Gain 相互独立**：共用变异区间标准化和证据检索，不共用规则表、适用性决策或加分逻辑。
4. **Agent 提议、规则引擎裁决、人工复核**：LLM 负责搜集、梳理和说明证据；确定性程序检查条件、约束、分数与分类；最终采纳仍属于人工审核。
5. **用户可自定义端点、API Key、模型名（BYOK）**；Responses API 优先，兼容协议作为适配层能力，不能假定兼容 Responses 就自动兼容 `web_search`。
6. **SepiLens 与云平台计费解耦**；云平台可对每次有效增强判读收取 1 积分，但不在核心引擎里写死积分、人民币或套餐。

### 0.1 首版非目标

- 不对每个 WES 的 50,000+ SNV/InDel 位点运行 Agent；云平台先按既定规则初筛、人工复核，仅对人工选出的少量位点点击调用。
- 不开发完整 NGS 上游分析流程，不读取 FASTQ/BAM 并从头检出 CNV/SNV。
- 不在首版实现肿瘤体细胞致癌性分级、治疗证据等级或疗效推荐；如未来新增，应建立独立标准和验证体系。
- 不将尚未证实的胚系来源自动视为胚系变异，不把肿瘤拷贝数改变自动套入 ClinGen constitutional CNV 评分。
- 不承诺 LLM 判读可替代人工签发的临床报告。
- 不将付费、SaaS 用户体系、NGS 样本管理写入引擎核心。

---

## 1. 标准体系与医学适用边界

### 1.1 五个功能模块共享基础设施，但独立维护规则

```text
SepiLens API
  ├─ HPO Extraction                    → HPO ontology + LLM
  ├─ Sequence Variant Classification
  │    ├─ ACMG/AMP 2015                → RuleSet A
  │    └─ SVC v4                       → RuleSet B（未批准前禁用）
  └─ Constitutional CNV Classification
       ├─ ClinGen / ACMG CNV LOSS 2020 → RuleSet C
       └─ ClinGen / ACMG CNV GAIN 2020 → RuleSet D

共用：Provider · Retrieval · Normalization · Evidence Store · Review · Audit
隔离：适用条件 · Evidence Code · RuleSet · Scoring · Classifier · 验证基准
```

### 1.2 小变异：ACMG/AMP 2015

- 基线：Richards 等，2015 年 ACMG/AMP 序列变异解释共识。
- 支持 PVS1、PS、PM、PP、BA、BS、BP 等证据代码、各自适用条件和允许的强度修订；**不要把所有证据硬编码成简单布尔值**。
- 标准基线分类为 Pathogenic、Likely pathogenic、VUS、Likely benign、Benign。
- 另建 `specification_registry`：按 **gene + disease + inheritance/MOI** 选择已发布的 ClinGen VCEP 细化规则；无法确认时使用明示的 baseline，不擅自套用其他基因的专用规则。
- 规则组合表与证据冲突、独立性、强度调整均由确定性引擎负责。
- ClinVar 既有解释只作带版本和审核状态的证据来源之一，不能直接覆盖本次分类。
- 变异解读的命题应体现 **variant–disease–MOI**，不是单独一个 HGVS 字符串的永久固定标签。

### 1.3 小变异：SVC v4

- 采用独立 `svc_v4` 引擎，按最终正式发布的指南、CSpec Methods、Evidence Codes、计分及分类规则实现。
- 当前公开的 [ClinGen SVC v4 数据模型仓库](https://github.com/clingen-data-model/svcv4-model) 是 **Classification Model**；规则逻辑属于 **CSpec Method Model**，二者不可混淆。
- 对发布进度设“规则就绪门禁”：**正式标准已发布 → 规则授权/版本锁定 → 代码实现 → 标准案例验证 → 人工审批启用**。
- 若发布晚于项目进度：可以交付 `svc_v4` 的 schema、API 框架和 `unavailable` 状态，但不得自动切换到 2015 或臆造 v4 积分。
- 数据模型允许后续映射新版分级体系，最终分类名称、阈值、证据类别等以正式版本为准。

### 1.4 CNV：2020 ACMG/ClinGen constitutional CNV 技术标准

**Loss 与 Gain 各自有独立的证据评分表和条款。** 以 2020 年联合技术标准正文、补充材料和 ClinGen 计算器为准，不根据解读人员印象重新定义分值。

- `cnv_loss_2020` 对应 [ClinGen CNV Loss Calculator](https://cnvcalc.clinicalgenome.org/cnvcalc/cnv-loss)。
- `cnv_gain_2020` 对应 [ClinGen CNV Gain Calculator](https://cnvcalc.clinicalgenome.org/cnvcalc/cnv-gain)。
- 五组主章节：①基因组内容；②HI/TS、已知良性区域及部分基因内事件；③包含的蛋白编码基因数量；④文献病例、表型匹配和病例对照证据；⑤遗传来源及家系证据。**具体分支、排他条件、可选分值和最大得分须分别按两张正式 rubric 配置。**
- Loss 以 **haploinsufficiency（HI）** 为重点，Gain 以 **triplosensitivity（TS）** 为重点，但不意味着分别仅需读取 HI 或 TS；必须遵照各自 rubric 评估其他组合和重叠情形。
- `intragenic` loss/gain、部分重叠、断点位置、内含子区域、编码序列方向、拷贝数、重复/三倍化、重排方向不明等需专门分支，不能仅通过“删除 / 复制”字符串做判决。
- 若无法确定 tandem duplication 的方向、位置或基因功能影响，需要保留不确定性，不得把额外拷贝数自动解释为确定的功能增益。
- 不完整的 CNV 边界、嵌合状态、不稳定 CN、低分辨率片段等进入人工复核或 `insufficient_data`。
- 支持 ClinGen **Dosage Sensitivity** 已公布的基因/区域 curations、版本与坐标，并允许获取其相关引用。
- 同一份 CNV 证据不得因覆盖多个相邻基因就被无约束叠加，必须符合各章节的互斥和上限要求。

#### CNV 评分和分类（两个引擎适用相同的五级分类区间，但规则表不同）

| 总分 | 候选分类 |
|---:|---|
| ≥ +0.99 | Pathogenic |
| +0.90 至 +0.98 | Likely pathogenic |
| −0.89 至 +0.89 | VUS |
| −0.98 至 −0.90 | Likely benign |
| ≤ −0.99 | Benign |

> 工程约束：分数精度、区间端点、逐项分值范围和分项封顶按 rubric 实现。建议在 Go 内用固定精度十进制或整数化的 rubric 分数，不用二进制浮点数进行临界值比较；对非标准小数处理需有明确规则，避免产生区间间隙。

#### 极其重要：体细胞肿瘤 CNV 不属于本模块的自动适用范围

这是 **constitutional（胚系）CNV** 标准。对肿瘤组织中获得的 `MET amplification`、`MYC amplification`、`PTEN deletion` 等，必须区分拷贝数检测、变异起源和临床意义分级。**不能直接用本标准给肿瘤体细胞扩增/缺失下 ACMG 胚系致病性结论。**

应要求调用方传入 `variant_origin`（`germline_confirmed` / `somatic_confirmed` / `unknown`）和 `sample_type`。来源不明时，可返回证据整理 / 研究性预评估，但禁止自动形成已确认胚系结论。

### 1.5 HPO 与 CNV 联动

- HPO 提取结果可作为 SNV/Indel 与 CNV 评估的表型输入。
- 保留 `present`、`absent`、`uncertain`、`family_history`、`inferred` 的来源语义。
- **只有病历中确认存在的患者表型** 才能在适用的疾病和具体规则下形成对应患者表型证据；LLM 从诊断名称推测出的可能症状只能作为提示，不能自动参与计分。
- CNV Section 4 的表型匹配、病例数量和来源独立性必须由特定规则和核验材料支持。

---

## 2. 用户流程与产品交互

### 2.1 HPO

1. 用户粘贴脱敏的临床文本或通过 API 传入。
2. LLM 提取文本中明确记录的症状、症状极性、起病时间、家族史、来源位置。
3. Go 本地 HPO 索引召回候选术语；LLM 仅在候选集内选最合适术语。
4. 后端验证 HPO ID、已废弃术语替代关系、同义词和版本。
5. 返回 HPO 术语表、原文跨度、证据类型、可信度及待审核标记。

### 2.2 SNV/InDel 增强判读

1. NGS 云平台分析原始 50,000+ 位点 → 固定规则初筛 → 专业人员复核。
2. 用户主动选中一个变异并点击“AI 增强判读”。
3. 用户选择 `acmg_amp_2015` 或 `svc_v4`（仅在该引擎已批准上线时可选）。
4. 传入变异标准化信息、转录本、疾病、MOI、临床表型和已有 VEP/gnomAD/ClinVar/dbNSFP 等结构化证据。
5. Agent 对可评估证据逐项提出候选判断，并形成缺口清单。
6. 按 `retrieval_mode` 选择不联网、按需搜索或深度搜索。
7. 检索新证据 → 核实 PMID/DOI、ClinVar、ClinGen、原始变异匹配 → 去重 → 更新待审证据。
8. 确定性规则引擎核验适用条件、冲突、强度、计分或组合规则，形成候选分类。
9. 保存来源、版本、人工修改、最终审核状态，不自动覆写解读人员的结论。

### 2.3 CNV Loss / Gain 增强判读

1. 选中 **一条经过质量控制和人工确认的 CNV 区间**，明确 GRCh37/GRCh38、坐标体系和事件方向。
2. 根据 `type=loss` 或 `type=gain` 显式选择独立引擎；拒绝缺少方向的输入。
3. 本地注释：基因与外显子相交、编码基因数量、HI/TS、已知致病/良性区域、基因内结构。
4. Agent 审查已提供的家系、临床表型、病例文献、病例对照研究；按需补充搜索。
5. 以 **Loss 或 Gain 独立评分表**输出每一条条款的适用性、分值建议、计算结果和支持资料。
6. 确定性 CNV 引擎进行互斥检查、子章节上限处理、分值聚合、总分到五级分类映射。
7. 对照 ClinGen 计算器可验证后形成待审核建议。

### 2.4 可选联网的三档模式

| 模式 | 标识 | 策略 | 默认建议 |
|---|---|---|---|
| 标准 | `off` | 仅使用输入与已同步的本地数据快照 | HPO、批量预处理 |
| 增强 | `targeted` | 先发现证据缺口，再进行受限的定向检索 | 用户点击的单变异评估 |
| 深度 | `deep` | 多轮检索、全文核查、冲突证据追踪；预算更严格 | 复杂变异按需使用 |

无论联网模式如何，**判读标准不变，变动的只有可供审查的证据集合**。无法联网或检索无结果不等于良性证据。

---

## 3. 技术架构：Go 模块化单体

```text
Next.js NGS 云平台              第三方软件 / CLI
           │                           │
           └──────── REST / SSE ───────┘
                        │
                 SepiLens API (Go)
                        │
        ┌────────── HTTP / Auth ───────────┐
        │     Validation / Idempotency     │
        └──────────────────────────────────┘
                        │
             Application Orchestrator
                        │
           ┌────────────┼───────────────┐
           ▼            ▼               ▼
          HPO       Sequence           CNV
        Engine      Engine             Engine
                   ├ ACMG 2015        ├ LOSS 2020
                   └ SVC v4           └ GAIN 2020
           └────────────┬───────────────┘
                        ▼
          Evidence / Rules / Provenance
                        │
      ┌─────────────────┼───────────────────┐
      ▼                 ▼                   ▼
  LLM Provider       Retrieval            Storage
  Responses First    Native Web          SQLite/Postgres
  Chat Fallback      PubMed/ClinVar       Jobs/Snapshots
                     ClinGen/Dosage
```

### 3.1 Go 技术选型

| 部分 | 推荐实现 | 说明 |
|---|---|---|
| 语言 | Go 1.25+（结合 SDK 实际最低版本锁定） | 统一开发、构建和部署 |
| 路由 | 标准库 `net/http` + `chi` | 轻量、OpenAPI 好集成 |
| HTTP/LLM | 官方 `openai-go/v3` 为主 | Responses API、流式响应、自定义 Base URL；厂商行为通过适配器隔离 |
| 结构化数据 | Go struct + JSON Schema + 严格校验 | JSON Schema 不等于医学规则正确性 |
| 异步执行 | Go Worker + 数据库持久化任务 | 进程重启可恢复；不依赖内存队列保证可靠性 |
| 数据库 | SQLite（开发 / 单机），PostgreSQL（生产扩展） | repository interface；不能把 DB 逻辑写进医学引擎 |
| SSE | `text/event-stream` | HPO 阶段进度和增强判读进度 |
| 日志与追踪 | `slog` + OpenTelemetry 可选 | 默认不记录患者原文、密钥和完整模型提示词 |
| Schema / 文档 | OpenAPI 3.1 | 随代码生成或 CI 校验漂移 |
| 打包 | Multi-stage Docker + 独立二进制 | Linux amd64/arm64；可选单机私有部署 |

**避免提前微服务化**：首版只启动 `sepilens serve` 一个主进程，任务 worker 可以作为同一进程内的独立模块；以后若负载需求成立再拆分。

### 3.2 LLM Provider 抽象

首选 `responses`，但内部必须定义独立的 Provider 接口，例如：

```go
type LLMProvider interface {
    Capabilities(ctx context.Context) (ModelCapabilities, error)
    Generate(ctx context.Context, req GenerateRequest) (GenerateResult, error)
    Stream(ctx context.Context, req GenerateRequest) (<-chan ModelEvent, error)
}
```

**Provider profiles** 按“用户 + 端点 + 模型 + 协议”配置；一个厂商可有不同端点 profile。

- `protocol`: `openai_responses`（默认）、`openai_chat`、`anthropic_messages`（兼容 DeepHPO 现有能力）。
- `capabilities`: `structured_output`、`streaming`、`function_calling`、`native_web_search`、`search_citations`、`usage_reporting`。
- `search_backend`: `native` / `sepilens_retrieval` / `disabled`。
- 工程上提供连接测试、模型能力探测和版本化能力表；**探测失败时禁止假定搜索支持**。
- 不向客户端展示底层各家独有字段；适配为 SepiLens 内部事件和证据来源结构。
- 避免把服务商内部的 `response_id` / state 当成跨供应商持久协议；默认使用无状态请求或应用层保存上下文。

### 3.3 检索系统

分为两条通道：

1. **原生搜索**：支持时使用 Responses `web_search`，保存工具调用、来源 URLs、检索时间与引用注释；应显式处理工具未被模型调用的情况。
2. **专业证据接口**：ClinGen/ClinVar/NCBI PubMed、ClinGen Dosage Sensitivity、本地规范化的人群数据库及医学术语库。Web Search 负责发现，专业来源负责精确核验。

必要的检索约束：

- 以标准化 HGVS / GRCh 坐标 / ClinVar allele ID / 基因与疾病为查询键；区分单个位点证据和仅基因相关证据。
- 文献检索结果未核对到确切变异、准确的基因/疾病和实验设计之前，不能进入可计分证据。
- 支持文献摘要 vs 可获取全文的证据等级标记。
- 识别同一病例在多篇论文中的重复报告，限制重复计数。
- 可配置请求数、检索深度、总 Token 预算、超时和总任务时长。
- 对外部网页、论文内容采用不可信输入处理，防止提示词注入指令污染医学判断。
- 保留 provider 原生 citations 与原始工具响应，但向客户端暴露经过校验的标准来源清单。

### 3.4 医学规则资源与 Agent 提示词彻底分离

| 层级 | 内容 | 是否允许 LLM 直接修改 |
|---|---|---|
| `agents/*/agent.md` | Agent 目标、证据审查步骤、限制、输出协议 | 不允许运行时修改 |
| `rules/<id>/<version>/` | 条款定义、适用条件、分值、互斥关系、分类映射 | **不允许** |
| `schemas/` | 输入、证据、候选结论、审核与溯源协议 | 不允许 |
| `knowledge/` | HPO、ClinGen 剂量敏感性、gene intervals 等本地数据快照 | 仅经版本化更新 |
| `evidence/` | 输入证据、检索证据、来源与审核意见 | LLM 可提议，必须校验 |

根目录 `AGENTS.md` 用于 Codex / 开发Agent的工程规约；运行时医学Agent使用独立的提示词文件。

---

## 4. 规范化数据模型

### 4.1 公共元数据

每次请求/结果至少保留：

- `request_id`、`job_id`、`tenant_id`、`idempotency_key`；
- `engine_id`、`guideline_version`、`ruleset_hash`、`specialization_id`；
- `model_provider`、`model_id`、`provider_profile_id`、`prompt_version`；
- `retrieval_mode`、`retrieval_timestamp`、`dataset_versions`；
- `input_snapshot_hash`、`evidence_snapshot_hash`、`result_version`；
- `proposed_classification`、`reviewed_classification`、`reviewer_status`；
- `usage`: input/output tokens、web 搜索次数、任务耗时及成本估算；
- `created_at`、`reviewed_at`、`source_provenance` 和所有人工修订记录。

### 4.2 SNV/InDel 输入

要求 genome build、规范化染色体坐标（含明确 1-based/0-based）、REF/ALT、转录本与版本（如存在）、HGVS、基因、疾病标识（优先 MONDO）、MOI、起源、样本类型、技术质量摘要、相关表型。支持附带现有 VEP、gnomAD、ClinVar、dbNSFP、功能实验与家系数据。

- 变异标准化需考虑 GRCh37/GRCh38、indel 左对齐、ref 验证、HGVS 与 VCF 的对应、等价表示和转录本差异。
- 自动判读不完整：若缺少必要疾病 / MOI 语境，返回 `needs_context`，不得编造疾病条件。

### 4.3 CNV 输入

```json
{
  "engine_id": "cnv_loss_2020",
  "cnv": {
    "assembly": "GRCh38",
    "chrom": "7",
    "start": 10000000,
    "end": 12000000,
    "coordinate_system": "1-based-inclusive",
    "type": "loss",
    "copy_number": 1,
    "breakpoint_precision": "estimated",
    "mosaicism": "unknown"
  },
  "clinical_context": {
    "variant_origin": "germline_confirmed",
    "sample_type": "blood",
    "disease_id": null,
    "hpo_terms": []
  },
  "evidence": {
    "inheritance": null,
    "segregation": [],
    "case_reports": [],
    "case_control": []
  },
  "options": {
    "retrieval_mode": "targeted",
    "provider_profile_id": "default"
  }
}
```

> 示例仅用于接口结构：区间/拷贝数为虚拟值，并非真实临床病例。生产版要求类型、坐标、性染色体剂量、复杂重排等联合一致性校验。Gain 改用 `cnv_gain_2020` 与 `type=gain`，不得在两者不一致时自动猜测。

### 4.4 统一证据输出

```json
{
  "engine_id": "cnv_loss_2020",
  "guideline_version": "2020",
  "rule_code": "2A",
  "status": "proposed_met",
  "suggested_score": "1.00",
  "score_applied": null,
  "reason": "待审核的证据解释",
  "source_ids": ["SOURCE-001"],
  "source_validation": "verified",
  "review_required": true,
  "rule_conflicts": [],
  "missing_information": []
}
```

- `status` 应覆盖：`met`、`not_met`、`not_applicable`、`insufficient_data`、`conflicting`、`pending_review`；Agent 还可使用 `proposed_*` 等候选状态。
- `suggested_score` 为候选分数，`score_applied` 必须由规则引擎验证后填写；不允许把 Agent 返回的数值直接作为最终分类依据。
- 来源须有稳定标识、URL/PMID、检索时间、来源版本、检索方式、全文/摘要标记、适用变异关系。

---

## 5. API 契约（v1）

### 5.1 核心接口

| 方法 | 路径 | 作用 |
|---|---|---|
| `GET` | `/healthz`、`/readyz` | 存活与依赖就绪探针 |
| `GET` | `/v1/capabilities` | 功能、规则状态、检索方式与能力矩阵 |
| `POST` | `/v1/providers/test` | 测试自定义端点、模型与搜索能力 |
| `POST` | `/v1/hpo/extract` | HPO 提取（可同步或 SSE） |
| `POST` | `/v1/variants/snv/evaluations` | SNV/InDel 增强判读任务 |
| `POST` | `/v1/variants/cnv/evaluations` | CNV Loss/Gain 判读任务 |
| `GET` | `/v1/jobs/{job_id}` | 状态、阶段、进度、资源使用量 |
| `GET` | `/v1/jobs/{job_id}/events` | SSE 进度事件，支持重连 |
| `GET` | `/v1/jobs/{job_id}/result` | 结构化证据与待审核分类结果 |
| `POST` | `/v1/jobs/{job_id}/cancel` | 取消与资源释放 |
| `POST` | `/v1/evaluations/{id}/reviews` | 人工证据审核、采纳/驳回及理由 |
| `GET` | `/v1/rulesets` | 可用规范、版本、启用状态与散列值 |

### 5.2 引擎选择明确且严格

- `POST /v1/variants/snv/evaluations` 必须传 `engine_id=acmg_amp_2015|svc_v4`。
- `POST /v1/variants/cnv/evaluations` 必须传 `engine_id=cnv_loss_2020|cnv_gain_2020`。
- `engine_id` 与 `variant.type` 不符 → `422`；规则未就绪 → `409 RULESET_NOT_AVAILABLE`，**绝不静默切换**。
- 表型提取是独立入口，但其结果可以通过共同结构传递给两个变异评估接口。
- 为便于对比可设计未来 `POST /v1/evaluations/compare`，首版可先由客户端两次调用实现，不强行新增功能。

### 5.3 错误码

- `INVALID_VARIANT`、`INVALID_COORDINATE_SYSTEM`、`ASSEMBLY_MISMATCH`；
- `UNSUPPORTED_VARIANT_ORIGIN`、`NEEDS_CLINICAL_CONTEXT`；
- `PROVIDER_UNAVAILABLE`、`PROVIDER_CAPABILITY_MISMATCH`；
- `RETRIEVAL_UNAVAILABLE`、`EVIDENCE_UNVERIFIED`、`RULE_CONFLICT`；
- `RULESET_NOT_AVAILABLE`、`INSUFFICIENT_DATA`、`CANCELLED`、`TIMEOUT`；
- `AUTH_FAILED`、`QUOTA_EXCEEDED`、`RATE_LIMITED`。

以标准 JSON 错误体输出稳定的 code、request_id、可重试性标记，绝不将密钥或患者原文写入错误日志。

---

## 6. 关键业务模块的详细开发要求

### 6.1 HPO：从 DeepHPO 平滑迁移

当前 DeepHPO 已采用 **“LLM 预处理 → 本地索引检索 → LLM 精确匹配”**，使用前缀、关键词、trigram 同义词检索及 SSE 阶段进度。新项目应原样复现其行为并建立回归测试，而不是重新发明全词表上下文注入模式。

实施步骤：

1. 从 DeepHPO 提取测试案例、HPO 词典数据、索引构建逻辑、候选 Top-K、术语匹配提示词和输出规则。
2. 将本地检索移植到 Go；HPO OBO 和自定义中文翻译数据分开管理版本、来源和使用许可。
3. 支持否定、家族史、疑似、已证实症状，以及“推测症状不计分”政策。
4. 本地缓存 HPO 词典及索引，在进程启动时加载或 mmap（由基准测试决定）。
5. 对相同临床文本，在旧版 DeepHPO 与 SepiLens 进行并列比较，处理预期与非预期差异。
6. 产出 HPO JSON 和 TSV 导出格式；提供纯本地关键词查询接口可作为后续增强，不作为首版必须公网提供的接口。

### 6.2 ACMG/AMP 2015 引擎

1. 建立证据字典：每条证据的代码、强度、触发条件、排除条件、依赖关系、支持资料。
2. 分离注释可自动确认的条件和需人工/文献判断的条件。
3. 规则表及 ClinGen 已发表的适用修订必须可追踪引用来源和版本。
4. 对已有证据状态执行确定性组合规则与冲突处理，输出五级分类或待审核状态。
5. 所有证据单独归档，可按来源复核；分类结果不是不可解释的单个 label。
6. 建立 baseline 与单个 VCEP specification 的差异测试，避免误选特异性规则。

### 6.3 SVC v4 引擎

1. 先定义公用 `VariantClassification`、`EvidenceItem`、`EvidenceLine` 的接口映射，不锁定尚未正式发布的具体代码表。
2. 参考 ClinGen `svcv4-model` 的正式 Schema，明确 Classification Model 与 CSpec Method Model 的边界。
3. 公开正式规范后再实现不同证据逻辑与计算；同一变异可保存 v2015 和 v4 两次独立评估。
4. 已保存 v2015 历史结果绝不原位改写成 v4；新引擎生成新的 `evaluation_id` 和 `ruleset_hash`。
5. 如果临床可用性/指南发布不满足门禁，API 返回 `RULESET_NOT_AVAILABLE`，前端展示尚未开放。

### 6.4 CNV Loss 引擎

必须单独实现并覆盖：

- Section 1：片段基因组内容（含无功能元素的情况）；
- Section 2：HI/良性区域重叠、断点与基因内事件等 Loss 专有分支；
- Section 3：包含的蛋白编码基因数量及对应规则；
- Section 4：病例、表型一致性、病例对照与频率来源；
- Section 5：遗传来源、共分离、家系以及相关扣分/加分；
- 每节及各小条款的 **互斥、分值范围、上限、依赖和例外**。

对应对照基准：ClinGen **Loss** 在线计算器、2020 指南补充材料及公开示例。

### 6.5 CNV Gain 引擎

必须单独实现并覆盖：

- Section 1：片段内容；
- Section 2：TS/HI/良性区域、基因内重复、部分重叠及 Gain 专有规则；
- Section 3：编码基因数量；
- Section 4：已有病例、表型、病例对照、群体数据；
- Section 5：遗传来源和共分离；
- 重复相邻拷贝、拷贝数增益机制与其不确定性；
- 每节及各小条款的 **互斥、分值范围、上限、依赖和例外**。

对应对照基准：ClinGen **Gain** 在线计算器、2020 指南补充材料及公开示例。

### 6.6 CNV 注释层专用要求

- 基因组版本转换不能悄悄执行；要注明映射成功率与未匹配区间。基于混用 build 的证据必须阻断评分。
- 实现区间相交、完整包含、部分重叠、长度、基因数、外显子与转录本影响的精确运算；标准化坐标端点。
- ClinGen 剂量敏感性数据按 gene / region 分开，保留日期、HI/TS 等级、变更记录和证据来源。
- 支持复发性 CNV 区域及良性区域的判别，并区分“已正式评估的低敏感性”和“未评估 / 证据不足”。
- CNV 结果与检测数据质量分离：CNVkit 的 log2、seg、weight、肿瘤纯度等首先属于检出与质量背景，不直接证明 constitutional CNV 的致病性。

---

## 7. 仓库规划

```text
sepilens/
├── AGENTS.md                          # Codex 等开发代理规则
├── README.md
├── LICENSE                            # 项目独立授权
├── go.mod
├── cmd/sepilens/main.go               # serve / worker / rules validate / db migrate
├── internal/
│   ├── api/                           # chi routes / middleware / SSE / OpenAPI
│   ├── auth/                          # Key / Tenant / RBAC
│   ├── application/                   # orchestration / idempotency / budget
│   ├── llm/
│   │   ├── provider.go
│   │   ├── responses/
│   │   ├── chatcompat/
│   │   └── anthropiccompat/
│   ├── retrieval/
│   │   ├── nativeweb/
│   │   ├── pubmed/
│   │   ├── clinvar/
│   │   └── clingen/
│   ├── hpo/
│   │   ├── extractor.go
│   │   ├── index.go
│   │   └── matcher.go
│   ├── variant/
│   │   ├── normalize/
│   │   ├── snv/
│   │   └── cnv/
│   ├── evidence/                      # canonical evidence / snapshots / provenance
│   ├── rules/
│   │   ├── registry.go
│   │   ├── acmg2015/
│   │   ├── svcv4/
│   │   ├── cnvloss2020/
│   │   └── cnvgain2020/
│   ├── review/
│   ├── jobs/
│   ├── storage/
│   └── telemetry/
├── agents/
│   ├── hpo/agent.md
│   ├── acmg2015/agent.md
│   ├── svcv4/agent.md
│   ├── cnvloss/agent.md
│   └── cnvgain/agent.md
├── rules/                              # 版本化、带来源的规则资源
│   ├── acmg_amp_2015/
│   ├── svc_v4/
│   ├── cnv_loss_2020/
│   └── cnv_gain_2020/
├── schemas/                            # OpenAPI + 输入/输出 JSON Schema
├── knowledge/                          # 本体和知识库版本清单（数据可单独下载）
├── migrations/
├── tests/
│   ├── fixtures/hpo/
│   ├── fixtures/acmg2015/
│   ├── fixtures/cnv_loss/
│   ├── fixtures/cnv_gain/
│   ├── fixtures/negative_security/
│   └── integration/
├── docs/
│   ├── DEVELOPMENT_PLAN.md
│   ├── API.md
│   ├── RULESET_POLICY.md
│   ├── VALIDATION.md
│   ├── SECURITY.md
│   └── DEPLOYMENT.md
├── Dockerfile
└── .github/workflows/
```

建议把本文存为 `docs/DEVELOPMENT_PLAN.md`。首版 Go 代码与医学规则资源应保持干净的 package 边界；`internal/rules` 是规则**执行器**，顶层 `rules/` 是经评审的**版本化规则数据**。

---

## 8. 安全、隐私与合规要求（必须从第一个 Sprint 开始）

### 8.1 BYOK 的安全模型

- 终端调用不应在每次评估请求中直接传明文 Key；使用 `provider_profile_id` 引用已建立的加密 profile，或在私有化部署中使用环境变量/密钥管理器。
- Key 仅用于发往该用户明确选择的端点；静态加密，按租户隔离，禁止记录到日志和崩溃报告。
- 端点默认 HTTPS，限制重定向、DNS 重绑定、内网 IP、云元数据地址等 SSRF 场景；私有模型只能通过明确授权的受信任网络配置开放。
- 支持租户级服务端 API Key、最小权限、请求限流和耗用量上限。
- 模型端点与外部检索允许范围由管理员策略决定；不能因为用户输入 URL 就允许访问内部文件或服务。

### 8.2 患者数据

- 默认不需要患者姓名、身份证、手机号、住址等直接标识符；支持脱敏输入。
- 临床原文、家系信息、患者相关 HPO 和变异解释视为敏感业务数据。
- 用户可选择不保留请求正文，只保留匿名摘要与必要的审计元数据。
- 增强检索的查询只发送规范化变异、基因、疾病等最小必要项，不附带未授权患者详情。
- 备份、导出、保留期限、删除、权限和数据跨境策略应在正式上线前完成审查。
- 外部模型提供方的数据处理与留存政策需由调用机构自行确认；不能因 BYOK 就默认合规。

### 8.3 医学输出限制

- 所有分类均区分 `agent_proposal`、`rule_engine_result` 和 `reviewer_decision`。
- 证据不充分、规则不适用和冲突分别标记；绝不能把“没有搜索到文献”转换成良性分值。
- 若仍需临床人员审查，不输出“已批准临床报告”字样。
- 引用网页仅属于未验证内容；用官方资料或原文核对后才能纳入可计分证据。

---

## 9. 测试与验证计划

### 9.1 软件测试

- Go 单元测试、API 契约测试、SQL 迁移测试、模拟 LLM/搜索的 deterministic replay。
- Provider 兼容矩阵：真实厂商端点的 Responses、SSE、Structured Outputs、工具回调、Web Search 和计费返回值。
- JSON Schema 严格校验（拒绝未声明分数/规则代码/模型自造证据）。
- 每条任务保持幂等：同一 `idempotency_key` 下不可重复运行并重复收费。
- 任务重启恢复、取消、超时、退避重试、SSE 断线重连、缓存过期与被撤回文献处理。
- 安全测试：SSRF、提示词注入、跨租户数据泄露、密钥泄露、恶意超长文本、畸形坐标、恶意引用 URL。

### 9.2 HPO 验证

- 用 DeepHPO 既有病例作为迁移回归集。
- 用独立人工标注样本评估 HPO 术语 **precision、recall、F1**。
- 单列否定症状、家族史、中文同义词、近义词、模糊表型和错误推断的表现。
- 对高风险的模型自行扩展表型，必须能识别并阻止自动计入患者表型。

### 9.3 ACMG 2015 验证

- 对每个证据代码建立正例、反例、边界、冲突和证据缺失测试。
- 对标准组合表、VCEP适配和强度调整进行独立测试。
- 与人工专家判读及权威公开案例比较；记录差异原因。
- 确认不同文献重复计数、不同疾病/MOI 导致的适用差异，以及在缺乏关键临床背景时正确拒绝分类。

### 9.4 CNV Loss / Gain 必须分别验证

- 按 ClinGen 官方案例、2020 标准补充材料以及 [CNV 计算器](https://cnvcalc.clinicalgenome.org/cnvcalc/)建立**两套互相独立的 golden fixtures**。
- 对 Section 1–5 每个**适用子条款**准备正例、反例、边界、互斥、上限、缺失信息测试。
- 对包含多个 HI / TS 基因、部分重叠、基因内缺失/复制、特殊断点、未评估剂量敏感性等高风险情形重点检查。
- 对 −0.99 / −0.90 / −0.89 / +0.89 / +0.90 / +0.99 等分类边界设置专项测试。
- Loss 和 Gain 即使相同区间、相同输入的基因列表，也不能产生共用的评分规则记录。
- 对肿瘤来源 CNV 和未知起源 CNV，测试系统是否阻断不恰当的胚系结论。

### 9.5 上线验收门槛（初始工程建议，不是指南规定）

| 指标 | 最低验收思路 |
|---|---|
| 规则引擎的确定性 | 同一输入+同一 RuleSet 与 Evidence Snapshot，评分/分类结果 100% 可复现 |
| 规则条款覆盖 | 已声明可用的规范范围内，所有适用子条款有自动化测试；未实现规则不能标成已支持 |
| CNV 官方计算器一致性 | 已建立的每条人工核对 golden case，逐项分值与最终分类一致；差异必须关闭或明确阻断上线 |
| 引用合法性 | 计分证据具有可核验来源或清晰的内部实测来源；无来源编造 |
| 隐私 | 不泄露 API Key，不跨租户复用患者数据 |
| 人工审核 | 每次采纳/驳回均有人员、时间与理由记录 |
| 成本 | 有每次任务的 Token/检索用量明细及预算熔断 |

对 HPO、ACMG Agent 文献审查的性能目标应在完成内部独立基准集之后量化确定，不虚构“95%准确率”等没有测试依据的指标。

---

## 10. 里程碑与 Sprint 安排

> 以下是**开发依赖顺序和估算工作量**，不是已承诺的交付日期。按一名熟悉 Go / 生信、使用 AI 辅助编码的开发者安排；实际临床验证和正式规则发布可能延长。

| 阶段 | 时间预算（估算） | 主要开发内容 | 退出条件 |
|---|---|---|---|
| M0 产品与规范冻结 | 第 1 周 | 文档、医学范围、规则版本、目录、OpenAPI 草案、授权核查 | 5 个引擎 ID 与首版边界固定 |
| M1 Go 服务底座 | 第 2–3 周 | `chi`、鉴权、配置、Responses Provider、SSE、日志、任务存储、Docker | Mock 与至少一家真实 LLM 打通 |
| M2 HPO 迁移 | 第 4–5 周 | 移植 DeepHPO 三阶段流程、本体索引、回归测试 | 与原版对照完毕，支持结构化输出 |
| M3 统一证据与检索 | 第 6–7 周 | Evidence Store、外部搜索、PubMed/ClinGen、来源核验、审计 | 支持有/无联网的同一任务流程 |
| M4 ACMG/AMP 2015 | 第 8–10 周 | 证据规则、ClinGen细则注册、组合引擎、golden cases | 基线规则逐条验证完成 |
| M5 CNV Loss/Gain | 第 11–14 周 | CNV注释、HI/TS、两套 rubric、确定性评分、官方对照测试 | Loss/Gain 均可单独生成审计结果 |
| M6 上线准备 | 第 15–16 周 | 安全审查、异常场景、部署、SDK 示例、云平台联调 | 版本候选发行，文档与测试齐全 |
| M7 SVC v4 | **与正式指南/ CSpec 发布时间绑定** | schema 映射、独立评分实现、标准案例验证 | 正式发布+规则验证+人工批准后单独启用 |

若人力受限，可先发布 **HPO + ACMG 2015** 私有测试版，再以小版本增加 CNV Loss 和 Gain；但对外宣称完整首版时，应完成所有标记“完整上线”的引擎。

### 10.1 开发任务优先级

**P0 必做**

- [ ] Go 项目骨架、OpenAPI、基础 CI 与 Docker。
- [ ] Responses Provider 适配与自定义 endpoint/model/key。
- [ ] Key 安全和 SSRF 防护。
- [ ] HPO 迁移、旧版回归和术语来源管理。
- [ ] ACMG/AMP 2015 基线规则与证据来源审计。
- [ ] ClinGen CNV Loss/Gain **两套**完整评分表与计算器对照测试。
- [ ] 可选联网搜索、原始证据核验、搜索异常处理。
- [ ] 异步任务持久化、幂等请求、人工审核状态。
- [ ] 不支持的医学场景正确拒绝/降级，不生成伪正式结论。

**P1 后续增强**

- [ ] 特定 VCEP/CSpec规则批量导入与版本差异查看。
- [ ] CNV 与 HPO 自动关联、断点复杂结构辅助展示。
- [ ] 双规范对比（ACMG 2015 / SVC v4）。
- [ ] 文献全文结构化抽取与重复病例溯源。
- [ ] PostgreSQL集群部署与团队审核工作流。
- [ ] SDK（Python、TypeScript、Go）和 Web 控制台。

**P2 远期扩展**

- [ ] 肿瘤体细胞致癌性、临床意义分级独立引擎。
- [ ] RNA fusion / CNV 体细胞靶向治疗证据整理。
- [ ] 自动周期性知识库更新、历史结果变化提醒。
- [ ] 本地轻量模型、离线/私有化多节点部署。

---

## 11. 与云平台集成及商业化

SepiLens 不直接记录余额或执行积分交易，只输出 `job_id`、结果、是否成功交付、模型/搜索用量及必要审计字段。

云平台负责：

1. WES 原始分析按计算分钟计费（既有规则：约 45 分钟/样本 → 约 45 积分，仅作为业务参考）。
2. 解读人员在已过滤、复核的少量位点上主动触发增强判读。
3. 展示“本次 1 积分”的确认弹窗，并支持“本次会话不再提示”；免弹窗**不表示免付费**，按钮仍显示价格。
4. 通过外层 Billing Gateway 预冻结 1 积分 → SepiLens 执行 → 成功形成符合约定的可审核结果才结算 → 系统失败释放冻结。
5. `insufficient_data` 如属于完整交付的有效判读结果，可以按产品条款收费；外部模型报错、解析失败、任务超时等不应当作已成功交付。
6. 旧结果免费查看；用户明确选择“重新生成/更新证据”时再开启新的收费任务。
7. **BYOK 双重费用透明**：用户供应商 Key 可能产生 Token/搜索费用；平台的 1 积分属于 Agent 编排服务费，必须明确告知。
8. 缓存公共文献/规则而非跨租户缓存患者级结论；遵循对应数据源许可与访问约束。

### 11.1 为什么这不是“批量LLM跑5万位点”

按既有产品策略，固定生信注释与规则初筛是批量层；解读复核后的少数 SNV/InDel 或 CNV 才进入 SepiLens。**默认单事件评估，每项单独审核、单独计费**。这样控制外部请求成本、医学风险和资源竞争。

---

## 12. 初期发行规划

| 发行 | 预期交付 | 临床相关限制 |
|---|---|---|
| `v0.1.0` | Go API + Providers + BYOK + SSE + HPO | HPO 提取为辅助结构化，不自动推断事实 |
| `v0.2.0` | ACMG/AMP 2015 + 结构化证据、规则验证 | 先限定已验证的适用范围 |
| `v0.3.0` | CNV Loss + Gain 两个独立引擎 | 明确只用于 constitutional CNV |
| `v0.4.0` | Web Search + 专业检索 + 完整审核与任务 | 来源、预算、权限与复现性达标 |
| `v1.0.0` | 多厂商联调、隐私加固、完整文档、云平台接入 | 有经验证的功能清单及限制声明 |
| `v1.x` / `v2.x` | SVC v4 独立可用 | 以正式指南、CSpec可用及临床验证通过为前提；版本号由实际发布情况确定 |

与上面的开发 Sprint 允许交叉推进：例如检索模块可与 ACMG 2015 并行，发行版本是用户可见的能力门禁，不是所有代码必须严格串行完成。

---

## 13. 文档交付清单

正式发布至少包含：

- `README.md`：定位、架构、Quick Start、功能范围。
- `docs/DEVELOPMENT_PLAN.md`：本计划。
- `docs/API.md` + `openapi.yaml`：完整 API 协议与样例。
- `docs/RULESET_POLICY.md`：所有医学规则来源、版本、适用性、已实现条款和例外。
- `docs/VALIDATION.md`：HPO、ACMG 2015、CNV Loss 和 Gain 独立验证报告；v4 在未达标时明确禁用。
- `docs/PROVIDER_MATRIX.md`：各端点支持 Responses、Structured Outputs、SSE、原生联网等能力的实际测试结果。
- `docs/SECURITY.md`：BYOK 密钥保护、SSRF 防护、隐私、保留和删除策略。
- `docs/DEPLOYMENT.md`：Docker、环境变量、SQLite/PostgreSQL、备份恢复。
- `CHANGELOG.md`：规则 / 数据库 / Schema / 接口每次兼容性变化记录。
- `THIRD_PARTY_NOTICES.md`：DeepHPO代码重新授权记录，以及 HPO、ClinGen、注释数据库和依赖许可证核验结果。

---

## 14. 当前最值得立即开展的前五项工作

1. **先建仓库和稳定的 `engine_id` 注册表**：防止四套分类规则日后混成一个 `acmg` 目录。
2. **先完成 Go Responses 适配与结构化输出协议**：跑通一条脱敏 HPO 示例和一个模拟的评估任务。
3. **迁移 DeepHPO 的三阶段流程**：复用既有成果，形成第一个可独立发布的业务能力。
4. **并行整理三套已发布规则的机器可读定义**：ACMG 2015、CNV Loss 2020、CNV Gain 2020，先编写黄金案例再实现分类函数。
5. **建立 SVC v4 发布跟踪和 `disabled` 门禁**：技术上预留接口，但不能阻塞当前成熟能力的上线，也不能提前宣称临床支持。

**项目验收核心不在于 LLM 写出多自然的医学文字，而在于证据是否可靠、规则是否正确、人工审核是否清晰、同一输入和规则版本是否可以复现。**

---

## 15. 官方资料与实施依据（核对日期：2026-10-10）

1. **ACMG/AMP 2015 序列变异指南**（Richards 等）：https://pubmed.ncbi.nlm.nih.gov/25741868/
2. **ACMG/ClinGen constitutional CNV 技术标准**（Riggs 等，2020）：https://www.nature.com/articles/s41436-019-0686-8
3. **ClinGen 官方 CNV Calculator**：https://cnvcalc.clinicalgenome.org/cnvcalc/
4. **ClinGen CNV Loss rubric**：https://cnvcalc.clinicalgenome.org/cnvcalc/cnv-loss
5. **ClinGen CNV Gain rubric**：https://cnvcalc.clinicalgenome.org/cnvcalc/cnv-gain
6. **ClinGen 剂量敏感性数据下载**：https://search.clinicalgenome.org/kb/downloads
7. **ClinGen 剂量敏感性工作组**：https://clinicalgenome.org/curation-activities/dosage-sensitivity/
8. **ClinGen SVC v4 Classification Model**：https://github.com/clingen-data-model/svcv4-model
9. **ClinGen CSpec**：https://cspec.clinicalgenome.org/
10. **OpenAI Responses Web Search**：https://developers.openai.com/api/docs/guides/tools-web-search
11. **OpenAI 官方 Go SDK**：https://github.com/openai/openai-go
12. **DeepHPO 原项目**：https://github.com/pzweuj/DeepHPO

> **规范冻结政策**：实施前须再核对官方文献、补充材料及 ClinGen 更新页。2026 年的 ClinGen Dosage Sensitivity Curation SOP v2 主要是剂量敏感性证据整理流程的变化，**不能据此把 2020 年 CNV 分级标准当作已经被整体替代**。所有将来正式发布的修订，均需建立新 `ruleset_version` 并重新验证。
