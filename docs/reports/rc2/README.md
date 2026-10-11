# RC2 SVC 草案实验交付验证

锁定 ClinGen 参考仓库提交 ef66faff51a265fef7b5c4e6439905f3aa540c46。详见 svc-delivery.json、Windows/Linux 安装验收和双架构容器验收 JSON。临床批准保持禁用。

全量离线测试、go vet、Linux 全量 race 检查通过。安装包在独立解压目录完成 Windows/Linux 各 33 项检查；amd64/arm64 容器完成实际 SVC 精确边界、临床审核门禁与 SIGTERM 检查。arm64 使用模拟执行，未声称真实 arm64 硬件验证。报告记录实际二进制 SHA256，与发布包核对。

5,245 组上游程序对照含 5,240 个一致结果和 5 个明确的 POP_FRQ 精确十进制边界修正。程序对照不能作为独立医学黄金验证。in-frame 的手工实验路径没有上游 scorer 对照，独立临床案例、方法模型对齐和专家批准均待完成。

构建与验证：scripts/release.ps1 → scripts/images.ps1 → scripts/smoke.py / container_smoke.py → scripts/package.py → scripts/verify_release.py。默认 RC2，交付目录 dist/rc2。RC1 保留原有目录和报告。本轮没有实际厂商联调、完整官方数据或医学批准。
