# 结构化注释与表型驱动评估：工程交付进度

版本：v2.0；更新日期：2026-10-19。

本轮完成 M2b → M4 → M5 → M6 的工程路径。医学批准、真实数据许可确认和独立临床校准仍为独立门禁，五个引擎继续默认 disabled，AI 总开关默认 disabled。

## 里程碑

| 阶段 | 工程结果 | 验收范围 |
|---|---|---|
| M0 / M1 / M2a / M3 | 延续已有实现；修复来源伪造、PVS1 隐式前提、过期数据和降级强度接线 | 现有回归及新增负向案例 |
| M2b | 版本化离线资产、流式 TSV 解析、只读 SQLite 位点/基因/蛋白残基索引、v2 加密快照 | 四类索引来源及 HPOA 合成夹具；百万行性能测量 |
| M4 | OBO 与中文翻译合并、HPOA/背景频率、六种关系、排名/margin、十项 PP4 门禁和匹配 API | 关系、单调性、平局、单候选、十项逐项失败和权限检查 |
| M5 | 双协议多轮工具、固定对象只读工具、预算、引用/事实校验、覆盖与重算、失败保存基线 | 两协议本机 mock 全流程、六种 change、伪造引用和遗漏 |
| M6 | 已保存条款上的人工修订、不可变历史、审核结算、审计/密钥轮换/删除兼容 | Windows / Linux 合成验证及 Linux race |

## 已确定的行为

- 生产注释通过 options.annotation_source=sepilens_snapshot 选择，不能混入请求注释；请求中的来源声明不能授予可信身份。
- 支持 ClinVar variant_summary、GenCC legacy、AlphaMissense 2023 及明确格式的 gnomAD 导出 TSV。Pangolin 保留调用方输入，可信快照状态为 unavailable。
- 缺失、过期和不可用均不产生良性推论。队列固定数据集、profile 版本与评估时间；旧版本不可用时明确失败。
- v2 快照覆盖注释、表型、agent 和 overrides；缺省 v1 保持原散列算法，旧快照不原位改写。
- 候选疾病为当前基因的 Moderate/Strong/Definitive 断言与请求疾病的并集，不替换请求疾病，不按名称猜测映射。
- clinician_verified 仅对 reviewer/admin 身份保留；submitter 的该声明降为 caller_supplied。阴性术语仅在临床确认且频率明确或有明确排除注释时计入。
- PP4 单候选、未确认单基因病因、过期数据、竞争诊断均不能 met；缺乏完整 GenCC 覆盖声明时不推断单一病因。
- AI met 需要独立服务端事实，模型陈述和摘要不足以增加已满足证据；新材料可经人工 revise_evidence 核验采纳。
- 无依据覆盖保留本地值并记录 unsupported_override，遗漏记录 ai_omitted；模型自报分类不参与正式计算。
- AI 失败保存 local_baseline、用量和脱敏错误码，任务为 failed；读取部分结果时 delivery_status=failed。未审核 AI 结果不可结算。
- 人工修订保留未修改条款，增加 revision 并清除旧审核。表型变化应新建评估，不能用条款修订绕过 PP4 门禁。

## 验证与文档

全量 Go 测试、vet、Windows 构建、Linux race 及双平台 smoke 使用合成数据验证；OpenAPI 从生成器重建，当前 23 个操作、69 个 schema。

百万行合成索引：Windows amd64、16 个逻辑处理器；300 次预热查询采样 p95 为 0.960ms，导入 22.314 秒，采样 Go heap 峰值约 4.49MB，索引约 200MB。冷查询本次计时为 0ms（低于计时分辨率，不代表零成本）。Go heap 指标不等同于进程峰值 RSS，也不代表真实数据导入性能。

操作及迁移见 [快照与评估说明](SNAPSHOT_ASSESSMENT_V2.md)；验证产物见 [本轮验证目录](reports/next/README.md)。

## 后续独立工作

1. 部署者确认真实数据许可、版本、散列和导出流程，完成真实规模导入。
2. 用 scripts/calibrate_phenotype.py 测量固定种子的合成查询术语留一排名。脚本不移除参考库注释，不属于独立临床验证。
3. 专业团队建立独立病例集，完成医学阈值校准、黄金案例报告和签名批准包。
4. 按批准包显式启用适用引擎；AI 另需 SEPILENS_AI_MODE=enabled。CNV、SVC v4 和全仓库行尾归一化不属于本轮变更。
