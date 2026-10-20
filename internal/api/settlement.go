package api

import (
	"context"
	"encoding/json"
	"time"

	"github.com/pzweuj/sepilens/internal/assessment"
	"github.com/pzweuj/sepilens/internal/engine"
	"github.com/pzweuj/sepilens/internal/evidence"
	"github.com/pzweuj/sepilens/internal/jobs"
)

func (s *Server) reviewSettlementEligible(ctx context.Context, j jobs.Job, r EvaluationResultPayload) bool {
	a, ok := s.approvals[engine.EngineID(j.EngineID)]
	if !ok || a.ContentHash != r.RulesetHash || j.ResultKind != "production" || r.RuleEngineResult == nil || r.FailureCode != "" {
		return false
	}
	snap, err := s.evidenceStore.GetSnapshot(ctx, j.TenantID, j.JobID)
	if err != nil || snap.LegacyUnverified {
		return false
	}
	var input assessment.Request
	if json.Unmarshal(snap.Annotations, &input) != nil || assessment.HashRequest(input) != r.InputHash || r.PolicyHash != s.policyHash() {
		return false
	}
	var effective assessment.Result
	b, _ := json.Marshal(r.RuleEngineResult)
	if json.Unmarshal(b, &effective) != nil || effective.Status != "calculated" {
		return false
	}
	verified := map[string]bool{}
	for _, s := range snap.Sources {
		verified[s.ID] = s.ValidationStatus == evidence.ValidationVerified && s.IdentifierValid && s.RecordExists && s.ObjectMatched && s.MaterialVerified
	}
	saved, err := s.savedInput(ctx, j)
	if err != nil {
		return false
	}
	current, _ := assessment.Sources(input, saved.Actor, s.cfg.SourcePolicies, time.Now().UTC())
	for _, src := range current {
		verified[src.ID] = verified[src.ID] && src.ValidationStatus == evidence.ValidationVerified
	}
	for _, it := range snap.Items {
		if it.Status != evidence.StatusMet {
			continue
		}
		if len(it.SourceIDs) == 0 {
			return false
		}
		for _, id := range it.SourceIDs {
			if !verified[id] {
				return false
			}
		}
	}
	return true
}
