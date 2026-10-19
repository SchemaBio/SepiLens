package api

import (
	"context"
	"encoding/json"

	"github.com/pzweuj/sepilens/internal/annotation"
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
	if len(snap.Annotations) > 0 {
		var set annotation.Set
		if json.Unmarshal(snap.Annotations, &set) != nil || set.Source != annotation.SourceSnapshot {
			return false
		}
	}
	verified := map[string]bool{}
	for _, s := range snap.Sources {
		verified[s.ID] = s.ValidationStatus == evidence.ValidationVerified && s.IdentifierValid && s.RecordExists && s.ObjectMatched && s.MaterialVerified
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
