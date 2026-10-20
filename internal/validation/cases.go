package validation

import (
	"encoding/json"
	"errors"
	"github.com/pzweuj/sepilens/internal/assessment"
	"github.com/pzweuj/sepilens/internal/config"
	"github.com/pzweuj/sepilens/internal/engine"
	"os"
	"sort"
	"time"
)

type Case struct {
	AssessmentTime time.Time           `json:"assessment_time"`
	ID             string              `json:"id"`
	Source         string              `json:"source"`
	Reviewer       string              `json:"reviewer"`
	Kind           string              `json:"kind"`
	Coverage       []string            `json:"coverage"`
	Actor          string              `json:"actor"`
	Request        *assessment.Request `json:"request"`
	Expected       struct {
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
	EngineID      engine.EngineID     `json:"engine_id"`
	RulesetHash   string              `json:"ruleset_hash"`
	SchemaVersion string              `json:"schema_version"`
	PolicyHash    string              `json:"source_policy_hash"`
	Coverage      map[string][]string `json:"coverage"`
	Cases         []CaseResult        `json:"cases"`
}

func Run(id engine.EngineID, casesPath, policyPath, out string) error {
	if id == engine.EngineSVCv4 {
		return errors.New("draft parity cannot authorize clinical use")
	}
	content, codes, err := Rules(id)
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(casesPath)
	if err != nil {
		return err
	}
	var cases []Case
	if json.Unmarshal(raw, &cases) != nil || len(cases) == 0 {
		return errors.New("independently reviewed cases required")
	}
	policies := map[string]config.SourcePolicy{}
	if policyPath != "" {
		b, e := os.ReadFile(policyPath)
		if e != nil {
			return e
		}
		if json.Unmarshal(b, &policies) != nil {
			return errors.New("invalid source policies")
		}
	}
	b, _ := json.Marshal(policies)
	report := Report{EngineID: id, RulesetHash: engine.Hash(content), SchemaVersion: assessment.SchemaVersion, PolicyHash: engine.Hash(b), Coverage: map[string][]string{}}
	known := map[string]bool{}
	for _, code := range codes {
		known[code] = true
	}
	seen := map[string]bool{}
	failed := false
	for _, c := range cases {
		valid := c.ID != "" && c.Source != "" && c.Reviewer != "" && !seen[c.ID] && len(c.Coverage) > 0
		seen[c.ID] = true
		kind := false
		for _, k := range RequiredKinds {
			kind = kind || k == c.Kind
		}
		valid = valid && kind
		var actual []string
		classification := ""
		var score *int64
		if c.Request == nil || c.Request.EngineID != string(id) || c.AssessmentTime.IsZero() || c.AssessmentTime.After(time.Now().UTC()) {
			valid = false
		} else {
			req, e := assessment.Normalize(*c.Request)
			if e != nil {
				valid = false
			} else {
				sources, _ := assessment.Sources(req, c.Actor, policies, c.AssessmentTime)
				result := assessment.Evaluate(req, sources)
				classification = result.Classification
				for _, v := range result.Criteria {
					if v.Status == "met" {
						actual = append(actual, v.Code)
					}
				}
				if result.Computation != nil {
					data, _ := json.Marshal(result.Computation)
					var computation struct {
						Score *int64 `json:"total_score_hundredths"`
					}
					_ = json.Unmarshal(data, &computation)
					score = computation.Score
				}
			}
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
	b, err = json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err = os.WriteFile(out, b, 0600); err != nil {
		return err
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
