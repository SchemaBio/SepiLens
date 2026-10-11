# SVC v4 草案交付核对

核对日期：2026-10-11。按用户要求开展当前草案开发，锁定 main 提交 ef66faff51a265fef7b5c4e6439905f3aa540c46（2026-09-24），服务启动不追踪 main。

[上游代码](https://github.com/clingen-data-model/svcv4-model/tree/ef66faff51a265fef7b5c4e6439905f3aa540c46/src/svcv4_model/scoring) 实际包含分类、聚合、临床、位点、人群及变异影响参考实现。README 未覆盖全部代码，不能据此认定仓库只有数据结构。参考实现明确为 NON-AUTHORITATIVE，正式方法以 CSpec 为准。

本轮交付 Go 原生移植、严格输入校验、精确十进制计分、版本锁定和实验 API。接受分析人员提交的已编码方法输入，不声称自动完成原始注释、预测器校准、功能实验 OddsPath 计算或全部 CSpec 决策树。

5,245 组移植对照中，5,240 组与上游一致；5 组 POP_FRQ 的 15 倍边界按精确十进制修正为 -6，上游浮点结果为 -3。夹具保留原值、修正值及例外标签。对照期望来自上游程序，不是独立医学黄金案例。

[上游已知缺口](https://github.com/clingen-data-model/svcv4-model/blob/ef66faff51a265fef7b5c4e6439905f3aa540c46/docs/reference/known-gaps.md) 尚未关闭，包括 LOC 入口条件与部分区间推断、DNV 区域条件、XLR 非分离、POP_HMZ 表格与正文差异、CCS 材料条件、剪接上限和最终基因疾病有效性上限。config 有 motif 配置，但参考 scorer 延后该方法；本轮不据此宣称完成 motif 或尚未发布的 noncoding 方法。

实验结果存于 agent_proposal.experimental_result；正式结果为 null，输入未核验，禁止临床用途与结算。未 opt-in 返回 409，体细胞来源返回 422，未知来源仅整理证据。审核接口不能把实验结果采纳成正式结果。

临床验收待办：正式规范及方法模型对齐、独立逐条黄金案例、适用性与相关性核验、专家批准。厂商实际联调、完整官方数据及其许可也不会因实验交付而自动通过。操作见 [实验接口](SVC_V4_EXPERIMENTAL.md)。
