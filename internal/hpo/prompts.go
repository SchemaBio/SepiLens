package hpo

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

var (
	reCodeBlock = regexp.MustCompile("(?s)```(?:json)?\\s*\\n?(.*?)\\n?```")
	reJSONObj   = regexp.MustCompile("(?s)\\{.*\\}")
	reJSONArray = regexp.MustCompile("(?s)\\[.*\\]")
)

const PreprocessSystemPrompt = `你是一位临床医学文本分析专家。请从病历文本中提取患者当前存在的症状、体征及临床背景。

**严格规则**：
1. **提取患者本人当前存在的阳性症状**：
   - 主诉与现病史中明确记录的所有阳性症状与体征（如"发作性认知障碍"、"头晕"、"乏力"）
   - 既往病史中的明确疾病诊断（如"脑梗死"、"肝硬化"）
   - 拟诊断或入院诊断
2. **严禁将以下内容归入阳性症状（symptoms）**：
   - 明确否定的症状（如"无头痛"、"否认发热"、"未见意识障碍"），必须归入 negated_symptoms
   - 家族史（如"父亲高血压"、"母亲糖尿病"），必须归入 family_history
3. **输出格式（严格 JSON）**：
{
  "symptoms": ["症状1", "症状2"],
  "medical_history": ["既往疾病1"],
  "diagnosis": ["诊断1"],
  "negated_symptoms": ["否定症状1"],
  "family_history": ["家族史1"]
}

严禁输出 Markdown 代码块以外的解释说明文字。`

func BuildMatchSystemPrompt(candidateTable string) string {
	return fmt.Sprintf(`# Role
你是一位医学HPO术语匹配专家。请从给定的候选术语表中选出最精确匹配患者阳性症状的HPO术语。

# 约束协议
返回严格的 JSON 数组，每个元素包含：
- "hpo_id": 字符串，格式 "HP:XXXXXXX"，必须来自于下方的候选术语表
- "confidence": 枚举值，仅允许："高"、"中"、"低"
- "remark": 格式为 "【原始症状词】→ 匹配理由"

严禁输出任何非JSON文本或Markdown解释。若无合适匹配，返回空数组 []。

# 核心规则
1. 只能从下方的候选术语表中选择，严禁编造或使用表外的 HPO ID。
2. 优先选择最特异、最具体的子级术语，避免选择宽泛的父类。
3. **否定语义识别**：输入若包含否定词（如"非"、"无"、"未"、"不"、"否认"、"未见"），表示该特征不存在，**严禁匹配被否定的术语**（例如"急性非ST段抬高型心肌梗死"严禁匹配"ST段抬高"；"无发热"严禁匹配"发热"）。
4. 家族史疾病严禁作为患者本人的表型匹配。
5. 置信度标准：
   - "高"：症状与术语名称或定义精确对应。
   - "中"：症状与术语大体一致，但范围偏大或需结合诊断推断。
   - "低"：仅部分相关或为宽泛分类。

# 候选HPO术语表（HP:ID|英文名|中文名|中文定义）
---
%s
---`, candidateTable)
}

func FormatCandidateTable(candidates []CandidateTerm) string {
	lines := make([]string, 0, len(candidates))
	for _, c := range candidates {
		def := c.Term.DefinitionCN
		if def == "" || def == "-" {
			def = c.Term.Definition
		}
		def = strings.ReplaceAll(def, "\n", " ")
		lines = append(lines, fmt.Sprintf("%s|%s|%s|%s", c.Term.ID, c.Term.Name, c.Term.NameCN, def))
	}
	return strings.Join(lines, "\n")
}

func ParsePreprocessJSON(raw string) (PreprocessResult, error) {
	clean := strings.TrimSpace(raw)
	if match := reCodeBlock.FindStringSubmatch(clean); len(match) > 1 {
		clean = strings.TrimSpace(match[1])
	} else if match := reJSONObj.FindString(clean); match != "" {
		clean = match
	}

	type rawPreprocess struct {
		Symptoms         []string `json:"symptoms"`
		MedicalHistory   []string `json:"medical_history"`
		MedicalHistoryAlt []string `json:"medicalHistory"`
		Diagnosis        []string `json:"diagnosis"`
		NegatedSymptoms  []string `json:"negated_symptoms"`
		NegatedSymptomsAlt []string `json:"negatedSymptoms"`
		FamilyHistory    []string `json:"family_history"`
		FamilyHistoryAlt []string `json:"familyHistory"`
		InferredSymptoms []string `json:"inferred_symptoms"`
	}

	var parsed rawPreprocess
	if err := json.Unmarshal([]byte(clean), &parsed); err != nil {
		return PreprocessResult{}, fmt.Errorf("parse preprocess json: %w", err)
	}

	medHist := parsed.MedicalHistory
	if len(medHist) == 0 {
		medHist = parsed.MedicalHistoryAlt
	}
	negated := parsed.NegatedSymptoms
	if len(negated) == 0 {
		negated = parsed.NegatedSymptomsAlt
	}
	famHist := parsed.FamilyHistory
	if len(famHist) == 0 {
		famHist = parsed.FamilyHistoryAlt
	}

	return PreprocessResult{
		Symptoms:         deduplicateStrings(parsed.Symptoms),
		MedicalHistory:   deduplicateStrings(medHist),
		Diagnosis:        deduplicateStrings(parsed.Diagnosis),
		NegatedSymptoms:  deduplicateStrings(negated),
		FamilyHistory:    deduplicateStrings(famHist),
		InferredSymptoms: deduplicateStrings(parsed.InferredSymptoms),
		RawExtraction:    raw,
	}, nil
}

type RawMatchItem struct {
	HPOID      string `json:"hpo_id"`
	Confidence string `json:"confidence"`
	Remark     string `json:"remark"`
}

func ParseMatchResponse(raw string, idx *Index) ([]ExtractionItem, error) {
	clean := strings.TrimSpace(raw)
	if match := reCodeBlock.FindStringSubmatch(clean); len(match) > 1 {
		clean = strings.TrimSpace(match[1])
	} else if match := reJSONArray.FindString(clean); match != "" {
		clean = match
	}

	var items []RawMatchItem
	if err := json.Unmarshal([]byte(clean), &items); err != nil {
		return nil, fmt.Errorf("parse match response json: %w", err)
	}

	var results []ExtractionItem
	seen := make(map[string]bool)

	for _, item := range items {
		hpoID := strings.ToUpper(strings.TrimSpace(item.HPOID))
		if hpoID == "" || seen[hpoID] {
			continue
		}

		term, exists := idx.GetTerm(hpoID)
		if !exists {
			// Strict safety gate: drop any term not found in official ontology
			continue
		}
		seen[hpoID] = true

		conf := ConfidenceMedium
		switch item.Confidence {
		case "高", "high", "High":
			conf = ConfidenceHigh
		case "中", "medium", "Medium":
			conf = ConfidenceMedium
		case "低", "low", "Low":
			conf = ConfidenceLow
		}

		origSymptom := ""
		if strings.HasPrefix(item.Remark, "【") && strings.Contains(item.Remark, "】") {
			parts := strings.SplitN(item.Remark[len("【"):], "】", 2)
			origSymptom = parts[0]
		}

		results = append(results, ExtractionItem{
			HPO:             term.ID,
			Name:            term.Name,
			NameCN:          term.NameCN,
			Definition:      term.Definition,
			DefinitionCN:    term.DefinitionCN,
			Confidence:      conf,
			Remark:          item.Remark,
			OriginalSymptom: origSymptom,
			Polarity:        PolarityPresent,
			ReviewRequired:  true,
		})
	}

	return results, nil
}

func ExtractQueryKeywords(prep PreprocessResult) []string {
	var combined []string
	combined = append(combined, prep.Symptoms...)
	combined = append(combined, prep.MedicalHistory...)
	combined = append(combined, prep.Diagnosis...)

	var expanded []string
	for _, item := range combined {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		expanded = append(expanded, item)

		// Split on typical conjunctions and punctuation
		parts := rePunct.Split(item, -1)
		for _, p := range parts {
			if len([]rune(p)) >= 2 && p != item {
				expanded = append(expanded, p)
			}
		}
	}
	return deduplicateStrings(expanded)
}
