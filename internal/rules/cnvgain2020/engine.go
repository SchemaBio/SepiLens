package cnvgain2020

import (
	_ "embed"
	"github.com/pzweuj/sepilens/internal/rules/cnvrubric"
	"github.com/pzweuj/sepilens/internal/variant/cnv"
)

//go:embed rubric.json
var Rules []byte

type ClauseEvaluation = cnvrubric.Clause
type GainEvaluationInput struct {
	CNV                    cnv.AnnotatedCNV      `json:"cnv"`
	TandemOrientationKnown bool                  `json:"tandem_orientation_known"`
	CaseControlStatSig     bool                  `json:"case_control_stat_sig"`
	CaseReportsCount       int                   `json:"case_reports_count"`
	Inheritance            string                `json:"inheritance"`
	Evidence               []cnvrubric.Selection `json:"evidence"`
}
type GainEvaluationResult = cnvrubric.Result
type Engine struct{}

func NewEngine() *Engine { return &Engine{} }
func (*Engine) Evaluate(in GainEvaluationInput) GainEvaluationResult {
	return cnvrubric.Evaluate("cnv_gain_2020", cnvrubric.Parse(Rules), cnvrubric.WithAnnotation(in.CNV, in.Evidence, true), in.CNV.CoverageComplete)
}
