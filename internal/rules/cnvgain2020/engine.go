package cnvgain2020

import (
	"fmt"

	"github.com/pzweuj/sepilens/internal/score"
	"github.com/pzweuj/sepilens/internal/variant/cnv"
)

type ClauseEvaluation struct {
	Section string `json:"section"`
	Code    string `json:"code"`
	Score   int64  `json:"score"` // Hundredths
	Name    string `json:"name"`
	Reason  string `json:"reason"`
}

type GainEvaluationInput struct {
	CNV                    cnv.AnnotatedCNV `json:"cnv"`
	TandemOrientationKnown bool             `json:"tandem_orientation_known"`
	CaseControlStatSig     bool             `json:"case_control_stat_sig"`
	CaseReportsCount       int              `json:"case_reports_count"`
	Inheritance            string           `json:"inheritance"`
}

type GainEvaluationResult struct {
	EngineID             string             `json:"engine_id"`
	TotalScore           string             `json:"total_score"`
	TotalScoreHundredths int64              `json:"total_score_hundredths"`
	Classification       string             `json:"classification"`
	Clauses              []ClauseEvaluation `json:"clauses"`
	ReviewRequired       bool               `json:"review_required"`
}

type Engine struct{}

func NewEngine() *Engine {
	return &Engine{}
}

func (e *Engine) Evaluate(in GainEvaluationInput) GainEvaluationResult {
	var clauses []ClauseEvaluation

	// 1. Section 1: Initial Genomic Content
	sec1Score := int64(0)
	if in.CNV.ContainsFunctionalElements {
		clauses = append(clauses, ClauseEvaluation{
			Section: "1",
			Code:    "1A",
			Score:   0,
			Name:    "Contains protein-coding or other functionally important elements",
			Reason:  "Contains functional gene elements",
		})
	} else {
		sec1Score = -60
		clauses = append(clauses, ClauseEvaluation{
			Section: "1",
			Code:    "1B",
			Score:   -60,
			Name:    "Does not contain protein-coding or known functional elements",
			Reason:  "Region is in a non-coding gene desert (-0.60)",
		})
	}

	// 2. Section 2: Overlap with established Triplosensitive (TS) Region, Benign, or Intragenic
	sec2Score := int64(0)
	switch {
	case in.CNV.CompleteTSOverlap:
		sec2Score = 100
		clauses = append(clauses, ClauseEvaluation{
			Section: "2",
			Code:    "2A",
			Score:   100,
			Name:    "Complete overlap with an established TS region/gene",
			Reason:  fmt.Sprintf("Gain completely encompasses established TS region/gene %s (+1.00)", in.CNV.HighestTSGene),
		})
	case in.CNV.CompletelyInBenign:
		sec2Score = -100
		clauses = append(clauses, ClauseEvaluation{
			Section: "2",
			Code:    "2E",
			Score:   -100,
			Name:    "Completely contained within an established benign gain region",
			Reason:  "Gain falls entirely within documented benign region (-1.00)",
		})
	case in.CNV.OverlapsBenign:
		sec2Score = -90
		clauses = append(clauses, ClauseEvaluation{
			Section: "2",
			Code:    "2F",
			Score:   -90,
			Name:    "Overlaps an established benign gain region",
			Reason:  "Gain partially overlaps documented benign region (-0.90)",
		})
	case in.CNV.IsIntragenic && in.CNV.HighestHI == cnv.ScoreSufficient:
		sec2Score = 90
		clauses = append(clauses, ClauseEvaluation{
			Section: "2",
			Code:    "2C",
			Score:   90,
			Name:    "Duplication of a single established HI gene that disrupts gene structure",
			Reason:  fmt.Sprintf("Intragenic duplication within HI gene %s (+0.90)", in.CNV.HighestHIGene),
		})
	case in.CNV.IsIntragenic:
		clauses = append(clauses, ClauseEvaluation{
			Section: "2",
			Code:    "2D",
			Score:   0,
			Name:    "Duplication completely within a single gene without frame disruption",
			Reason:  "Intragenic duplication with no established structural disruption (0.00)",
		})
	case !in.TandemOrientationKnown && in.CNV.GeneCount > 0:
		clauses = append(clauses, ClauseEvaluation{
			Section: "2",
			Code:    "2G",
			Score:   0,
			Name:    "Tandem duplication orientation or insertion site unknown",
			Reason:  "Direction/orientation of extra copy unknown; retains clinical uncertainty (0.00)",
		})
	default:
		clauses = append(clauses, ClauseEvaluation{
			Section: "2",
			Code:    "2H",
			Score:   0,
			Name:    "Gain does not overlap established TS or benign region",
			Reason:  "No established triplosensitivity or benign overlap (0.00)",
		})
	}

	// Section 2 cap: -100 .. +100
	if sec2Score > 100 {
		sec2Score = 100
	} else if sec2Score < -100 {
		sec2Score = -100
	}

	// 3. Section 3: Gene Number for Gain (Gain thresholds: 0-34, 35-49, >=50)
	sec3Score := int64(0)
	gc := in.CNV.GeneCount
	switch {
	case gc >= 50:
		sec3Score = 90
		clauses = append(clauses, ClauseEvaluation{
			Section: "3",
			Code:    "3C",
			Score:   90,
			Name:    "Gain contains >= 50 protein-coding genes",
			Reason:  fmt.Sprintf("Contains %d genes (>= 50) (+0.90)", gc),
		})
	case gc >= 35:
		sec3Score = 45
		clauses = append(clauses, ClauseEvaluation{
			Section: "3",
			Code:    "3B",
			Score:   45,
			Name:    "Gain contains 35-49 protein-coding genes",
			Reason:  fmt.Sprintf("Contains %d genes (35-49) (+0.45)", gc),
		})
	default:
		clauses = append(clauses, ClauseEvaluation{
			Section: "3",
			Code:    "3A",
			Score:   0,
			Name:    "Gain contains 0-34 protein-coding genes",
			Reason:  fmt.Sprintf("Contains %d genes (0-34) (0.00)", gc),
		})
	}

	// 4. Section 4: Detailed Cases & Literature
	sec4Score := int64(0)
	if in.CaseControlStatSig {
		sec4Score += 45
		clauses = append(clauses, ClauseEvaluation{
			Section: "4",
			Code:    "4A",
			Score:   45,
			Name:    "Case-control evidence with statistically significant enrichment",
			Reason:  "Enrichment in affected cohorts (+0.45)",
		})
	}
	if in.CaseReportsCount > 0 {
		casePts := int64(in.CaseReportsCount * 10)
		if casePts > 45 {
			casePts = 45
		}
		sec4Score += casePts
		clauses = append(clauses, ClauseEvaluation{
			Section: "4",
			Code:    "4B",
			Score:   casePts,
			Name:    "Individual case reports with consistent phenotype",
			Reason:  fmt.Sprintf("%d case report(s) documented (+%.2f)", in.CaseReportsCount, float64(casePts)/100.0),
		})
	}
	if sec4Score > 90 {
		sec4Score = 90
	}

	// 5. Section 5: Inheritance
	sec5Score := int64(0)
	switch in.Inheritance {
	case "de_novo_confirmed":
		sec5Score = 45
		clauses = append(clauses, ClauseEvaluation{
			Section: "5",
			Code:    "5A",
			Score:   45,
			Name:    "Confirmed de novo",
			Reason:  "Both maternity and paternity confirmed de novo (+0.45)",
		})
	case "de_novo_unconfirmed":
		sec5Score = 15
		clauses = append(clauses, ClauseEvaluation{
			Section: "5",
			Code:    "5B",
			Score:   15,
			Name:    "Assumed de novo without confirmation",
			Reason:  "De novo reported without parental verification (+0.15)",
		})
	case "inherited_affected":
		sec5Score = 15
		clauses = append(clauses, ClauseEvaluation{
			Section: "5",
			Code:    "5C",
			Score:   15,
			Name:    "Inherited from affected parent",
			Reason:  "Consistent phenotype in affected parent (+0.15)",
		})
	case "inherited_unaffected":
		sec5Score = -30
		clauses = append(clauses, ClauseEvaluation{
			Section: "5",
			Code:    "5D",
			Score:   -30,
			Name:    "Inherited from unaffected parent",
			Reason:  "Penetrant disorder inherited from unaffected parent (-0.30)",
		})
	case "non_segregation":
		sec5Score = -45
		clauses = append(clauses, ClauseEvaluation{
			Section: "5",
			Code:    "5F",
			Score:   -45,
			Name:    "Lack of segregation",
			Reason:  "Does not segregate with disease in family (-0.45)",
		})
	}

	totalHundredths := sec1Score + sec2Score + sec3Score + sec4Score + sec5Score
	classification := score.Classify(totalHundredths)
	scoreStr := fmt.Sprintf("%.2f", float64(totalHundredths)/100.0)

	return GainEvaluationResult{
		EngineID:             "cnv_gain_2020",
		TotalScore:           scoreStr,
		TotalScoreHundredths: totalHundredths,
		Classification:       classification,
		Clauses:              clauses,
		ReviewRequired:       true,
	}
}
