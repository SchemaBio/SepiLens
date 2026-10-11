# 官方数据导入

服务不会自动下载或替换数据。先由数据管理员核对官方版本、许可证和 SHA-256，然后提供锁定 manifest；目录边界、源 URL、文件散列和派生参考文件散列均被校验。

```sh
sepilens data import -manifest manifest.json -output locked-bundle
sepilens data verify -output locked-bundle
sepilens serve -bundle-dir locked-bundle
```

实际子命令参数以 `sepilens data import -h` / `data verify -h` 为准。manifest 的每条 asset 包含 `path,url,sha256,version,license,kind,assembly,complete,retrieved_at`；kind 支持 hpo_obo、refseq_gff、clingen_tsv、reference_fasta。`path` 必须相对，不能越界或经过符号链接；URL 仅允许对应官方 HTTPS 来源。`sha256` 填真实下载文件散列，不能留占位值；retrieved_at 必须是 RFC3339 时间。

```json
{"version":"release-YYYY-MM-DD","assets":[{"path":"hp.obo","url":"https://raw.githubusercontent.com/obophenotype/human-phenotype-ontology/LOCKED_COMMIT/hp.obo","sha256":"REPLACE_WITH_REAL_64_HEX","version":"LOCKED_COMMIT","license":"CONFIRMED_LICENSE_REFERENCE","kind":"hpo_obo","complete":true,"retrieved_at":"2026-10-11T00:00:00Z"}]}
```

这是格式示例，不是可用数据锁。Import 显式下载、核对文件；已存在且正确的文件可离线导入。reference 支持 FASTA / gzip FASTA，gzip 解压后的派生文件散列保存到 manifest，启动重新校验。参考索引使用行宽与偏移随机访问，不把整个基因组装入内存。RefSeq GFF 导入编码基因、转录本、外显子及链方向；HPO OBO 包含废弃、替代、同义关系；ClinGen TSV 支持区域/基因剂量敏感性记录。

每个 assembly 只有同时声明并验证完整 RefSeq、ClinGen 和 reference 才能作为完整 CNV 覆盖。GRCh37/38 独立，不跨 assembly 推断。`complete=true` 是数据管理员的覆盖声明，不是下载器自动证明的完整性；需要在医学数据验收中确认。

当前未随发行包再分发官方完整基因组、ClinGen 或新 HPO。原有中文翻译文件许可证尚待确认。测试 bundle 是合成数据，仅检验导入、坐标、散列与边界；不得当作官方临床数据。大文件与再分发许可证必须由部署方完成确认，保留对应官方许可页面及获取记录。
