package acmg2015

import (
	"fmt"
)

const (
	ClassPathogenic       = "Pathogenic"
	ClassLikelyPathogenic = "Likely pathogenic"
	ClassVUS              = "VUS"
	ClassLikelyBenign     = "Likely benign"
	ClassBenign           = "Benign"
)

type ClassificationResult struct {
	Classification        string   `json:"classification"`
	PathogenicPath        string   `json:"pathogenic_path,omitempty"`
	BenignPath            string   `json:"benign_path,omitempty"`
	VeryStrongCount       int      `json:"very_strong_count"`
	StrongCount           int      `json:"strong_count"`
	ModerateCount         int      `json:"moderate_count"`
	SupportingCount       int      `json:"supporting_count"`
	BenignStandAloneCount int      `json:"benign_standalone_count"`
	BenignStrongCount     int      `json:"benign_strong_count"`
	BenignSupportingCount int      `json:"benign_supporting_count"`
	Conflicts             []string `json:"conflicts,omitempty"`
	ConflictDetected      bool     `json:"conflict_detected"`
}

type Combiner struct{}

func NewCombiner() *Combiner {
	return &Combiner{}
}

func (c *Combiner) Combine(criteria []AppliedCriterion) ClassificationResult {
	res := ClassificationResult{
		Classification: ClassVUS,
	}

	for _, crit := range criteria {
		if crit.Status != CriterionStatusMet {
			continue
		}

		if crit.Direction == DirectionPathogenic {
			switch crit.EvaluatedStrength {
			case StrengthVeryStrong:
				res.VeryStrongCount++
			case StrengthStrong:
				res.StrongCount++
			case StrengthModerate:
				res.ModerateCount++
			case StrengthSupporting:
				res.SupportingCount++
			}
		} else if crit.Direction == DirectionBenign {
			switch crit.EvaluatedStrength {
			case StrengthStandAlone:
				res.BenignStandAloneCount++
			case StrengthStrong:
				res.BenignStrongCount++
			case StrengthSupporting:
				res.BenignSupportingCount++
			}
		}
	}

	// 1. Evaluate Pathogenic criteria (Richards 2015 Table 5)
	pathogenicMet := false
	pathogenicPath := ""

	vs := res.VeryStrongCount
	s := res.StrongCount
	m := res.ModerateCount
	p := res.SupportingCount

	switch {
	case vs >= 1 && s >= 1:
		pathogenicMet = true
		pathogenicPath = "Pathogenic (1a): 1 Very Strong + ≥1 Strong"
	case vs >= 1 && m >= 2:
		pathogenicMet = true
		pathogenicPath = "Pathogenic (1b): 1 Very Strong + ≥2 Moderate"
	case vs >= 1 && m == 1 && p >= 1:
		pathogenicMet = true
		pathogenicPath = "Pathogenic (1c): 1 Very Strong + 1 Moderate + 1 Supporting"
	case vs >= 1 && p >= 2:
		pathogenicMet = true
		pathogenicPath = "Pathogenic (1d): 1 Very Strong + ≥2 Supporting"
	case s >= 2:
		pathogenicMet = true
		pathogenicPath = "Pathogenic (2): ≥2 Strong"
	case s == 1 && m >= 3:
		pathogenicMet = true
		pathogenicPath = "Pathogenic (3a): 1 Strong + ≥3 Moderate"
	case s == 1 && m == 2 && p >= 2:
		pathogenicMet = true
		pathogenicPath = "Pathogenic (3b): 1 Strong + 2 Moderate + ≥2 Supporting"
	case s == 1 && m == 1 && p >= 4:
		pathogenicMet = true
		pathogenicPath = "Pathogenic (3c): 1 Strong + 1 Moderate + ≥4 Supporting"
	}

	// 2. Evaluate Likely Pathogenic criteria
	likelyPathogenicMet := false
	likelyPathogenicPath := ""

	if !pathogenicMet {
		switch {
		case vs >= 1 && m >= 1:
			likelyPathogenicMet = true
			likelyPathogenicPath = "Likely pathogenic (1): 1 Very Strong + 1 Moderate"
		case s == 1 && (m == 1 || m == 2):
			likelyPathogenicMet = true
			likelyPathogenicPath = "Likely pathogenic (2): 1 Strong + 1-2 Moderate"
		case s == 1 && p >= 2:
			likelyPathogenicMet = true
			likelyPathogenicPath = "Likely pathogenic (3): 1 Strong + ≥2 Supporting"
		case m >= 3:
			likelyPathogenicMet = true
			likelyPathogenicPath = "Likely pathogenic (4): ≥3 Moderate"
		case m == 2 && p >= 2:
			likelyPathogenicMet = true
			likelyPathogenicPath = "Likely pathogenic (5): 2 Moderate + ≥2 Supporting"
		case m == 1 && p >= 4:
			likelyPathogenicMet = true
			likelyPathogenicPath = "Likely pathogenic (6): 1 Moderate + ≥4 Supporting"
		}
	}

	// 3. Evaluate Benign criteria
	benignMet := false
	benignPath := ""

	ba := res.BenignStandAloneCount
	bs := res.BenignStrongCount
	bp := res.BenignSupportingCount

	switch {
	case ba >= 1:
		benignMet = true
		benignPath = "Benign (1): 1 Stand-alone (BA1)"
	case bs >= 2:
		benignMet = true
		benignPath = "Benign (2): ≥2 Strong (BS)"
	}

	// 4. Evaluate Likely Benign criteria
	likelyBenignMet := false
	likelyBenignPath := ""

	if !benignMet {
		switch {
		case bs >= 1 && bp >= 1:
			likelyBenignMet = true
			likelyBenignPath = "Likely benign (1): 1 Strong + 1 Supporting"
		case bp >= 2:
			likelyBenignMet = true
			likelyBenignPath = "Likely benign (2): ≥2 Supporting"
		}
	}

	res.PathogenicPath = pathogenicPath
	if likelyPathogenicPath != "" {
		res.PathogenicPath = likelyPathogenicPath
	}
	res.BenignPath = benignPath
	if likelyBenignPath != "" {
		res.BenignPath = likelyBenignPath
	}

	// 5. Conflict Resolution & Final Classification
	isPath := pathogenicMet || likelyPathogenicMet
	isBenign := benignMet || likelyBenignMet

	if isPath && isBenign {
		res.ConflictDetected = true
		res.Conflicts = append(res.Conflicts,
			fmt.Sprintf("Conflict: met pathogenic rule (%s) and benign rule (%s)", res.PathogenicPath, res.BenignPath))
		res.Classification = ClassVUS
		return res
	}

	switch {
	case pathogenicMet:
		res.Classification = ClassPathogenic
	case likelyPathogenicMet:
		res.Classification = ClassLikelyPathogenic
	case benignMet:
		res.Classification = ClassBenign
	case likelyBenignMet:
		res.Classification = ClassLikelyBenign
	default:
		res.Classification = ClassVUS
	}

	return res
}
