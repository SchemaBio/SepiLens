# YiJian → SepiLens v2 对接契约

SepiLens 不安装、导入或查询医学注释数据库。YiJian 通过 API Key 提交本次需要的 JSON；业务任务、结果、审核和审计使用加密 SQLite 存储。输入结构以 [OpenAPI](openapi.yaml) 为准。示例位于 [examples/v2](examples/v2)，全部为合成资料，不代表数据许可或临床案例。

## 请求与返回

`POST /v2/evaluations`，`Authorization: Bearer KEY`，可带 `Idempotency-Key`。`schema_version` 固定 `sepilens-evaluation-2.0`，`mode` 必填 `local` 或 `ai_assisted`。SNV/InDel 使用 variant，CNV 使用 cnv。四个 engine_id 分别为 acmg_amp_2015、svc_v4、cnv_gain_2020、cnv_loss_2020。

本地同步返回 HTTP 200；联网返回 HTTP 202 和 job_id、assessment_id、status、is_replayed，再查询 `GET /v2/jobs/{job_id}` 和 `/result`。联网失败可保留本地基线：有部分结果时 result 返回 200，但 delivery_status=failed、failure_code 非空，任务仍 failed，不能结算。没有部分结果的失败返回 409。两种模式均先持久化输入，返回前本地计算不调用模型或网络。

结果包含 assessment_id、schema_version、input_hash、source_policy_hash、ruleset_hash、local_baseline、agent_proposal、rule_engine_result、reviewer_decision、revision、review_status、settlement_eligible。规则结果包含逐条 criteria（code/status/strength 或 score_hundredths）、缺口及计算解释；CNV 带分节整数分数。SVC 只在 agent_proposal.experimental_result 内返回实验结果，rule_engine_result=null、clinical_use=forbidden，临床启用和结算始终禁止。SVC points 为六位以内精确十进制字符串。

未知字段、重复 JSON 键和多个 JSON 值返回 400；语义或对象错配返回 422；默认请求上限 8 MiB，超限 413，可通过 SEPILENS_MAX_EVIDENCE_BYTES 调整（1 KiB–64 MiB）。变异 assembly 必须明确为 GRCh37 或 GRCh38，CNV 坐标为 1-based-inclusive；不做隐式 liftover。somatic_confirmed 不适用。缺失事实返回 insufficient_data，不推出良性。

## 外部证据与信任

每个来源记录 sources 包含 id、dataset、kind、version、retrieved_at、record_id、object、license_confirmed，以及 complete 和 clinical。object 必须与本次变异身份、疾病和 MOI 完全一致：SNV 包括 assembly/chrom/pos/ref/alt/gene，CNV 包括 assembly/chrom/start/end；均包含 disease_id/inheritance。坐标和参考版本先在 YiJian 归一化。

服务端配置 SEPILENS_PRINCIPALS，把 Key 绑定 tenant、actor、role。调用方需 submit_evidence=true；admin 也可提交。SEPILENS_SOURCE_POLICIES 按 dataset 指定允许的 versions、actors、assemblies、max_age_days（默认90）及是否允许 clinical。默认空策略不信任任何来源；API Key 证明提交者身份，不证明所有事实。自报 source、verified、任意签名不能授予信任。现有 annotation.source 字段仅为该结构的来源标签，实际信任只取服务端决策。结果来源类型为 external_evidence：这是通过服务身份与来源策略接受的上游证据声明，不表示 SepiLens 独立访问过原始数据库；真实记录、许可与临床核验材料由 YiJian／专业团队负责。

公开注释使用 annotations 和相应来源 kind：clinvar、gnomad、alphamissense、pangolin、gencc。变异后果单独来源可使用 consequence。模型不会将 ClinVar 分类直接转换成 PP5/BP6。GenCC 有效性不能代替 LoF 机制。

患者特异或专家编码材料通过 clinical=true 且被策略允许的来源提交：criteria 提供 code/status/strength（CNV 为 score_hundredths）/facts/source_ids，可附 case_ids；mechanism 提供 LoF、转录本相关性、NMD、转录本与来源。规则仍执行所需事实及强度上限。PVS1 和 PP4 不能用原始条款覆盖绕过前提。保留 family/case 身份以去重，但不把这些标识发送给模型。

可选 evidence_package.content_hash 是对 Package 结构做 Go encoding/json 规范化序列化后 SHA256，排除 content_hash 字段（实现将其置空并 omitempty）；建议客户端省略，由服务端生成输入散列。input_hash 覆盖整个规范化请求，使用相同算法并排除可选包散列，不是只对医学数据库版本取散列。

## 剂量与表型材料

剂量包 dosage 必须包括与本次对象相同的 coverage、complete、source_ids、regions。区域包含基因／区域名称、坐标、版本及 HI/TS 原始值（-1、0、1、2、3、30、40），可含 IsGene、ProteinCoding、外显子和链方向。完整覆盖需包与来源同时明确声明；未查到记录不是良性。蛋白编码基因数和功能元件判断分别携带来源，不使用 HI_Max/TS_Max 或默认 5F 代替证据。部分重叠和断点条款仍需明确临床适用材料。

表型包 phenotype 包含患者结构化 input、terms 本体子图、associations、明确 background 频率、候选疾病、版本、来源和覆盖声明。子图须包含匹配所需的完整祖先关系和可解析 ID；不必传整个数据库。背景频率由 YiJian 基于锁定全体参考计算，不能用当前候选疾病集合重新估计。基因–疾病断言放入 gene_assertions，并用 gencc 来源说明完整覆盖；无法确认竞争基因已排除时 PP4 不满足。临床确认来自 clinical_source_ids，不从字符串 clinician_verified 自动授信。阴性频率未知不计分，旧 hpo_terms 不作为已确认表型。

## LLM、审核与重放

联网另需 SEPILENS_AI_MODE=enabled 和 provider_profile_id。admin 使用 PUT /v2/providers/{profile_id} 配置 protocol/base_url/model/api_key；profile 按版本固定，GET 不返回密钥。生产医学引擎需签名批准；SVC 使用 allow_experimental 单独门禁。

工具只读固定输入、表型和当前对象的官方检索；默认12次检索、最多100次，最多12轮、100次工具调用。输入标识映射成临时引用别名，模型不接收患者原文、家系标识、记录标识和任意注释说明。模型输出须通过结构、条款、引用、事实、强度和 change 校验；六类 change 均经过裁决，无依据保留本地值，遗漏记 ai_omitted。SVC 只允许确认已有方法输出；联网材料可供专业复核，模型不得改写专家编码的草案方法值。模型自报分类不计分。

审核使用 POST /v2/evaluations/{id}/reviews，提供当前 revision、decision、reason；reviewer_id 由认证身份确定。修改证据从已保存有效条款集合合并并重算，生成新快照、增加 revision，旧批准失效。批准不能绕过规则、来源策略和许可门禁。GET /v2/jobs/{id}/audit 供 reviewer/admin 导出输入、工具调用、裁决及历史。

重放使用已保存输入和固定规则，不需要 YiJian 原数据库在线；联网审计保留原工具返回和裁决，不承诺重新调用模型得到相同内容。切换来源策略或规则需新批准，历史结果不原位改写。

## 合成响应示例

`*.response.synthetic.json` 是使用对应请求与合成来源策略，通过实际服务捕获的响应。ACMG/CNV 使用 simulation，SVC 使用 experimental；均 clinical_use=forbidden、settlement_eligible=false。随机 assessment_id、输入散列和规则散列仅描述该次工程运行，不是黄金临床期望。联网请求先返回任务标识，最终使用相同结果结构，并增加 AI 提议、裁决和用量。
