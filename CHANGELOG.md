# 变更记录 (CHANGELOG)

## [v0.1.0-alpha] - 2026-10-10

### 初始发布

#### 核心引擎
- **HPO 提取引擎 (`hpo_extract`)**：迁入并重构 DeepHPO 本地本体库（20,384 条术语），实现高并发倒排索引与基于字符 3-gram 的同义词扩展器，支持三阶段提取、否定语义拦截与 TSV/SSE 导出。
- **ACMG/AMP 2015 引擎 (`acmg_amp_2015`)**：实现完整 28 个证据代码字典与 5 级强度修饰；实现 Richards 2015 Table 5 全部 14 条确定性组合判定路径；实现致病与良性冲突仲裁状态机；实现 ClinGen VCEP 细则注册表（含 CFTR 典范细则）。
- **CNV Loss 2020 引擎 (`cnv_loss_2020`)**：实现 2020 ClinGen Loss Rubric 完整 Section 1–5 判定与上限封顶，对齐官方 Loss 计算器。
- **CNV Gain 2020 引擎 (`cnv_gain_2020`)**：实现 2020 ClinGen Gain Rubric 完整 Section 1–5 判定与三倍敏感性评估，对齐官方 Gain 计算器。
- **SVC v4 门禁 (`svc_v4`)**：预留接口定义，严格保持 409 门禁阻断，确保正式指南发布并验证前不输出非成熟结论。

#### 证据与检索
- 实现不可篡改证据快照（SHA-256 锁定）与 SQLite 持久化。
- 实现专业检索协调器（ClinVar、ClinGen 剂量敏感性、PubMed），支持 off、targeted、deep 三档模式。
- 实现外部不可信内容净化器，防御 Prompt 注入攻击。

#### 安全与云平台集成
- 完善云平台计费网关握手协议（`delivery_status`、`settlement_eligible`）。
- 实施 SSRF 严格防护（拦截环回、私网及云厂商元数据地址）。
- 实现多租户资产隔离与 API 密钥日志脱敏。
- 交付完整 OpenAPI 3.1 规范与生产 Docker 配置。
