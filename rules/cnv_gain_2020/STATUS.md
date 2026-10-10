# ClinGen/ACMG 2020 CNV Gain 规则状态

- **引擎标识**：`cnv_gain_2020`
- **参考规范**：Riggs et al. 2020 (Technical standards for the interpretation and reporting of constitutional copy-number variants)
- **在线计算器**：https://cnvcalc.clinicalgenome.org/cnvcalc/cnv-gain
- **启用状态**：`enabled` (`enabled: true`)
- **适用范围**：仅限 Constitutional (胚系) CNV 增益/重复，明确拒绝体细胞肿瘤变异。
- **独立性声明**：与 `cnv_loss_2020` 完全隔离，拥有独立的条款分支与基因数阶梯（Gain: 35/50 vs Loss: 25/35）。
- **实现架构**：
  1. 完整实现 Section 1–5 增益专项条款及三倍敏感性（TS）判定；
  2. 串联重复（tandem duplication）未明方向或基因功能影响的情形保留不确定性说明；
  3. 采用定点整数（百分位，100 = 1.00）精确计分，无二进制浮点误差；
  4. 支持 ClinGen 官方计算器黄金用例 100% 一致性自动化比对。
