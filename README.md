# SepiLens（薛冷）

SepiLens 是独立的医学 AI 增强解读 API 服务，支持表型术语提取与胚系变异致病性解释的结构化审计流程。

## 核心引擎标识

系统固定包含五个相互独立的引擎标识：

- `hpo_extract`：临床表型 HPO 术语提取
- `acmg_amp_2015`：SNV/InDel ACMG/AMP 2015 序列变异致病性评估
- `svc_v4`：SNV/InDel SVC v4 致病性评估（当前保留接口，正式标准发布并验证前处于禁用状态）
- `cnv_loss_2020`：Constitutional CNV 缺失致病性评估（ACMG/ClinGen 2020）
- `cnv_gain_2020`：Constitutional CNV 增益致病性评估（ACMG/ClinGen 2020）

## 架构原则

1. **确定性裁决与模型提议分离**：大语言模型负责提议候选证据与理由，确定性规则引擎负责约束检验与计分，人工专业人员保留最终审核权。
2. **多引擎独立隔离**：ACMG 2015、SVC v4、CNV Loss 与 CNV Gain 拥有独立的规则集合、分值模型与审计记录。
3. **安全与隐私保障**：支持 BYOK（自带 API 密钥），支持请求脱敏，具备 SSRF 地址拦截与敏感字段日志屏蔽机制。

## 快速开始

### 依赖环境

- Go 1.26+

### 运行服务

```bash
go run ./cmd/sepilens
```

默认监听地址为 `:8080`。

### 运行测试

```bash
go test ./...
```
