package cnvloss2020

import (
	_ "embed"
	"github.com/pzweuj/sepilens/internal/rules/cnvrubric"
	"github.com/pzweuj/sepilens/internal/variant/cnv"
)

//go:embed rubric.json
var Rules []byte

type ClauseEvaluation = cnvrubric.Clause
type LossEvaluationInput struct {
	CNV                cnv.AnnotatedCNV      `json:"cnv"`
	CaseControlStatSig bool                  `json:"case_control_stat_sig"`
	CaseReportsCount   int                   `json:"case_reports_count"`
	Inheritance        string                `json:"inheritance"`
	Evidence           []cnvrubric.Selection `json:"evidence"`
}
type LossEvaluationResult = cnvrubric.Result
type Engine struct{}

func NewEngine() *Engine { return &Engine{} }
func (*Engine) Evaluate(in LossEvaluationInput) LossEvaluationResult {
	return cnvrubric.Evaluate("cnv_loss_2020", cnvrubric.Parse(Rules), cnvrubric.WithAnnotation(in.CNV, in.Evidence, false), in.CNV.CoverageComplete)
}
