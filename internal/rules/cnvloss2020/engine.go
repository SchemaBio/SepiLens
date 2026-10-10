package cnvloss2020

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

type LossEvaluationInput struct {
	CNV                cnv.AnnotatedCNV `json:"cnv"`
	CaseControlStatSig bool             `json:"case_control_stat_sig"`
	CaseReportsCount   int              `json:"case_reports_count"`
	Inheritance        string           `json:"inheritance"` // de_novo_confirmed, etc.
}

type LossEvaluationResult struct {
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

func (e *Engine) Evaluate(in LossEvaluationInput) LossEvaluationResult {
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

	// 2. Section 2: Overlap with established HI, Benign, or Intragenic
	sec2Score := int64(0)
	switch {
	case in.CNV.CompleteHIOverlap:
		sec2Score = 100
		clauses = append(clauses, ClauseEvaluation{
			Section: "2",
			Code:    "2A",
			Score:   100,
			Name:    "Complete overlap with an established HI region/gene",
			Reason:  fmt.Sprintf("Loss completely encompasses established HI region/gene %s (+1.00)", in.CNV.HighestHIGene),
		})
	case in.CNV.CompletelyInBenign:
		sec2Score = -100
		clauses = append(clauses, ClauseEvaluation{
			Section: "2",
			Code:    "2F",
			Score:   -100,
			Name:    "Completely contained within an established benign CNV region",
			Reason:  "Loss falls entirely within documented benign region (-1.00)",
		})
	case in.CNV.OverlapsBenign:
		sec2Score = -90
		clauses = append(clauses, ClauseEvaluation{
			Section: "2",
			Code:    "2G",
			Score:   -90,
			Name:    "Overlaps an established benign CNV with smaller size",
			Reason:  "Loss partially overlaps documented benign region (-0.90)",
		})
	case in.CNV.IsIntragenic && in.CNV.HighestHI == cnv.ScoreSufficient:
		sec2Score = 90
		clauses = append(clauses, ClauseEvaluation{
			Section: "2",
			Code:    "2D",
			Score:   90,
			Name:    "Intragenic loss of an established HI gene",
			Reason:  fmt.Sprintf("Intragenic deletion within established HI gene %s (+0.90)", in.CNV.HighestHIGene),
		})
	default:
		clauses = append(clauses, ClauseEvaluation{
			Section: "2",
			Code:    "2H",
			Score:   0,
			Name:    "Loss does not overlap established HI or benign region",
			Reason:  "No established dosage sensitive or benign overlap (0.00)",
		})
	}

	// Apply Section 2 cap: -100 .. +100
	if sec2Score > 100 {
		sec2Score = 100
	} else if sec2Score < -100 {
		sec2Score = -100
	}

	// 3. Section 3: Gene Number for Loss (Loss thresholds: 0-24, 25-34, >=35)
	sec3Score := int64(0)
	gc := in.CNV.GeneCount
	switch {
	case gc >= 35:
		sec3Score = 90
		clauses = append(clauses, ClauseEvaluation{
			Section: "3",
			Code:    "3C",
			Score:   90,
			Name:    "Loss contains >= 35 protein-coding genes",
			Reason:  fmt.Sprintf("Contains %d genes (>= 35) (+0.90)", gc),
		})
	case gc >= 25:
		sec3Score = 45
		clauses = append(clauses, ClauseEvaluation{
			Section: "3",
			Code:    "3B",
			Score:   45,
			Name:    "Loss contains 25-34 protein-coding genes",
			Reason:  fmt.Sprintf("Contains %d genes (25-34) (+0.45)", gc),
		})
	default:
		clauses = append(clauses, ClauseEvaluation{
			Section: "3",
			Code:    "3A",
			Score:   0,
			Name:    "Loss contains 0-24 protein-coding genes",
			Reason:  fmt.Sprintf("Contains %d genes (0-24) (0.00)", gc),
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
	// Section 4 cap: max 90
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

	return LossEvaluationResult{
		EngineID:             "cnv_loss_2020",
		TotalScore:           scoreStr,
		TotalScoreHundredths: totalHundredths,
		Classification:       classification,
		Clauses:              clauses,
		ReviewRequired:       true,
	}
}
