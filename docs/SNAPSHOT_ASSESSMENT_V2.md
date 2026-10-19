# 注释快照、表型与 AI 评估操作说明

2026-10-19；工程默认参数，未作医学批准。

## 资产导入

沿用 `sepilens data import -manifest FILE -output DIR` 和 `data verify -output DIR`，通过 `serve -bundle-dir DIR` 加载。索引单独存于 bundle，运行期只读，不与患者数据库共表。

每个资产固定 path、url、sha256、version、license、kind；新增数据资产还要求 format、retrieved_at。license_confirmed 默认 false，max_age_days 默认 90；位点来源必须提供 assembly，不隐式 liftover。complete 是部署者对完整覆盖的显式声明，不能从少量记录推断。

| kind | format | 输入 |
|---|---|---|
| hpo_obo | 沿用旧契约 | 官方 hp.obo，含 is_a、alt_id、replaced_by |
| hpo_translation | hpo-translation-v1 | DeepHPO 风格 ID → Term JSON，仅合并 name_cn、definition_cn |
| hpoa | hpoa-v1 | database_id、hpo_id、qualifier、frequency |
| gencc_tsv | gencc-legacy-v1 | gene_symbol、disease_curie、classification_title、moi_title、submitter_title |
| alphamissense_tsv | alphamissense-2023 | CHROM、POS、REF、ALT、transcript_id、protein_variant、am_pathogenicity、am_class |
| clinvar_snapshot | clinvar-variant-summary-v1 | AlleleID、VariationID、Assembly、Chromosome、PositionVCF、ReferenceAlleleVCF、AlternateAlleleVCF、GeneSymbol、ClinicalSignificance、ReviewStatus、PhenotypeIDS；Name 可提供蛋白与转录本 |
| gnomad_tsv | sepilens-gnomad-v1 | chrom、pos、ref、alt、popmax_af、faf95、allele_number、coverage_adequate |

gnomAD 输入是部署者预先从锁定官方数据生成的 TSV 投影，需预置本地文件，并审计保留上游来源及导出过程；它不是官方通用 TSV。缺少频率或覆盖信息时不得补零或自动声明充分覆盖。当前不直接解析原始 gnomAD VCF/Hail。GenCC 明确选择 legacy，未知格式拒绝加载。

来源说明：[ClinVar](https://www.ncbi.nlm.nih.gov/clinvar/docs/ftp_primer/)、[GenCC](https://thegencc.org/download)、[AlphaMissense](https://storage.googleapis.com/dm_alphamissense/README.pdf)。来源链接不构成许可批准。

TSV 流式解析，拒绝缺少必要表头、列数错误和非法数值；无适用坐标的 ClinVar 行跳过。多条不同记录保存在索引，查询时返回 conflicting。查询支持精确位点、基因–疾病–MOI 及限定版本转录本的蛋白残基，不能跨蛋白猜测等价位置。

导入在临时数据库完成事务、索引和完整性检查后发布；manifest 记录索引 SHA256 与输入绑定散列。加载时校验源文件与索引。升级发布新 bundle，勿手工改写已发布索引。

## 请求与输出

```json
{
  "engine_id":"acmg_amp_2015",
  "mode":"local",
  "variant":{"assembly":"GRCh38","chrom":"1","pos":100,"ref":"A","alt":"G","gene":"TEST","transcript":"NM_000001.1","consequence":"missense_variant"},
  "clinical_context":{"disease_id":"OMIM:1","inheritance":"AD","variant_origin":"germline_confirmed","sample_type":"blood"},
  "phenotype":{"source":"caller_supplied","hpo_terms":[{"id":"HP:0001945","polarity":"present"}]},
  "options":{"annotation_source":"sepilens_snapshot","combination_method":"acmg_2015_table"}
}
```

调用方提交注释时省略 annotation_source 或设为 caller_supplied，并提供 annotations。未核验事实不会进入正式应用条款，调用方注释不可结算。points 是候选条款的说明性计算；顶层 classification 服从服务端事实与来源校验。

local SNV 路径不调用模型或在线检索；自动派生首轮仅接受常染色体。PVS1 不从 GenCC 有效性或转录本编号推导机制和临床相关性，缺少依据保持 insufficient_data。

`POST /v1/phenotype/match` 接受 gene、disease_id、inheritance、phenotype，返回 match 和 pp4，不分类变异。固定 sepi-pheno-0.1，返回 HPOA、本体和翻译版本；只认显式疾病 ID，无法映射时返回缺口。平局保留相同名次并按 ID 稳定排序，单候选无 margin；未确认或频率未知的阴性术语不计分，阴性冲突不提高支持度。

AI 需签名批准的 production 引擎、SEPILENS_AI_MODE=enabled 和 options.provider_profile_id。profile 版本固定；双协议保留工具调用 ID 与结果关联，参考 [官方 Function calling 文档](https://developers.openai.com/api/docs/guides/function-calling)。工具循环使用非流式 Generate，原有普通文本流接口保留。

工具仅 get_annotations、get_phenotype、search_literature，参数为空对象，查询对象由服务端固定。max_searches 默认 12、硬上限 100，底层 HTTP 请求共用预算，无隐藏重试；模型循环最多 12 轮、100 次工具调用、3 分钟。输出和单项工具结果各限 256KiB，模型输入限 1MiB。retrieval_mode=off 禁止在线检索。

输出 local_baseline、agent_proposal.ai_assessment/reconciliation/major_override，以及服务端重算的 rule_engine_result。模型自报分类仅留作建议；伪造引用、无独立事实、非法 change 和超限强度不能生效。强度升级还需服务端授权的规范来源，模型自填 strength_basis 不足以获准。

AI 失败任务保留本地基线、用量和快照。GET jobs 状态为 failed；GET jobs/result 对有部分结果的失败任务返回 200，但 delivery_status=failed、failure_code 非空且不可结算。无部分结果的失败任务仍返回 409。客户端不能仅以 HTTP 200 判定成功。

## 迁移、审核与回滚

启动自动执行迁移 007：在 evidence_snapshots 增加可空 extensions_json，按租户/快照 ID 绑定 AES-GCM 加密，密钥轮换覆盖它。旧行无需回填。v2 散列覆盖 annotations、phenotype、agent、overrides；缺省 v1 的原字节算法不变。拒绝未知版本和带扩展字段的 v1 载荷。

升级前备份患者数据库和数据 bundle。旧程序不能可靠读取新增 v2 结果；回滚应恢复升级前数据库备份，勿原位降级或修改历史散列。

revise_evidence 按规则代码合并已保存的有效条款，保留未修改条款，追加 revision 和快照并清除旧审核。PP4 修订不得绕过已保存的表型门禁，表型变化需新建评估。审核仅对当前 revision 生效；结算仍需批准、可信来源及有效数据，人工判定独立存入 reviewer_decision。

租户删除和保留期清理删除整条快照与历史，扩展字段随行删除；公共知识索引不含患者数据。审计导出需要 reviewer/admin 权限，包含当前快照与完整历史。

## 验证与校准

百万行测量：设置 SEPILENS_INDEX_BENCHMARK=1，可选 SEPILENS_BENCH_REPORT，再运行 `go test ./internal/annotation -run TestMillionRecordIndexPerformance -count=1 -v`。普通测试跳过此任务；内存指标是采样 Go heap，并非峰值 RSS。

`scripts/calibrate_phenotype.py --hpoa FILE --gencc FILE --output REPORT` 通过运行中的匹配端点评分，令牌从 SEPILENS_CALIBRATION_TOKEN 读取。固定种子留出合成查询中的一个术语，报告 top1/top3/MRR 和输入散列；不移除参考库注释，不报告临床 precision/recall，不替代独立医学验证。
