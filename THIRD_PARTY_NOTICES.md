# 第三方组件与数据记录

SepiLens 源码使用仓库 LICENSE（Apache 2.0）。构建依赖的完整版本记录由 `go list -m -json all` 输出到交付 `dependencies.json`；已移除未使用的 OpenAI SDK，模型协议由 HTTP 适配器实现。

直接依赖：go-chi/chi v5（MIT）、modernc.org/sqlite（BSD-3-Clause）。传递依赖许可证请随精确锁定版本的源码核对；依赖清单不是再分发法律批准文件。

数据和医学规则的许可独立于软件许可证：

| 资产 | 本轮状态 |
|---|---|
| 原有 DeepHPO 衍生中文 HPO 文件 | 仓库原声明称作者已授权，但本轮未取得完整书面授权/翻译来源版本证据；生产再分发许可仍待确认 |
| 官方 HPO OBO | 支持从官方锁定版本导入；清单必须记录当版许可及获取时间；本轮未提供新的完整官方发布数据 |
| NCBI RefSeq / ClinVar / PubMed | 官方接口和导入格式已实现；实际数据版本及许可须纳入部署清单 |
| ClinGen 剂量记录与 CNV rubric | 来源见官方 Loss/Gain 页面；技术规范用于规则核对；完整数据集再分发许可须单独确认 |
| PMC 全文 | 仅开放子集、且许可 URL 可识别为 CC BY 3.0/4.0 或 CC0；每条保留许可及获取时间 |

原文医学指南的版权不因程序规则实现而改变。交付不包含专家批准，也不把未确认数据许可标为通过。镜像/包中的原有中文资源仅限现有项目内工程验收，完成许可确认前不能对外生产发布。

官方参考：[HPO](https://hpo.jax.org/)、[ClinGen Loss](https://cnvcalc.clinicalgenome.org/cnvcalc/cnv-loss)、[ClinGen Gain](https://cnvcalc.clinicalgenome.org/cnvcalc/cnv-gain)、[NCBI API](https://www.ncbi.nlm.nih.gov/home/develop/api/)、[PMC 使用条件](https://pmc.ncbi.nlm.nih.gov/tools/openftlist/)。

## ClinGen SVC v4 draft reference

Native scoring derives from clingen-data-model/svcv4-model ef66faff51a265fef7b5c4e6439905f3aa540c46, MIT copyright 2026 ClinGen Data Platform. Full license: internal/rules/svcv4/UPSTREAM_LICENSE. Narrative/data licenses require separate review.
