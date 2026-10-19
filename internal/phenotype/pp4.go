package phenotype

import "math"

type PP4Context struct {
	ClinicalVerified     bool
	GenCCStrong          bool
	SingleCauseConfirmed bool
	MOIConsistent        bool
	RefutedOrDisputed    bool
	InputsVerified       bool
}
type PP4Result struct {
	Status   string          `json:"status"`
	Strength string          `json:"strength"`
	Gates    map[string]bool `json:"gates"`
	Missing  []string        `json:"missing_information,omitempty"`
}

func PP4(r MatchResult, disease string, c PP4Context) PP4Result {
	var target *DiseaseScore
	for n := range r.Scores {
		if r.Scores[n].DiseaseID == disease {
			target = &r.Scores[n]
		}
	}
	g := map[string]bool{"clinician_verified": c.ClinicalVerified, "specific_present_terms": false, "minimum_total_lr": false, "rank_first": false, "minimum_margin": false, "gencc_strong": c.GenCCStrong && c.InputsVerified, "single_cause": c.SingleCauseConfirmed && c.MOIConsistent, "no_absent_conflict": true, "no_competing_diagnosis": false, "no_refuted_assertion": !c.RefutedOrDisputed}
	if target != nil {
		g["specific_present_terms"] = target.SpecificTerms >= minSpecificTerms
		g["minimum_total_lr"] = target.LogLR >= math.Log(minTotalLR)
		g["rank_first"] = target.Rank == 1
		g["minimum_margin"] = target.Margin != nil && *target.Margin >= minMargin
		g["no_competing_diagnosis"] = target.Rank == 1 && target.Margin != nil && *target.Margin > 1
		for _, m := range target.Terms {
			if m.Conflict {
				g["no_absent_conflict"] = false
			}
		}
	}
	out := PP4Result{Status: "met", Strength: "Supporting", Gates: g}
	for _, name := range []string{"clinician_verified", "specific_present_terms", "minimum_total_lr", "rank_first", "minimum_margin", "gencc_strong", "single_cause", "no_absent_conflict", "no_competing_diagnosis", "no_refuted_assertion"} {
		if !g[name] {
			out.Missing = append(out.Missing, name)
		}
	}
	if len(out.Missing) == 0 {
		return out
	}
	out.Status = "insufficient_data"
	if c.RefutedOrDisputed || (target != nil && len(r.Scores) > 1 && !g["no_competing_diagnosis"]) {
		out.Status = "conflicting"
	} else if target != nil && (!g["no_absent_conflict"] || (!g["minimum_total_lr"] && c.ClinicalVerified)) {
		out.Status = "not_met"
	}
	return out
}
