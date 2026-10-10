# ACMG/AMP 2015 规则状态

- **引擎标识**：`acmg_amp_2015`
- **指南基准**：Richards et al. 2015 (Standards and guidelines for the interpretation of sequence variants)
- **PubMed**：https://pubmed.ncbi.nlm.nih.gov/25741868/
- **启用状态**：`enabled` (`enabled: true`)
- **实现架构**：
  1. 完整覆盖 28 个证据代码字典（PVS1、PS1-4、PM1-6、PP1-5、BA1、BS1-4、BP1-7）；
  2. 实现 Richards 2015 Table 5 全部 14 条确定性组合判定路径（Pathogenic、Likely pathogenic、Benign、Likely benign 及 VUS）；
  3. 实现致病与良性冲突仲裁状态机（双侧满足时严格收敛为 VUS 并记录冲突明细）；
  4. 实现 ClinGen VCEP 细则注册表（`SpecificationRegistry`），支持 CFTR VCEP 典范细则精确匹配，未收录基因安全回退至 Richards 2015 基线；
  5. 与 Cycle 2 专业检索协调器深度集成，支持在评估任务中自动装配证据快照。
- **验证基准**：`tests/fixtures/acmg2015/golden_cases.json` 覆盖典型无义致病、良性多态性、同义突变及冲突 VUS，100% 通过自动化回归测试。
