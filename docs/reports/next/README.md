# 本轮工程验证记录

日期：2026-10-19。全部使用合成数据，不构成医学验证或许可批准。

- Windows：全量 go test、go vet、构建通过；smoke 36 项通过。
- Ubuntu / Linux：全量 go test -race、go vet、构建通过；smoke 36 项通过。
- OpenAPI：23 个操作、69 个 schema；连续重建输出字节一致，git diff --check 通过。
- 百万行索引：300 次采样，预热 p95 0.960ms，小于 50ms；导入 22.314 秒，索引 199831552 字节，采样 Go heap 峰值 4485592 字节。cold_query_ms=0 表示本次查询低于计时分辨率，不能解释为零开销。
- 合成表型校准：2 个病例，top1=1.0；同一输入和种子重复执行输出字节一致。这是查询术语留一的内部一致性测试，不是临床准确率。

初始模型与检索包失败已定位为 Windows 沙箱限制回环 HTTP；允许本机回环测试后通过。Windows 索引路径另修复为规范 file URI。安全策略未放宽。

## 产物

- [Windows smoke](smoke-windows.json)
- [Linux smoke](smoke-linux.json)
- [百万行索引](annotation-index-windows.json)
- [合成校准](phenotype-calibration-synthetic.json)
- [合成输入夹具](fixtures/manifest.input.json)

复现校准时先复制 fixtures 内文件到独立临时目录，执行 data import -manifest 临时目录/manifest.input.json -output 临时目录，再用 serve -bundle-dir 临时目录 启动。运行 scripts/calibrate_phenotype.py 时传入该目录中的 HPOA/GenCC 文件与服务地址，令牌放在 SEPILENS_CALIBRATION_TOKEN 环境变量。夹具采用测试专用许可声明，URL 是合成来源标记，不能下载替代数据。

真实官方全量数据导入、独立病例校准、医学黄金案例及签名批准仍须由部署与专业团队完成。gnomAD 需要预先生成明确格式的 TSV；疾病 ID 不匹配时不会按名称猜测映射。
