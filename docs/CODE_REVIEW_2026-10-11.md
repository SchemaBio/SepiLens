# SepiLens 开发目标、状态与代码复核

复核日期：2026-10-10 至 2026-10-11。基线提交：`105948c`。本次仅复核并新增报告，未修改业务代码。优先级 P1 表示上线前必须解决，P2 表示应排入近期修复。

## 结论

项目目标明确：以 Go 模块化单体提供独立的医学 AI 解读 API，覆盖 HPO、胚系 SNV/InDel ACMG/AMP 2015、constitutional CNV Loss/Gain 2020；SVC v4 保留接口并禁用。要求模型提议、确定性结果、人工决定三层分离，具备 BYOK、可选检索、持久化任务、证据审计及云平台交付握手。

实际状态是已有较完整模块划分和可测试原型，但尚未形成可启动、可验证的生产服务。README、CHANGELOG、VALIDATION、Provider 矩阵和 rules/STATUS 中的“完整实现”“生产验证通过”明显超过当前代码与验证证据。不能据此确认已达到 M4/M5/M6 或正式首版验收。

## 能力核对

| 能力 | 实际代码状态 | 判定 |
|---|---|---|
| 服务入口与部署 | 有 HTTP Server、配置、SQLite 和 Dockerfile；缺少 `cmd/sepilens` 与 `func main`，也未见 CI 工作流 | 不能按文档启动或构建镜像 |
| HPO | 本地知识库、索引、三阶段流程、JSON/TSV/SSE 路径存在 | 原型可测试，来源语义与严格召回验证不足 |
| ACMG 2015 | 证据字典、组合器、baseline 和一个 CFTR 示例细则存在 | 组合器已有基础测试；证据适用性远未完整 |
| CNV Loss/Gain | 分离的引擎、整数计分和分类边界存在 | 注释与 rubric 均为简化实现，存在错误判分 |
| SVC v4 | 注册为 disabled，接口返回 409 | 门禁符合当前工程要求；本次未重新判断指南发布进度 |
| LLM Provider | 标识为 Responses，实际请求 Chat Completions；Stream 是空占位 | 不能按声明认定 Responses/结构化输出/流式能力已完成 |
| 专业检索 | ClinVar/PubMed 查询内存 mock map；ClinGen 查询调用方注入的 map | 未实现真实外部专业检索 |
| 审计与任务 | SQLite 存储、租户查询过滤、结果三层字段和审核记录存在 | 异步恢复、审核采纳、原子持久化与不可变快照未完成 |

## 复核发现

### R01 · P1 · 缺少运行入口，交付无法构建

位置：`README.md`、`Dockerfile:15`。两者依赖 `./cmd/sepilens`，但仓库没有该目录，也没有任何 `func main`。实测 `go build ./cmd/sepilens` 报 `directory not found`。包测试成功不能替代服务构建验收。应补充入口和依赖装配、启动关闭逻辑，并将二进制构建纳入 CI；镜像还需包含 HPO 运行资源或明确资源加载方式。

### R02 · P1 · 缺失证据被自动转为已满足的 ACMG 证据

位置：`internal/rules/acmg2015/evaluator.go:92`、`:108`、`:118`，`specification.go:33`。缺失 AF 会直接标记 PM2=met，baseline 对任意 gene/disease 默认允许 PVS1，同义后果直接标记 BP7，无剪接/保守性材料。实测未验证基因、nonsense、无 AF、无来源，仅凭 PVS1+PM2 返回 Likely pathogenic。应将缺失材料输出为 not_evaluable；PVS1 必须核验疾病机制与转录本等适用条件，BP7 也必须核验所需材料。依据：[Richards 2015 原文 Tables 3/4](https://pmc.ncbi.nlm.nih.gov/articles/PMC4544753/)。

### R03 · P1 · 同一证据代码重复计数，功能证据否定词误判

位置：`internal/rules/acmg2015/evaluator.go:135`、`:146`，`combiner.go:41`。每条包含 damaging 的来源都会追加一个 PS3，组合器按条数计强度，未按代码合并或验证独立性。实测两条 damaging 摘要得到两个 Strong 并触发 Pathogenic；单条 `no damaging effect` 同时生成 PS3 和 BS3。应使用经过核验的结构化功能结论，按规则代码归并并显式处理相关证据及冲突，避免字符串关键词直接形成 met。

### R04 · P1 · CNV 注释忽略基因组版本，并把覆盖缺口视为基因沙漠

位置：`internal/variant/cnv/annotator.go:64`、`:100`。内置仅 7 条区域记录（其中 4 个 IsGene），且均为 GRCh38；匹配时仅检查染色体和坐标，不检查 Assembly。实测相同数字坐标在 GRCh37/GRCh38 都命中 RAI1 的完整 HI 重叠。不命中这张小表就默认无功能元素，并在规则引擎按 1B 扣 -0.60；这不能表示真实全基因组注释。GeneCount 也无法达到 Loss 25/35 或 Gain 35/50 阈值。应使用按 assembly 锁定的完整基因/外显子及剂量敏感性快照，数据覆盖不足时返回 insufficient_data。

### R05 · P1 · CNV 条款与官方 rubric 不一致

位置：`internal/rules/cnvloss2020/engine.go:85`、`:94`、`:154`、`:184`；`cnvgain2020/engine.go:95`、`:171`、`:200`。Loss 对良性区域部分重叠统一扣 -0.90，而官方 2G 对包含额外基因组材料要求 0 分继续评估；实测略微扩出良性区域即返回 -0.90/Likely benign。Loss 的基因内事件应进入 2E 的 PVS1 分级，不能统一以 2D/+0.90 处理。两个引擎还把病例对照证据写成 4A、对病例数按每例 +0.10 简化计分，将 non_segregation 写成 5F/-0.45，而官方 5F 是遗传信息不可用的 0 分条款。应分别建立完整机器可读 rubric、适用性、互斥、范围和上限，并逐条构建独立对照案例。依据：[ClinGen Loss](https://cnvcalc.clinicalgenome.org/cnvcalc/cnv-loss)、[ClinGen Gain](https://cnvcalc.clinicalgenome.org/cnvcalc/cnv-gain)。

### R06 · P1 · 输入适用边界与结构不确定性没有贯穿评分

位置：`internal/api/admission.go:148`、`:164`，`cnv_handler.go:134`。CNV coordinate_system 只要求非空，后续全部按 1-based-inclusive 计算，没有枚举校验或转换；来源只拒绝 somatic_confirmed，缺失/unknown 来源仍产生 production 分类。Gain 把 TandemOrientationKnown 硬编码为 true，基因内 HI 分支也未核验结构破坏；copy_number、breakpoint_precision、mosaicism 没有进入注释和评分。SNV 入参虽然携带 origin/sample_type，但不据此拒绝体细胞来源。应校验并标准化坐标与来源，不确定结构只能形成待核验提议或 insufficient_data，不能默认已确认。

### R07 · P1 · SSRF 防护仅检查字面 IP，域名解析与连接未受保护

位置：`internal/ssrf/guard.go:79`，`internal/llm/responses/client.go:37`。ValidateEndpoint 对普通 hostname 不解析 DNS；SDK 使用默认连接路径，也没有受限 DialContext 或重定向策略。公网样式域名解析至私网仍可进入请求流程，存在内网访问及 DNS rebinding 风险。实测任意普通域名通过验证，未进行解析。该反例证明验证缺口，未发起真实内网请求。应在连接时校验解析地址、限制重定向及特殊地址，显式私网开关才允许例外。

### R08 · P1 · 证据 verified 只代表格式正确，正式结果绕过完整性约束

位置：`internal/evidence/verifier.go:31`、`:39`，`internal/api/snv_handler.go:130`、`cnv_handler.go:127`。PMID/DOI/ClinVar ID 只经过正则就成为 verified；没有验证记录存在、变异/疾病匹配或来源内容。实测 `9999999999` 未解析便获 verified。正式 SNV/CNV handler 还直接写 SourceValidation=verified，部分条目没有 SourceIDs，未调用 EnforceEvidenceIntegrity。应分开 identifier_valid 与 verified，真正核验来源、对象匹配和版本后才允许 met 与计分，并在写入前统一执行完整性检查。

### R09 · P1 · 关键持久化失败仍报告成功交付并允许结算

位置：`internal/api/snv_handler.go:148`、`:158`、`:173`，`cnv_handler.go:172`、`:182`、`:197`。SaveSnapshot、UpdateJobStatus、AppendEvent 的错误被忽略；即使证据或结果没有落库，响应仍可能是 succeeded、delivery_status=delivered、settlement_eligible=true。应确保结果、证据、完成状态以事务或可靠提交协议一起成功，否则明确 failed/未交付、不得结算；添加存储故障注入测试。

### R10 · P1 · 人工采纳规则结果路径被无条件禁用

位置：`internal/api/jobs_handler.go:350`。所有 accept_engine_result 都直接返回 409，代码没有判断是否为 simulation 或已完成 production 任务。因此真实引擎结果也无法正常采纳，阻断三层模型的最终审核流程。应核验任务状态、结果存在性与 result_kind 后允许采纳，并校验 decision/classification 枚举。

### R11 · P1 · Provider 与检索能力声明超过实现

位置：`internal/llm/responses/client.go:53`、`:81`、`:101`，`internal/api/provider_test_handler.go:44`，`internal/retrieval/retrievers.go:49`、`:75`。Generate 调用 Chat.Completions，没有提交 JSON Schema/web_search；Stream 仅返回空 done。Provider test 只检查 URL 就声明 structured_output、streaming、native_web_search 全部支持，不检查密钥、模型或实际连通性。ClinVar/PubMed Retrieve 仅返回 mockData。应准确区分协议，完成实际调用与探测，未验证能力默认 false，mock 仅保留在测试注入路径。

### R12 · P1 · 任务尚未异步化，恢复与取消语义不成立

位置：`internal/api/snv_handler.go:18`、`cnv_handler.go:21`，`internal/jobs/store.go:165`、`:239`，`internal/api/jobs_handler.go:222`。评估在 HTTP 请求 context 中同步完成后才返回 202；没有 worker 或持久化请求，仅保存请求 hash，进程重启无法重放 queued 任务。取消只改数据库状态，不停止执行；完成更新也不检查旧状态，可能覆盖 cancelled。Job SSE 只读取一次已有事件并关闭，未实现后续事件订阅或续传。应持久化可恢复的输入，异步 worker 执行，状态转换用原子条件更新，并将取消信号传递到任务执行。

### R13 · P2 · 并发幂等与证据快照不可变性缺失

位置：`internal/storage/sqlite/migrations/001_init.sql:17`，`internal/evidence/store.go:52`、`:70`。幂等键仅普通索引，先查再插无唯一约束，同键并发请求可能生成多任务/重复费用。SaveSnapshot 对相同 ID 执行 UPDATE，读取也不重算 hash，因此 SHA-256 字段不等于不可篡改审计。应增加按租户的非空幂等键唯一约束及冲突重放；证据快照采用 append-only 版本，并验证读取完整性。

### R14 · P1 · HPO 来源语义、黄金验证与启用门禁不足

位置：`internal/hpo/prompts.go:133`、`:187`，`extractor.go:338`，`tests/integration/hpo_regression_test.go:73`，`internal/engine/registry.go:60`。模型结果只核验 ID 在整个 ontology 中，不验证属于本轮候选或有原文支撑；返回条目统一 present，不能可靠保留 family_history/inferred/uncertain。HPO 回归中预期阳性未返回、甚至未召回，只执行 t.Logf，不使测试失败。黄金 fixture 数量仅 HPO 4、ACMG 4、Loss 3、Gain 2；另有组合器单元测试，但这些不构成完整医学适用性验证。ACMG/CNV 已默认 enabled，而代码和案例不足以满足 AGENTS.md 的完整黄金验证门禁。应补充严格断言、来源跨度与语义测试、官方条款案例矩阵；完成版本锁定和审核前撤回生产启用声明。

## 实际验证

- Go 版本：`go1.26.2 windows/amd64`。
- `go test ./...`：全部现有测试通过。首次执行因沙箱不允许默认模块缓存写入而未完成；经工具批准下载依赖后完整执行成功。
- `go vet ./...`：通过。
- `go build ./cmd/sepilens`：失败，运行入口目录缺失。
- 临时 Go 反例程序验证了 R02、R03、R04、R05、R07、R08；未使用真实患者数据、密钥或外部模型。程序已移除，仅保留报告。
- 本次没有真实模型联调、Docker 构建、真实专业检索、性能基准或专家临床验证；不将其写成通过。

反例输出摘要：

```text
missing_AF_unknown_gene: Likely pathogenic
repeated_PS3: Pathogenic
negated_functional_effect: 同一来源同时生成 PS3 与 BS3
assembly_GRCh37: HI=true genes=[RAI1]
assembly_GRCh38: HI=true genes=[RAI1]
benign_overlap_extra_material: -0.90 Likely benign
unresolved_identifier_status: verified
hostname_without_DNS_validation: <nil>
```

## 建议开发顺序

1. 收紧 ACMG/CNV 生产启用门禁，同步纠正文档状态；先修复缺失证据、错误计分、assembly 与来源边界。
2. 补运行入口与 CI 构建，修复 SSRF 和来源核验，完成准确的 Provider/检索能力实现。
3. 实现异步恢复、持久化事务、取消、并发幂等和生产结果审核采纳。
4. 建立按条款、机制、版本、异常输入划分的独立黄金案例，再进行模型联调、性能、安全与发布验收。

可保留的工程基础包括：固定五引擎标识、SVC v4 禁用、Loss/Gain 目录分离、CNV 整数汇总与阈值比较、租户过滤查询以及三层结果字段。它们方向正确，但不能替代以上验收条件。
