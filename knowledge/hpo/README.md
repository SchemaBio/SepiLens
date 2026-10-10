# HPO 知识库资产说明

本项目使用的 HPO 中文本体数据直接来源于 [DeepHPO](https://github.com/pzweuj/DeepHPO)，并经 DeepHPO 唯一作者授权重构集成到 SepiLens 中。

## 资产清单

- `hpo_terms_cn.json`：全量 HPO 中英双语本体主库（共 20,384 条术语），包含 `id`、`name`、`name_cn`、`definition`、`definition_cn`。
- `hpo_names.txt`：所有英文名称与中文译名去重并排序后的名称词表（共 40,347 行），供字符 Trigram 倒排索引与模糊同义词扩展。

## 词条结构示例

```json
{
  "HP:0002321": {
    "id": "HP:0002321",
    "name": "Vertigo",
    "definition": "An illusory sense that either the self or the environment is revolving.",
    "name_cn": "眩晕",
    "definition_cn": "一种自我或周围环境旋转的不适感觉。"
  }
}
```

## 维护更新

如需增量同步最新官方 `hp.obo`，参考 DeepHPO 流程工具链重新生成上述 JSON 及 TXT，替换后重启服务即可生效。
