# 规则发布与批准

固定引擎：`hpo_extract`、`acmg_amp_2015`、`svc_v4`、`cnv_loss_2020`、`cnv_gain_2020`。临床 enabled 默认为 false，依赖注入不能绕过。SVC v4 状态 experimental，可显式请求草案评分，本轮不可临床批准。

`sepilens rules export -out rules/packages` 导出 ACMG/Loss/Gain 实际规则、适用事实、来源和逐条验证矩阵。矩阵中的 pending 是明确的临床验收缺口。工程单元测试和当前 HPO fixture 不冒充独立黄金案例。

```sh
sepilens validate -engine cnv_loss_2020 -cases independently-reviewed.json -bundle locked-bundle -out validation.json
```

案例文件包含独立期望分类、整数分值、代码、出处、复核人员，以及 positive / negative / missing / boundary / mutual_exclusion / cap 类别；格式定义见 `internal/validation/cases.go`。期望不得从当前实现生成。任一不一致返回失败并保留报告。

启用包需要每个支持条款覆盖全部六类案例，案例通过且有来源/人员记录。批准记录经部署方受信 Ed25519 公钥验证，绑定实际规则内容、验证报告、版本和当前可执行文件散列。`serve -rules-dir PATH` 读取 `{engine_id}.approval.json` 与 `{engine_id}.validation.json`；`SEPILENS_APPROVAL_PUBLIC_KEY` 是 base64 公钥。内容或二进制改变使批准失效。签名私钥由医学批准方保管，程序不生成假批准。

目前未提供任何真实专家批准包，因此全部医学分类门禁保持关闭。HPO 可提取候选但没有医学启用签名流程；本轮不声称已获 HPO 医学启用批准。

ACMG 必须逐项提供经过专业核验的事实与来源，缺失 AF 不满足 PM2、未知机制不满足 PVS1，同义后果不自动满足 BP7；重复代码和相关来源显式处理。VCEP 不自带已发布细则，仅按 gene–disease–MOI 精确匹配外部已核验配置，内置 CFTR 类型不作为生产批准资料。

CNV Loss/Gain 保存独立 rubric，整数 100 代表 1.00。缺少完整 assembly 数据不把空注释当基因沙漠；未确认结构/方向不能满足相应适用事实。病例重复、互斥和上限在规则层校验，专业人员仍需对病例、家系和疾病一致性负责。
