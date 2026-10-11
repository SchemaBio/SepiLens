# 证据和任务流水线

输入校验、坐标规范化与版本锁定 → 加密持久化任务 → 返回 queued → worker 检索/模型提议 → 结构校验和来源核验 → 确定性规则 → 结果/快照/终态/事件一次事务提交 → 专业审核或证据修正 → 新版本审计。

`agent_proposal`、`rule_engine_result`、`reviewer_decision` 分层返回。模型或调用方提交的 met / verified 不受信，会降为提议；专业审核可在记录存在、标识有效的来源上确认对象和材料，留下 actor、时间和理由。规则只计算完整核验及适用性事实满足的材料。

PubMed 和 ClinVar 使用 NCBI 官方接口；ClinGen 使用锁定快照。PMC 仅开放访问子集且识别 CC BY 3.0/4.0 或 CC0 许可 URL 的记录可用于全文。未知或限制性许可保留摘要与缺口。SNV ClinVar 匹配 assembly/chr/VCF pos/ref/alt；CNV 只检索区域，复杂事件匹配由专业复核完成。

off 禁止在线检索；targeted 最多 3 次 HTTP、15 秒；deep 最多 10 次、30 秒。search/fetch/全文/失败请求共用预算，不把检索失败视为良性。预算不足保留缺口。deep 在精确查询无结果时补查基因，并通过 PubMed elink 追踪第一条记录的一层引用（最多 3 条）；补查和引用只形成候选材料，不自动证明疾病匹配。

租约到期恢复尚未外部付费调用的任务；调用开始后遇到不确定中断直接失败，要求显式重新生成。取消以条件更新阻止后续完成，HTTP 断开不等于取消。SSE 从 Last-Event-ID 继续轮询持久化事件，心跳并在终态关闭。

仅启用引擎的正常 production 事务交付可结算，含正常 insufficient_data；失败、取消、simulation、unknown 来源的 evidence_only 和历史未验证结果不可结算。免费读取既有结果不重复模型调用；重新生成需新幂等键。
