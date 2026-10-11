package validation

import (
	"encoding/json"
	"errors"
	"github.com/pzweuj/sepilens/internal/engine"
	"github.com/pzweuj/sepilens/internal/evidence"
	"github.com/pzweuj/sepilens/internal/knowledge"
	"github.com/pzweuj/sepilens/internal/rules/acmg2015"
	"github.com/pzweuj/sepilens/internal/rules/cnvgain2020"
	"github.com/pzweuj/sepilens/internal/rules/cnvloss2020"
	"github.com/pzweuj/sepilens/internal/rules/cnvrubric"
	"github.com/pzweuj/sepilens/internal/variant/cnv"
	"os"
	"sort"
)

type Case struct {
	ID       string   `json:"id"`
	Source   string   `json:"source"`
	Reviewer string   `json:"reviewer"`
	Kind     string   `json:"kind"`
	Coverage []string `json:"coverage"`
	SNV      *struct {
		Variant  acmg2015.SNVVariantInput      `json:"variant"`
		Context  acmg2015.ClinicalContextInput `json:"context"`
		Criteria []acmg2015.AppliedCriterion   `json:"criteria"`
		Sources  []evidence.Source             `json:"sources"`
	} `json:"snv,omitempty"`
	CNV *struct {
		Input            cnv.CNVInput          `json:"input"`
		Evidence         []cnvrubric.Selection `json:"evidence"`
		VerifiedMaterial bool                  `json:"verified_material"`
	} `json:"cnv,omitempty"`
	Expected struct {
		Classification string   `json:"classification"`
		Score          *int64   `json:"score_hundredths,omitempty"`
		Codes          []string `json:"codes"`
	} `json:"expected"`
}
type CaseResult struct {
	ID       string `json:"id"`
	Source   string `json:"source"`
	Reviewer string `json:"reviewer"`
	Kind     string `json:"kind"`
	Passed   bool   `json:"passed"`
}
type Report struct {
	EngineID    engine.EngineID     `json:"engine_id"`
	RulesetHash string              `json:"ruleset_hash"`
	DatasetHash string              `json:"dataset_hash"`
	Coverage    map[string][]string `json:"coverage"`
	Cases       []CaseResult        `json:"cases"`
}

func Run(id engine.EngineID, casesPath, bundleDir, out string) error {
	if id == engine.EngineSVCv4 {
		return errors.New("SVC draft reference parity cannot authorize clinical approval; independent clinical validation is pending")
	}
	content, codes, e := Rules(id)
	if e != nil {
		return e
	}
	b, e := os.ReadFile(casesPath)
	if e != nil {
		return e
	}
	var cases []Case
	if e = json.Unmarshal(b, &cases); e != nil {
		return e
	}
	if len(cases) == 0 {
		return errors.New("independently reviewed cases required")
	}
	report := Report{EngineID: id, RulesetHash: engine.Hash(content), DatasetHash: "unavailable", Coverage: map[string][]string{}}
	ann := cnv.NewAnnotator()
	if bundleDir != "" {
		bundle, e := knowledge.Load(bundleDir)
		if e != nil {
			return e
		}
		ann = cnv.NewSnapshotAnnotator(bundle.Regions, bundle.Complete, bundle.Hash)
		report.DatasetHash = bundle.Hash
	}
	known := map[string]bool{}
	for _, code := range codes {
		known[code] = true
	}
	seen := map[string]bool{}
	failed := false
	for _, c := range cases {
		valid := c.ID != "" && c.Source != "" && c.Reviewer != "" && !seen[c.ID]
		seen[c.ID] = true
		kindKnown := false
		for _, k := range RequiredKinds {
			kindKnown = kindKnown || k == c.Kind
		}
		valid = valid && kindKnown && len(c.Coverage) > 0
		var actual []string
		classification := ""
		var score *int64
		if id == engine.EngineACMG2015 && c.SNV != nil {
			r := acmg2015.NewEvaluator(nil).EvaluateEvidence(c.SNV.Variant, c.SNV.Context, c.SNV.Criteria, c.SNV.Sources)
			classification = r.Classification
			for _, crit := range r.AppliedCriteria {
				if crit.Status == acmg2015.CriterionStatusMet {
					actual = append(actual, string(crit.Code))
				}
			}
		} else if c.CNV != nil {
			a := ann.Annotate(c.CNV.Input)
			for i := range c.CNV.Evidence {
				c.CNV.Evidence[i].Verified = c.CNV.VerifiedMaterial
			}
			r := cnvloss2020.NewEngine().Evaluate(cnvloss2020.LossEvaluationInput{CNV: a, Evidence: c.CNV.Evidence})
			if id == engine.EngineCNVGain2020 {
				r = cnvgain2020.NewEngine().Evaluate(cnvgain2020.GainEvaluationInput{CNV: a, Evidence: c.CNV.Evidence})
			}
			classification = r.Classification
			score = &r.TotalScoreHundredths
			for _, clause := range r.Clauses {
				actual = append(actual, clause.Code)
			}
		} else {
			valid = false
		}
		sort.Strings(actual)
		expected := append([]string(nil), c.Expected.Codes...)
		sort.Strings(expected)
		valid = valid && classification == c.Expected.Classification && equal(actual, expected)
		if c.Expected.Score != nil {
			valid = valid && score != nil && *score == *c.Expected.Score
		}
		for _, code := range c.Coverage {
			valid = valid && known[code]
			report.Coverage[code] = append(report.Coverage[code], c.ID)
		}
		report.Cases = append(report.Cases, CaseResult{c.ID, c.Source, c.Reviewer, c.Kind, valid})
		failed = failed || !valid
	}
	b, e = json.MarshalIndent(report, "", "  ")
	if e != nil {
		return e
	}
	if e = os.WriteFile(out, b, 0600); e != nil {
		return e
	}
	if failed {
		return errors.New("independent case comparison failed")
	}
	return nil
}
func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
