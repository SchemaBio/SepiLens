# SVC v4 规则状态

- **引擎标识**：`svc_v4`
- **参考规范**：ClinGen SVC v4 Sequence Variant Classification (Draft Model: https://github.com/clingen-data-model/svcv4-model)
- **启用状态**：`disabled` (`enabled: false`)
- **就绪门禁**：
  1. ClinGen / ACMG 正式发布 SVC v4 终版指南；
  2. CSpec 方法模型及计分细则冻结并完成代码化；
  3. 通过官方验证案例比对；
  4. 经过医学与技术合规双重审批后方可启用。
- **当前策略**：接口接受该 engine_id，校验后永久返回 `409 RULESET_NOT_AVAILABLE`，禁止降级或混用 2015 规则。
