# SepiLens 医学规则集管理与门禁策略

## 1. 五引擎独立注册与生命周期

SepiLens 系统固定包含五个引擎标识，所有引擎在代码与规则资源层严格隔离：

| 引擎标识 | 指南基准 | 适用对象 | 首版策略 |
|---|---|---|---|
| `hpo_extract` | HPO 本地本体 + LLM 精确匹配 | 临床表型文本 | 移植 DeepHPO 流程，独立回归集 |
| `acmg_amp_2015` | Richards et al. 2015 | 胚系 SNV/InDel | 确定性组合表，ClinGen 细则注册 |
| `svc_v4` | ClinGen SVC v4 官方标准 | 胚系 SNV/InDel | 保持禁用，满足就绪门禁后启用 |
| `cnv_loss_2020` | Riggs et al. 2020 (Loss) | 胚系 CNV 缺失 | 独立评分表与 ClinGen Loss 计算器对齐 |
| `cnv_gain_2020` | Riggs et al. 2020 (Gain) | 胚系 CNV 增益 | 独立评分表与 ClinGen Gain 计算器对齐 |

## 2. SVC v4 就绪门禁

1. **正式标准发布**：以 ACMG 与 ClinGen 联合发布的正式 SVC v4 序列变异解释指南为准；
2. **规范版本锁定**：CSpec 方法模型与条款字典冻结并形成机器可读规则；
3. **代码实现与验证**：实现确定性分类逻辑并通过标准黄金用例回归；
4. **人工审批准入**：经医学与法规专家审核后方可由管理员手动开启。在此之前，API 保持 `409 RULESET_NOT_AVAILABLE`，禁止降级到 2015 规则。

## 3. CNV Loss 与 Gain 独立性

- **Loss 与 Gain 具有完全不同的条款分支、上限设定与证据逻辑**（例如 haploinsufficiency 对比 triplosensitivity）；
- 相同坐标区间的 Loss 和 Gain 评估必须生成完全隔离的规则记录，禁止共用条款判定或分值聚合。

## 4. 2020 CNV 标准与 Dosage Sensitivity SOP 关系

- ClinGen Dosage Sensitivity Curation SOP 主要是剂量敏感性证据整理流程规范，并未废止或整体替代 2020 年发表的 constitutional CNV 技术标准；
- 本系统在计算 CNV 致病性评分时，以 2020 ACMG/ClinGen 联合技术标准正文、补充材料及官方在线计算器为准；
- 剂量敏感性数据作为输入知识源之一，保留版本戳与策展日期。
