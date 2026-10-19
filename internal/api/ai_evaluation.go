package api

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"time"

	"github.com/pzweuj/sepilens/internal/agent"
	"github.com/pzweuj/sepilens/internal/annotation"
	"github.com/pzweuj/sepilens/internal/engine"
	"github.com/pzweuj/sepilens/internal/evidence"
	"github.com/pzweuj/sepilens/internal/llm/responses"
	"github.com/pzweuj/sepilens/internal/retrieval"
	"github.com/pzweuj/sepilens/internal/rules/acmg2015"
	"github.com/pzweuj/sepilens/internal/workflow"
)

func (s *Server) executeAI(ctx context.Context, t workflow.Task, in envelope) (any, *evidence.EvidenceSnapshot, error) {
	started := time.Now()
	approval, ok := s.approvals[engine.EngineID(t.EngineID)]
	if s.cfg.AIMode != "enabled" || t.Kind != "production" || !ok || approval.ContentHash != in.RulesetHash {
		return nil, nil, errors.New("AI ruleset gate unavailable")
	}
	r := in.SNV
	p, err := s.phenotypeAssessment(ctx, r)
	if err != nil {
		return nil, nil, err
	}
	r.phenotypeResult = p
	base, ok := s.evaluate(in, nil, nil).(DerivedResult)
	if !ok {
		return nil, nil, errors.New("structured local baseline required")
	}
	sources := annotationSources(r, base.Derivation)
	maxSearches := r.Options.MaxSearches
	if maxSearches == 0 {
		maxSearches = 12
	}
	budget := &retrieval.Budget{Max: maxSearches}
	ctx = retrieval.WithBudget(ctx, budget)
	prov := evidence.ProvenanceMetadata{RetrievalMode: "ai_assisted", RetrievalTimestamp: started.UTC().Format(time.RFC3339), DatasetVersions: map[string]string{"bundle": in.DatasetHash}}
	payload := EvaluationResultPayload{EvaluationID: t.JobID, EngineID: t.EngineID, ResultKind: t.Kind, ClinicalUse: "requires_review", DeliveryStatus: "delivered", ReviewRequired: true, RulesetHash: in.RulesetHash, DatasetHash: in.DatasetHash, ProfileVersion: in.ProfileVersion, PromptVersion: agent.PromptVersion, ReviewStatus: "pending", Revision: 1, LocalBaseline: base, RuleEngineResult: base}
	snapshot := &evidence.EvidenceSnapshot{SnapshotID: workflow.ID("snp-"), EvaluationID: t.JobID, TenantID: t.TenantID, CreatedAt: started.UTC()}
	snapshot.HashVersion = "v2"
	snapshot.Annotations, _ = json.Marshal(r.Annotations)
	if r.Phenotype != nil {
		snapshot.Phenotype, _ = json.Marshal(map[string]any{"input": r.Phenotype, "assessment": p})
	}
	var run agent.RunResult
	finish := func(problem error) (any, *evidence.EvidenceSnapshot, error) {
		payload.Usage = ResourceUsage{InputTokens: run.InputTokens, OutputTokens: run.OutputTokens, WebSearches: budget.Count(), DurationMS: time.Since(started).Milliseconds()}
		prov.TokensUsed = run.InputTokens + run.OutputTokens
		prov.RequestsCount = budget.Count()
		prov.DurationMS = payload.Usage.DurationMS
		if problem != nil {
			code := "AI_PROVIDER_UNAVAILABLE"
			if errors.Is(problem, agent.ErrOutputInvalid) {
				code = "AI_OUTPUT_INVALID"
			}
			if errors.Is(problem, agent.ErrBudgetExceeded) || errors.Is(problem, retrieval.ErrBudgetExceeded) {
				code = "AI_TOOL_BUDGET_EXCEEDED"
			}
			payload.FailureCode = code
			payload.DeliveryStatus = "failed"
			problem = &workflow.PartialFailure{Code: code}
		}
		snapshot.Sources = sources
		snapshot.Provenance = prov
		snapshot.Agent, _ = json.Marshal(map[string]any{"run": run, "proposal": payload.AgentProposal, "failure_code": payload.FailureCode})
		if snapshot.Items == nil {
			snapshot.Items = derivedEvidence(base.Derivation, t.EngineID)
		}
		snapshot.Items = evidence.NewSourceVerifier().EnforceEvidenceIntegrity(snapshot.Items, sources)
		var e error
		snapshot.Hash, e = evidence.SnapshotHash(*snapshot)
		if e != nil {
			return nil, nil, e
		}
		return payload, snapshot, problem
	}
	profile, err := s.profiles.Get(ctx, t.TenantID, r.Options.ProviderProfileID, in.ProfileVersion)
	if err != nil {
		return finish(agent.ErrProvider)
	}
	client, err := responses.NewClient(profile.Config)
	if err != nil {
		return finish(agent.ErrProvider)
	}
	if err = t.External(); err != nil {
		return finish(agent.ErrProvider)
	}
	// Do not forward raw request bodies or caller evidence prose to a model.
	modelAnnotations := annotationForModel(r.Annotations)
	modelContext := r.ClinicalContext
	modelContext.HPOTerms = nil
	input := map[string]any{"variant": r.Variant, "clinical_context": modelContext, "annotations": modelAnnotations, "phenotype": p, "local_assessment": base, "sources": sources}
	run, err = agent.Run(ctx, client, input, func(ctx context.Context, name string) (any, error) {
		switch name {
		case "get_annotations":
			return modelAnnotations, nil
		case "get_phenotype":
			return p, nil
		case "search_literature":
			if s.coordinator == nil {
				return []evidence.Source{}, nil
			}
			q := retrieval.Query{Assembly: r.Variant.Assembly, Chrom: r.Variant.Chrom, Pos: r.Variant.Pos, Ref: r.Variant.Ref, Alt: r.Variant.Alt, Gene: r.Variant.Gene, DiseaseID: r.ClinicalContext.DiseaseID, HGVS: r.Variant.HGVSc, VariantType: "snv", Mode: resolveRetrievalMode(r.Options)}
			_, found, e := s.coordinator.Coordinate(ctx, q)
			sources = mergeSources(sources, found)
			if e != nil {
				return nil, e
			}
			return found, nil
		}
		return nil, agent.ErrOutputInvalid
	})
	if err != nil {
		return finish(err)
	}
	facts := map[acmg2015.CriterionCode]map[string]bool{}
	verified := map[string]bool{}
	for _, src := range sources {
		verified[src.ID] = src.ValidationStatus == evidence.ValidationVerified && src.IdentifierValid && src.RecordExists && src.ObjectMatched && src.MaterialVerified
	}
	for _, c := range base.Derivation.Candidates {
		ok := len(c.SourceIDs) > 0
		for _, id := range c.SourceIDs {
			ok = ok && verified[id]
		}
		if ok {
			facts[c.Code] = c.Facts
		}
	}
	effective, audit := agent.Merge(base.Derivation, run.Assessment, sources, agent.MergeOptions{AllowPP5BP6: r.Options.AllowPP5BP6, Facts: facts})
	result := s.evaluateDerivation(r, effective, sources)
	major := base.Classification != result.Classification
	payload.RuleEngineResult = result
	payload.AgentProposal = map[string]any{"ai_assessment": run.Assessment, "reconciliation": audit, "major_override": major, "tools": run.Trace}
	snapshot.Overrides, _ = json.Marshal(audit)
	snapshot.Items = derivedEvidence(effective, t.EngineID)
	return finish(nil)
}

func annotationForModel(in *annotation.Set) *annotation.Set {
	if in == nil {
		return nil
	}
	b, _ := json.Marshal(in)
	var out annotation.Set
	_ = json.Unmarshal(b, &out)
	if out.ClinVar != nil {
		out.ClinVar.Reason = ""
		var ids []string
		for _, id := range out.ClinVar.Conditions {
			if regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*:[A-Za-z0-9_.-]+$`).MatchString(id) {
				ids = append(ids, id)
			}
		}
		out.ClinVar.Conditions = ids
	}
	if out.GnomAD != nil {
		out.GnomAD.Reason = ""
	}
	if out.AlphaMissense != nil {
		out.AlphaMissense.Reason = ""
	}
	if out.Pangolin != nil {
		out.Pangolin.Reason = ""
	}
	if out.GenCC != nil {
		out.GenCC.Reason = ""
		out.GenCC.DiseaseName = ""
		out.GenCC.Submitter = ""
	}
	return &out
}

func mergeSources(a, b []evidence.Source) []evidence.Source {
	byID := map[string]bool{}
	for _, s := range a {
		byID[s.ID] = true
	}
	for _, s := range b {
		if !byID[s.ID] {
			a = append(a, s)
			byID[s.ID] = true
		}
	}
	return a
}

func (s *Server) evaluateDerivation(r *SNVEvaluationRequest, d acmg2015.Derivation, sources []evidence.Source) DerivedResult {
	var v acmg2015.SNVVariantInput
	var c acmg2015.ClinicalContextInput
	b, _ := json.Marshal(r.Variant)
	_ = json.Unmarshal(b, &v)
	b, _ = json.Marshal(r.ClinicalContext)
	_ = json.Unmarshal(b, &c)
	if a := r.Annotations; a != nil && a.GnomAD != nil {
		if a.GnomAD.PopMaxAF != nil && *a.GnomAD.PopMaxAF > .05 {
			v.AlleleFrequency = a.GnomAD.PopMaxAF
		} else {
			v.AlleleFrequency = a.GnomAD.FAF95
		}
	}
	out := s.acmgEvaluator.EvaluateEvidence(v, c, d.Applied(), sources)
	points := pointsFor(r.Options, d)
	if combinationMethod(r.Options) == "svi_points" {
		official := acmg2015.CombinePoints(out.AppliedCriteria)
		out.Classification = official.Classification
	}
	if len(d.Conflicts) > 0 {
		out.RuleConflicts = append(out.RuleConflicts, d.Conflicts...)
	}
	if r.Annotations != nil && r.Annotations.GenCC != nil && r.Annotations.GenCC.RefutedOrDisputed() {
		out.Classification = acmg2015.ClassVUS
		out.MissingInformation = append(out.MissingInformation, "gene-disease assertion is refuted or disputed; professional resolution required")
	}
	return DerivedResult{EvaluationOutput: out, Derivation: d, CombinationMethod: combinationMethod(r.Options), Points: points, Phenotype: r.phenotypeResult}
}
