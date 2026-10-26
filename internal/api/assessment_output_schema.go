package api

import (
	"github.com/pzweuj/sepilens/internal/assessment"
	"github.com/pzweuj/sepilens/internal/rules/acmg2015"
	"github.com/pzweuj/sepilens/internal/rules/cnvgain2020"
	"github.com/pzweuj/sepilens/internal/rules/cnvloss2020"
	"github.com/pzweuj/sepilens/internal/rules/cnvrubric"
	"sort"
	"strings"
)

func assessmentSchemaFor(r assessment.Request) map[string]any {
	s := assessmentSchema()
	props := s["properties"].(map[string]any)
	props["classification"] = map[string]any{"type": "string", "enum": strings.Split("|Pathogenic|Likely_Pathogenic|VUS|Likely_Benign|Benign|pathogenic|likely_pathogenic|uncertain_significance|likely_benign|benign|insufficient_data|VUS-low|VUS-mid|VUS-high", "|")}
	criteria := props["criteria"].(map[string]any)
	item := criteria["items"].(map[string]any)
	fields := item["properties"].(map[string]any)
	codes := []string{}
	switch r.EngineID {
	case "svc_v4":
		if assessment.InferClinicalContext(r) {
			props["svc_context"] = svcContextSchema()
			s["required"] = append(s["required"].([]any), "svc_context")
		}
		codes = []string{"POP", "CLN", "LOC", "NUL", "CDS", "PFD"}
		fields["strength"] = map[string]any{"type": "string", "enum": []string{""}}
		fields["score_hundredths"] = map[string]any{"type": "null"}
		if r.Package.Draft == nil {
			fields["points"] = map[string]any{"type": "null"}
			fields["facts_confirmed"] = map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "maxItems": 0}
		}
	case "acmg_amp_2015":
		for code := range acmg2015.CanonicalCriteria {
			codes = append(codes, string(code))
		}
		fields["strength"] = map[string]any{"type": "string", "enum": []string{"", "Supporting", "Moderate", "Strong", "VeryStrong", "StandAlone"}}
	case "cnv_loss_2020", "cnv_gain_2020":
		if assessment.InferClinicalContext(r) {
			props["cnv_context"] = map[string]any{"type": "object", "additionalProperties": false, "required": []string{"disease_id", "inheritance", "rationale", "source_ids"}, "properties": map[string]any{"disease_id": map[string]any{"type": "string", "maxLength": 128}, "inheritance": map[string]any{"type": "string", "enum": []string{"unknown", "AD", "AR", "XL", "XLD", "XLR", "SD", "MT"}}, "rationale": map[string]any{"type": "string", "maxLength": 4000}, "source_ids": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}}}
			s["required"] = append(s["required"].([]any), "cnv_context")
		}

		rules := cnvloss2020.Rules
		if r.EngineID == "cnv_gain_2020" {
			rules = cnvgain2020.Rules
		}
		for _, d := range cnvrubric.Parse(rules) {
			codes = append(codes, d.Code)
		}
		fields["strength"] = map[string]any{"type": "string", "enum": []string{""}}
		fields["points"] = map[string]any{"type": "null"}
	}
	if len(codes) > 0 {
		sort.Strings(codes)
		fields["code"] = map[string]any{"type": "string", "enum": codes}
		criteria["maxItems"] = len(codes)
	}
	return s
}
