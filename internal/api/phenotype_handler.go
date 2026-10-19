package api

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/pzweuj/sepilens/internal/annotation"
	"github.com/pzweuj/sepilens/internal/phenotype"
)

type PhenotypeMatchRequest struct {
	Gene        string         `json:"gene"`
	DiseaseID   string         `json:"disease_id"`
	Inheritance string         `json:"inheritance"`
	Phenotype   PhenotypeInput `json:"phenotype"`
}
type PhenotypeAssessment struct {
	Match phenotype.MatchResult `json:"match"`
	PP4   phenotype.PP4Result   `json:"pp4"`
}

func (s *Server) phenotypeAssessment(ctx context.Context, r *SNVEvaluationRequest) (*PhenotypeAssessment, error) {
	if s.phenotypeData == nil || r.Phenotype == nil {
		return nil, nil
	}
	input := phenotype.Input{Source: r.Phenotype.Source}
	for _, t := range r.Phenotype.Terms {
		input.Terms = append(input.Terms, phenotype.Observation{ID: t.ID, Polarity: t.Polarity, Onset: t.Onset})
	}
	candidates := []string{r.ClinicalContext.DiseaseID}
	pc := phenotype.PP4Context{ClinicalVerified: input.Source == "clinician_verified"}
	asOf := r.assessmentTime
	if asOf.IsZero() {
		asOf = time.Now().UTC()
	}
	if s.annotationIndex != nil {
		assertions, err := s.annotationIndex.GeneAssertions(ctx, "")
		if err != nil {
			return nil, err
		}
		licensed := false
		complete := false
		for _, a := range s.annotationIndex.Assets {
			if a.Kind == "gencc" && a.LicenseConfirmed {
				at, e := time.Parse(time.RFC3339, a.RetrievedAt)
				days := a.MaxAgeDays
				if days == 0 {
					days = 90
				}
				licensed = e == nil && !at.After(asOf) && asOf.Sub(at) <= time.Duration(days)*24*time.Hour
				complete = a.Complete
			}
		}
		genes := map[string]bool{}
		for _, a := range assertions {
			strong := a.GenCCSupports()
			eligible := strong || strings.EqualFold(a.Classification, "Moderate")
			if strings.EqualFold(a.Gene, r.Variant.Gene) && eligible {
				candidates = append(candidates, a.DiseaseID)
			}
			if a.DiseaseID != r.ClinicalContext.DiseaseID {
				continue
			}
			if strong {
				genes[strings.ToUpper(a.Gene)] = true
			}
			if strings.EqualFold(a.Gene, r.Variant.Gene) {
				pc.RefutedOrDisputed = pc.RefutedOrDisputed || a.RefutedOrDisputed()
				if strong && a.MOI == r.ClinicalContext.Inheritance {
					pc.GenCCStrong = true
					pc.MOIConsistent = true
				}
			}
		}
		pc.InputsVerified = licensed
		pc.SingleCauseConfirmed = licensed && complete && len(genes) == 1 && genes[strings.ToUpper(r.Variant.Gene)]
	}
	sort.Strings(candidates)
	match, err := s.phenotypeData.Match(input, candidates)
	if err != nil {
		return nil, err
	}
	if len(match.Missing) > 0 {
		pc.SingleCauseConfirmed = false
	}
	if !s.phenotypeData.ValidUntil.IsZero() && asOf.After(s.phenotypeData.ValidUntil) {
		match.Missing = append(match.Missing, "HPOA snapshot stale")
		pc.SingleCauseConfirmed = false
	}
	return &PhenotypeAssessment{Match: match, PP4: phenotype.PP4(match, r.ClinicalContext.DiseaseID, pc)}, nil
}
func (s *Server) handlePhenotypeMatch(w http.ResponseWriter, r *http.Request) {
	var req PhenotypeMatchRequest
	if !decode(w, r, &req) {
		return
	}
	if req.Gene == "" || req.DiseaseID == "" || req.Inheritance == "" {
		WriteError(w, r, 422, ErrCodeNeedsClinicalContext, "gene, disease_id and inheritance required", false)
		return
	}
	if code, msg := validatePhenotype(&req.Phenotype); code != "" {
		WriteError(w, r, 422, code, msg, false)
		return
	}
	if req.Phenotype.Source == "clinician_verified" {
		p := principal(r)
		if p.Role != "reviewer" && p.Role != "admin" {
			req.Phenotype.Source = "caller_supplied"
		}
	}
	if s.phenotypeData == nil {
		WriteError(w, r, 409, ErrCodeKnowledgeNotLoaded, "licensed HPOA and complete ontology required", false)
		return
	}
	result, err := s.phenotypeAssessment(r.Context(), &SNVEvaluationRequest{Variant: SNVVariant{Gene: req.Gene}, ClinicalContext: SNVContext{DiseaseID: req.DiseaseID, Inheritance: req.Inheritance}, Phenotype: &req.Phenotype})
	if err != nil {
		WriteError(w, r, 422, ErrCodeInvalidVariant, "phenotype identifiers cannot be resolved", false)
		return
	}
	writeJSON(w, 200, result)
}

// usableAnnotationSource prevents staleness or caller trust assertions from
// becoming server verified evidence.
func usableAnnotationSource(s *annotation.Set, id string) bool {
	if s == nil {
		return false
	}
	kind := strings.SplitN(id, ":", 2)[0]
	switch kind {
	case "clinvar":
		return s.ClinVar != nil && annotation.HasRecord(s.ClinVar.Status)
	case "gnomad":
		return s.GnomAD != nil && annotation.HasRecord(s.GnomAD.Status)
	case "alphamissense":
		return s.AlphaMissense != nil && annotation.HasRecord(s.AlphaMissense.Status)
	case "pangolin":
		return s.Pangolin != nil && annotation.HasRecord(s.Pangolin.Status)
	case "gencc":
		return s.GenCC != nil && annotation.HasRecord(s.GenCC.Status)
	}
	return false
}
