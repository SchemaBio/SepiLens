# ClinGen/ACMG 2020 CNV Loss 规则状态

- **引擎标识**：`cnv_loss_2020`
- **参考规范**：Riggs et al. 2020 (Technical standards for the interpretation and reporting of constitutional copy-number variants)
- **在线计算器**：https://cnvcalc.clinicalgenome.org/cnvcalc/cnv-loss
- **启用状态**：`enabled` (`enabled: true`)
- **适用范围**：仅限 Constitutional (胚系) CNV 缺失，明确拒绝体细胞肿瘤变异。
- **实现架构**：
  1. 完整实现 Section 1–5 缺失专项条款及上限封顶逻辑；
  2. 采用定点整数（百分位，100 = 1.00）精确计分，无二进制浮点误差；
  3. 与 `cnv_gain_2020` 引擎保持独立隔离，分项上限（Section 2: ±1.00; Section 3: +0.90; Section 4: -1.00..+0.90; Section 5: ±0.90）独立生效；
  4. 支持 ClinGen 官方计算器黄金用例 100% 一致性自动化比对。
