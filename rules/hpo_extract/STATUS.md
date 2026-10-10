# HPO 术语提取规则状态

- **引擎标识**：`hpo_extract`
- **上游来源**：DeepHPO（https://github.com/pzweuj/DeepHPO，经唯一作者授权移植重构）
- **本地本体**：`knowledge/hpo/hpo_terms_cn.json`（20,384 条术语）+ `knowledge/hpo/hpo_names.txt`（40,347 行词表）
- **启用状态**：`enabled`（知识库文件就绪时系统启动自动开启）
- **实现架构**：
  1. 第一阶段：大模型预处理提取患者阳性症状、否定症状及家族史；
  2. 第二阶段：Go 高效倒排索引与基于字符 Trigram + Jaccard 相似度的同义词扩展器召回 Top 50 候选；
  3. 第三阶段：大模型在受限候选表中进行严格分类匹配与置信度打标；
  4. 最终校验：严格过滤任何不在本地词表中的虚构 HPO ID，严格阻断被否定的症状。
- **验证基准**：`tests/fixtures/hpo/golden_cases.json` 覆盖心血管、神经系统、否定语义及发育异常典型用例，100% 通过自动化回归测试。
