# SVC v4 锁定草案实验评分

仅提供研究与工程对照用途。临床 enabled=false；状态 experimental，experimental_available=true。版本为 draft-reference-ef66faff51a265fef7b5c4e6439905f3aa540c46。

向 POST /v1/variants/snv/evaluations 提交以下合成输入。认证、幂等、队列、SSE、快照、审计、加密和删除使用通用任务机制。

```json
{
  "engine_id":"svc_v4",
  "variant":{"assembly":"GRCh38","chrom":"1","pos":100,"ref":"A","alt":"G","gene":"EXAMPLE"},
  "clinical_context":{"disease_id":"MONDO:EXAMPLE","inheritance":"AR","variant_origin":"germline_confirmed","sample_type":"blood","hpo_terms":[]},
  "options":{"retrieval_mode":"off","allow_experimental":true},
  "draft_assessment":{
    "upstream_commit":"ef66faff51a265fef7b5c4e6439905f3aa540c46",
    "moi":"AR","gene_disease_validity":"STRONG",
    "population":{"faf":"0.000001","daft":"0.0001"},
    "variant_impact":{"workflow":"nonsense","prediction_outcome":"NMD_NO_RESCUE","initial_points":"6","gencc_mechanism":"ESTABLISHED","exon_relevance":"ALL","informative":[{"id":"iv-1","classification":"PATHOGENIC"}]}
  }
}
```

立即返回 202 queued；通过 /v1/jobs/{job_id}/result 读取。上述输入的实验分数为 8，分类 likely_pathogenic；这不是医学案例。结果位于 agent_proposal.experimental_result，rule_engine_result=null、inputs_verified=false、clinical_use=forbidden、settlement_eligible=false。普通请求为 409，错误分支或版本、MOI 不一致为 422；unknown 来源不评分。缺失材料保留 ND，与计分零区分。分值输出为精确十进制字符串，最多六位小数。

## 范围与限制

类型见 OpenAPI DraftAssessment、Impact、Case、CaseControl、Population。继承方式为 AD/AR/XLD/XLR/SD。每个 family_id 只允许一个 index case；LOC 需指定 locus_case_id。

支持锁定的 40 个共享 LOF/剪接分支、whole-gene duplication NA、missense amino-acid 与 splice 两条路径比较、人群 POP、临床 CLN、位点 LOC。in-frame repeat/non-repeat 有显式参数路径，但上游尚无完整参考 scorer，该部分仅作为依据锁定仓库材料的实验实现。只能选择一个变异影响族；missense 保留两条比较路径及采用路径。

精确 workflow/prediction_outcome 组合见 rules/packages/svc_v4/rules.json 的 branches。initial_points、fxn_points、spa_points 是调用方提交的已编码方法值。FAF/DAFT、诊断率、OR/CI 为严格十进制文本，拒绝区间与近似字符串。informative 分类与关系仍属未核验输入。模型提议和检索结果不替换 draft_assessment。未实现 motif、noncoding 原始决策过程。其他上游适用性与上限缺口见 [交付核对](SVC_V4_RELEASE_ASSESSMENT.md)。

## 再现与批准

upstream-lock.json 保存上游提交及文件散列；branches.json 通过 Python AST 提取参数。规则包绑定数据和实际 Go 实现，ruleset_hash 等于导出 rules.json 的 SHA256，修改代码或参数即改变散列。任务执行时检查锁定版本，历史快照不原位改写。

```powershell
python scripts/import_svc_reference.py --help
python scripts/svc_reference_vectors.py --help
go test ./internal/rules/svcv4 ./internal/api ./internal/engine -count=1
go run ./cmd/sepilens rules export
```

生成夹具需指定上游 checkout 和 pydantic，仅用于开发；部署无需 Python 或在线下载。5,245 组是移植一致性验证，含 5 个公开精确边界差异，不能用于签署临床批准。规则案例矩阵保持 pending_independent_validation，validate 命令拒绝将草案作为临床批准材料。

上游代码为 MIT；许可原文见 internal/rules/svcv4/UPSTREAM_LICENSE。医学叙述材料与数据授权分别确认，本轮不打包上游全部叙述材料。正式规范、独立黄金验证、方法模型对齐和专家批准仍待完成。
