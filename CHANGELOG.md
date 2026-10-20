# Changelog

## 0.3.0-dev — 外部证据 API v2

- 不兼容升级：移除医学数据导入、注释索引、本地参考资源与 /v1；不迁移旧业务存储。
- YiJian JSON 证据包、API Key 提交权限、来源策略、四引擎本地同步和联网异步入口。
- 保留独立医学批准、SVC 实验门禁、受控工具与重算、失败基线、修订及审计。
- 真实厂商与医学验证仍是独立验收项。

## Unreleased

- 离线注释索引、HPOA 表型评分与 PP4 十项门禁；新增 `/v1/phenotype/match`。
- v2 加密证据快照及兼容迁移 007；来源防伪、过期门禁和 PVS1 前提修复。
- 双协议 AI 多轮工具、受控条款覆盖、服务端重算与失败基线保存；默认关闭。
- 人工修订保留有效条款和不可变历史，扩展审核结算、审计及双平台验证。

- 结构化注释与表型驱动的 ACMG 评估设计稿，见 `docs/ASSESSMENT_ARCHITECTURE.md`。
- 版本号改为单一来源：构建注入 `main.version`，由 `/v1/capabilities` 返回。

## 0.2.0-rc2

- Pinned SVC v4 experimental reference scorer with explicit opt-in; no clinical classification or settlement.
- 5,245 upstream parity vectors, including five exact-decimal POP_FRQ corrections.
- Versioned implementation-bound rule package and documented API/known gaps.

## 0.2.0-rc1 — 工程候选

- 新增服务入口、配置校验、优雅停止、版本化 SQLite 迁移及 CI。
- 五个医学引擎默认禁用；批准绑定规则、验证报告及可执行文件散列，SVC v4 不可启用。
- 新增 AES-GCM 存储、租户与角色认证、Provider profile 版本、DNS 连接保护和密钥轮换。
- 评估改为持久化异步队列，支持唯一幂等、租约恢复、取消、持续 SSE 和原子交付。
- 实现 Responses / Chat Completions 协议、结构化输出校验、流式及实际能力探测。
- 增加官方数据锁定导入、参考序列验证、HPO 候选跨度与极性、PubMed/ClinVar 和许可受限 PMC 接口。
- 分离 ACMG 适用条件与 Loss/Gain rubric，保留缺失材料，采用定点整数。
- 新增证据不可变快照、散列校验、人工证据修正与结果版本、审计导出及租户删除。

该版本不声明通过独立医学黄金验证、真实厂商联调或数据再分发许可证确认。旧 alpha 的生产验证声明撤回；迁移后旧结果及旧证据标记为历史未验证数据。
