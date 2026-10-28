# AI 建议质量评测

`evaluate.py` 是离线汇总工具，不联网、不读取凭据、不调用模型。生产发布和模型执行不属于此脚本职责。

```powershell
python -m unittest discover -s scripts/quality-eval -p test_*.py
python scripts/quality-eval/evaluate.py .quality-eval/labels.json .quality-eval/captures.json --output .quality-eval/report.json
```

## 标签与比较约束

- 每份数据集只使用一种标签来源：`engineering`、`clinvar_reference` 或 `independent_review`。工程回放不能混入临床指标。
- 独立复核标签必须关联复核记录的 SHA256。ClinVar 聚合分类不能生成条款级标签；该来源的 `clauses` 必须为空，条款准确性指标返回 null。
- ClinVar 分类一致率仅纳入对象精确匹配、无冲突、审核至少二星且有明确五级分类的记录。未匹配、低审核和冲突记录单独计数。VUS 一致不代表诊断正确。
- 记录 `evaluation_condition`：`engineering_replay`、`held_out_reference` 或 `annotation_assisted`。模型可见 ClinVar 分类、星级、复核结论或其规则派生标签时，只能报告 annotation-assisted 一致性，不能作为独立准确性提升依据。保留与屏蔽 ClinVar 的实验不得混在同一份捕获中。
- baseline 和 evidence_chain 必须固定输入、证据包及模型配置 SHA256，重复运行编号单独记录。检索需固定回放，不能用实时检索差异解释开关收益。
- 数据集只保存独立案例编号、哈希、标签和审计标志。患者、样本、家系、任务标识和临床原文不进入该格式；实际变异输入应在受保护、未跟踪的评测目录内单独保管。

`test_evaluate.py` 的 fixture 展示完整最小格式。`verified`、`defaulted`、冲突及跨引擎/重复计分标志必须由服务审计或独立工程检查产生，不能直接映射模型的 `facts_confirmed`。同一 code 的多次输出逐项计数，不以最后一项覆盖前一项；是否重复计分以独立审计标志判断。

## 解读报告

按引擎及开关分别报告错误条款采纳率、证据支持率、关键遗漏率、冲突识别率、有效建议覆盖率、分类参照一致率、未标注条款、服务失败和实际执行覆盖。null 表示缺少有效分母。全部返回未知会增加关键遗漏并降低有效覆盖，不能算改进。未执行引擎和预算跳过必须保留。

该工具不自动宣告验收通过，也不证明临床准确性提升。没有 CNV 或 SVC 独立标签时，不得用 SNV ClinVar 一致率替代这些引擎验收。SVC 正式规则仍保持禁用。

## 本轮费用约束

用户批准真实模型评测总费用上限 **5 元人民币**。只限提供商模型费用，不应通过正常用户任务产生 SaaS 积分消费。执行器必须在每次请求前按已核实的高峰输入/输出价格和明确 token 上限预留费用，包含纠正请求；无法取得 usage 的请求保留整个预留，不按零费用处理。费用、未决预留之和触及上限即停止，不自动追加预算。

`budget.py` 提供独立 SQLite 预留账本，使用整数微元、多连接事务和持久预留。它不会发起请求或自行限制模型输出；后续执行器必须将 token 上限实际施加到每一条提供商请求。没有接入执行器前，不能声称已具备端到端费用硬限额。usage 按未命中高峰价计算保守上界，不能将其误称为提供商实际账单。

2026-10-28 核实的官方 DeepSeek Flash 高峰价格为输入未命中 2 元/百万 token、输出 8 元/百万 token；不能假定缓存命中或闲时折扣。执行时再次核价。没有可核实对象和标签的数据，不应先花费预算再解释。

参考：[ClinVar review status](https://www.ncbi.nlm.nih.gov/clinvar/docs/review_status/)、[ClinVar clinical significance](https://www.ncbi.nlm.nih.gov/clinvar/docs/clinsig/)、[DeepSeek 价格](https://api-docs.deepseek.com/zh-cn/quick_start/pricing/)。
