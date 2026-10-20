# 0.2.0-rc1 验证记录

> 2026-10-20：当前接口为 /v2/evaluations，医学数据库及旧数据导入已移除。以下旧命令、接口或数据集绑定属于历史记录；当前契约见 [YiJian 对接](YIJIAN_INTEGRATION.md)，存储与批准迁移见 [部署说明](DEPLOYMENT.md)。SVC 草案适用性限制和医学批准要求继续有效。

验证日期：2026-10-11。基线复核为 `105948c`；该基线之后已有 `3262e17`（RC2）提交，RC2 的验收记录见 `docs/reports/rc2/`。该文件撤回 alpha 的“100% 医学准确率/官方对齐/生产启用”声明。

## 工程验证

已实际执行 Windows 全量 `go test ./...`、`go vet ./...`，以及 Linux Go 容器的全量 `go test -race ./...`。两个协议通过本地 HTTP 模拟服务器检查请求格式、Schema、流式、用量、失败和错误脱敏；这些属于协议工程测试，不是厂商真实联调。

测试包括：跨租户访问、审核角色与版本、并发幂等、请求断开后继续执行、取消竞态、未完成外部调用恢复、存储提交失败、快照篡改、加密认证与事务轮换、旧重复键迁移、旧来源降权、DNS 混合私网拒绝及重定向拒绝、检索实际请求预算、ClinVar 等位匹配/区域查询、许可白名单和引用追踪。

`TestMockCloudSubmitProgressReviewSettleDelete` 在隔离测试内使用合成批准条件，走通提交→进度→结果→证据→专业审核→结算→审计→删除。测试条件不导出为医学批准文件，正式交付仍默认禁用。

`scripts/smoke.py` 用实际 Windows 交付二进制、本地 HTTP、全新数据库执行 30 项检查，包含启动、认证、门禁、HPO 显式降级及跨度、异步执行、SSE 续传、幂等冲突、密钥不可读回、备份恢复、重启、事务轮换和租户删除。运行报告保存在 `docs/reports/`；报告仅包含合成测试和硬件/性能信息，不含运行密钥。

Windows 和 Linux amd64 实际解包后均通过上述 30 项检查。Linux 安装检查在 Ubuntu WSL2 上执行；两个实际 OCI 镜像均通过二进制散列一致性、非 root、任务/证据/审核门禁、删除和 SIGTERM 退出检查。arm64 由 Docker Desktop 仿真执行，不能替代原生 arm64 硬件验收。最终记录：[Windows 安装](reports/windows-install-smoke.json)、[Linux 安装](reports/linux-install-smoke.json)、[双架构容器](reports/container-smoke.json)。

`scripts/verify_release.py` 校验所有交付 SHA-256、安装包内二进制、可执行权限、源码归档、OpenAPI 引用、OCI 架构，以及报告与最终二进制身份。版本化迁移嵌入二进制，并随源码完整交付。依赖清单记录 27 个模块，收集到 34 份许可证/声明文本；文本已收集不代表完成法律审核。

支持构建：Windows amd64、Linux amd64、Linux arm64（CGO_ENABLED=0）。Dockerfile 为 Linux amd64/arm64 非 root 镜像；实际镜像与平台启动记录、校验和见交付 manifest 与报告。若报告中未标通过，不应仅凭构建声明认为已部署验收。

## 规则工程检查与医学门禁

ACMG 单元测试覆盖规范组合、重复代码、相关来源和各条款的材料核验；没有 AF、疾病机制或剪接材料时不会推定满足。Loss/Gain 独立保存 rubric，并对每个声明支持条款核对手工转录的官方整数范围、边界、缺失前提、未核验材料和累积上限；另验证分类分界、互斥、病例去重、跨类别上限、gene count 阈值、assembly 与结构不确定性。

`official_bounds_test.go` 的数值来自 [ClinGen Loss](https://cnvcalc.clinicalgenome.org/cnvcalc/cnv-loss) 和 [ClinGen Gain](https://cnvcalc.clinicalgenome.org/cnvcalc/cnv-gain)，是工程范围契约，不是专家复核患者黄金案例。合成数据测试覆盖导入、替代关系、参考碱基和锁定散列；不冒充完整官方数据。

`rules/packages/*/manifest.json` 的每条规则六类临床案例矩阵均为 pending_independent_validation。CLI validate 比较外部独立期望，错误返回非零；从当前代码生成期望不能作为批准材料。批准必须签名绑定规则、报告、当前 dataset hash 和当前可执行文件 hash。SVC v4 临床引擎保持禁用；RC2 提供显式 opt-in 的草案实验评分。

## 性能报告解释

性能 workload 为“合成 evidence_only、检索 off、无模型”，单机 SQLite、2 worker、客户端并发 4。报告给出硬件 CPU、逻辑核数、进程峰值驻留内存、请求数、吞吐、p50/p95 和失败数。它不预测真实临床模型延迟、检索速度或医学准确率；真实网络 workload 尚未测量。默认 polling 对轻任务吞吐有影响，报告不作额外性能承诺。

## 尚未通过的验收

- 两个协议真实厂商联调：没有提供专用厂商测试密钥，未执行、未标通过。
- 完整官方 GRCh37/38、RefSeq、ClinGen、HPO 数据包覆盖与再分发许可：导入工具可用，真实完整锁定清单和授权证据待确认。
- 独立黄金案例、全部条款六类专业核对、复杂基因内事件、专家签名批准：未取得，因此医学门禁关闭。
- 部署现场断电与长期运行、实际外部模型保留合同：未验收。
- 原有中文 HPO 翻译资源授权证据、传递依赖完整许可证审核：待确认，不能对外生产发布。

这些验收项不由 mock 或现有 fixture 替代。工程候选可以用于隔离部署验收，尚不能声明整个首版计划的所有退出条件已通过。

SVC v4 草案实验移植验证另见 SVC_V4_EXPERIMENTAL.md；不改变 RC1 临床批准状态。
