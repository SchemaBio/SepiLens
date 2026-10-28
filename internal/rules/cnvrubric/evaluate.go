package cnvrubric

import (
	"encoding/json"
	"fmt"
	"github.com/pzweuj/sepilens/internal/score"
	"sort"
)

type Definition struct {
	Code   string   `json:"code"`
	Min    int64    `json:"min"`
	Max    int64    `json:"max"`
	CapMin int64    `json:"cap_min"`
	CapMax int64    `json:"cap_max"`
	Group  string   `json:"group,omitempty"`
	Facts  []string `json:"facts"`
}
type Selection struct {
	Code      string          `json:"code"`
	Score     int64           `json:"score_hundredths"`
	SourceIDs []string        `json:"source_ids"`
	CaseIDs   []string        `json:"case_ids,omitempty"`
	Facts     map[string]bool `json:"facts"`
	Verified  bool            `json:"-"`
}
type Clause struct {
	Section   string   `json:"section"`
	Code      string   `json:"code"`
	Score     int64    `json:"score"`
	Name      string   `json:"name"`
	Reason    string   `json:"reason"`
	SourceIDs []string `json:"source_ids,omitempty"`
}
type Result struct {
	EngineID             string   `json:"engine_id"`
	TotalScore           string   `json:"total_score"`
	TotalScoreHundredths int64    `json:"total_score_hundredths"`
	Classification       string   `json:"classification"`
	Clauses              []Clause `json:"clauses"`
	ReviewRequired       bool     `json:"review_required"`
	MissingInformation   []string `json:"missing_information,omitempty"`
	Status               string   `json:"status"`
}

func Parse(b []byte) []Definition {
	var ds []Definition
	if json.Unmarshal(b, &ds) != nil {
		panic("invalid compiled rubric")
	}
	return ds
}
func Format(n int64) string {
	sign := ""
	if n < 0 {
		sign = "-"
		n = -n
	}
	return fmt.Sprintf("%s%d.%02d", sign, n/100, n%100)
}
func Evaluate(id string, defs []Definition, selections []Selection, complete bool) Result {
	r := Result{EngineID: id, Classification: "insufficient_data", Status: "insufficient_data", ReviewRequired: true}
	if !complete {
		r.MissingInformation = []string{"complete assembly-specific annotation required"}
		return r
	}
	dict := map[string]Definition{}
	for _, d := range defs {
		dict[d.Code] = d
	}
	sums := map[string]int64{}
	sharedTotals := map[string]int64{}
	groups := map[string]string{}
	cases := map[string]bool{}
	hi := false
	sorted := append([]Selection(nil), selections...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Code < sorted[j].Code })
	for _, s := range sorted {
		d, ok := dict[s.Code]
		valid := ok && s.Verified && len(s.SourceIDs) > 0 && s.Score >= d.Min && s.Score <= d.Max
		for _, f := range d.Facts {
			valid = valid && s.Facts[f]
		}
		if !valid {
			r.MissingInformation = append(r.MissingInformation, s.Code+": verified applicable material required")
			continue
		}
		if d.Group != "" && groups[d.Group] != "" && groups[d.Group] != s.Code {
			r.Status = "conflicting"
			r.MissingInformation = append(r.MissingInformation, "mutually exclusive clauses: "+d.Group)
			return r
		}
		groups[d.Group] = s.Code
		if s.Code == "2A" {
			hi = true
		}
		if s.Code[0] == '4' && id == "cnv_loss_2020" && hi {
			r.MissingInformation = append(r.MissingInformation, "section 4 skipped after established HI overlap")
			continue
		}
		duplicate := false
		within := map[string]bool{}
		for _, c := range s.CaseIDs {
			if c == "" || cases[c] || within[c] {
				duplicate = true
			}
			within[c] = true
		}
		if duplicate {
			r.Status = "conflicting"
			r.MissingInformation = append(r.MissingInformation, "duplicate or unresolved case identity")
			return r
		}
		if s.Code[0] == '4' && len(s.CaseIDs) == 0 {
			r.MissingInformation = append(r.MissingInformation, s.Code+": case identity required")
			continue
		}
		for _, c := range s.CaseIDs {
			cases[c] = true
		}
		next := sums[s.Code] + s.Score
		if next > d.CapMax {
			next = d.CapMax
		}
		if next < d.CapMin {
			next = d.CapMin
		}
		applied := next - sums[s.Code]
		shared := ""
		capMin, capMax := int64(0), int64(0)
		switch s.Code {
		case "4A", "4B", "4C", "4D":
			shared = "de_novo"
			capMin = -30
			capMax = 90
		case "4F", "4G", "4H":
			shared = "segregation"
			capMax = 45
		case "4L", "4M":
			shared = "case_control_enrichment"
			capMax = 45
		}
		if shared != "" {
			total := sharedTotals[shared] + applied
			if total > capMax {
				total = capMax
			}
			if total < capMin {
				total = capMin
			}
			applied = total - sharedTotals[shared]
			sharedTotals[shared] = total
			next = sums[s.Code] + applied
		}
		sums[s.Code] = next
		r.Clauses = append(r.Clauses, Clause{Section: s.Code[:1], Code: s.Code, Score: applied, Name: s.Code, Reason: "verified structured applicability", SourceIDs: s.SourceIDs})
	}
	if len(r.Clauses) == 0 {
		r.MissingInformation = append(r.MissingInformation, "no verified rubric evidence")
		return r
	}
	for _, n := range sums {
		r.TotalScoreHundredths += n
	}
	r.TotalScore = Format(r.TotalScoreHundredths)
	r.Classification = score.Classify(r.TotalScoreHundredths)
	r.Status = "calculated"
	return r
}
