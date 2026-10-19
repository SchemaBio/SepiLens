package acmg2015

import "sort"

// PointsCombination is the Tavtigian/SVI point-based alternative to the 2015
// table. It is an optional specification: results carry their own version and
// never replace the table path unless the caller selects it.
type PointsCombination struct {
	Classification string   `json:"classification"`
	PathogenicPts  int      `json:"pathogenic_points"`
	BenignPts      int      `json:"benign_points"`
	Conflict       bool     `json:"conflict"`
	AppliedCodes   []string `json:"applied_codes"`
	SpecVersion    string   `json:"spec_version"`
}

const PointsSpecVersion = "svi-points-0.1"

func criterionPoints(c AppliedCriterion) int {
	if c.Status != CriterionStatusMet {
		return 0
	}
	if c.Direction == DirectionBenign {
		switch c.EvaluatedStrength {
		case StrengthStandAlone:
			return 0 // handled separately
		case StrengthStrong:
			return -4
		case StrengthSupporting:
			return -1
		}
		return 0
	}
	switch c.EvaluatedStrength {
	case StrengthVeryStrong:
		return 8
	case StrengthStrong:
		return 4
	case StrengthModerate:
		return 2
	case StrengthSupporting:
		return 1
	}
	return 0
}

// CombinePoints scores criteria with the SVI point framework. A stand-alone
// benign criterion forces Benign regardless of pathogenic points; both sides
// reaching a threshold yields a conflict rather than a silent choice.
func CombinePoints(criteria []AppliedCriterion) PointsCombination {
	res := PointsCombination{SpecVersion: PointsSpecVersion}
	seen := map[CriterionCode]bool{}
	standAloneBenign := false
	for _, c := range criteria {
		if c.Status != CriterionStatusMet || seen[c.Code] {
			continue
		}
		seen[c.Code] = true
		res.AppliedCodes = append(res.AppliedCodes, string(c.Code))
		if c.Direction == DirectionBenign && c.EvaluatedStrength == StrengthStandAlone {
			standAloneBenign = true
			continue
		}
		pts := criterionPoints(c)
		if pts > 0 {
			res.PathogenicPts += pts
		} else {
			res.BenignPts += -pts
		}
	}
	sort.Strings(res.AppliedCodes)
	if standAloneBenign {
		res.Classification = ClassBenign
		return res
	}
	pathogenic := 0
	switch {
	case res.PathogenicPts >= 10:
		pathogenic = 3 // Pathogenic
	case res.PathogenicPts >= 6:
		pathogenic = 2 // Likely pathogenic
	}
	benign := 0
	switch {
	case res.BenignPts >= 7:
		benign = 3 // Benign
	case res.BenignPts >= 4:
		benign = 2 // Likely benign
	}
	switch {
	case pathogenic > 0 && benign > 0:
		res.Conflict = true
		res.Classification = ClassVUS
	case pathogenic == 3:
		res.Classification = ClassPathogenic
	case pathogenic == 2:
		res.Classification = ClassLikelyPathogenic
	case benign == 3:
		res.Classification = ClassBenign
	case benign == 2:
		res.Classification = ClassLikelyBenign
	default:
		res.Classification = ClassVUS
	}
	return res
}
