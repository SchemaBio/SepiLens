package assessment

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/pzweuj/sepilens/internal/evidence"
)

// Structured inputs describe supplied records, not newly established rule facts.
// Confirmation verifies provenance only; applicability still belongs to Evaluate.
type AIStructuredFact struct {
	Field          string              `json:"field"`
	Value          any                 `json:"value"`
	Path           string              `json:"path"`
	Status         string              `json:"status"`
	Origin         string              `json:"origin"`
	SourceIDs      []string            `json:"source_ids"`
	SourceVersions []map[string]string `json:"source_versions"`
	Confirmation   string              `json:"confirmation"`
}
type AIStructuredInputs struct {
	Version       string             `json:"version"`
	Facts         []AIStructuredFact `json:"facts"`
	OmittedFields []string           `json:"omitted_fields"`
	OmittedCount  int                `json:"omitted_count"`
}

func BuildStructuredInputFacts(r Request, sources []evidence.Source) AIStructuredInputs {
	out := AIStructuredInputs{Version: "structured-inputs-v1", Facts: []AIStructuredFact{}, OmittedFields: []string{}}
	byID := map[string]evidence.Source{}
	for _, source := range sources {
		byID[source.ID] = source
	}
	blocked := evidence.ConflictingSourceGroupIDs(sources)
	add := func(field, path string, value any, ids []string, clinicalRequired bool) {
		// Keep provider context bounded without hiding that material was omitted.
		if len(out.Facts) >= 256 {
			out.OmittedCount++
			if len(out.OmittedFields) < 32 {
				out.OmittedFields = append(out.OmittedFields, "input."+field)
			}
			return
		}
		fact := AIStructuredFact{Field: "input." + field, Path: path, Value: value, Status: "reported", Origin: "structured_input", SourceIDs: append([]string{}, ids...), SourceVersions: []map[string]string{}, Confirmation: "unconfirmed"}
		verified := len(ids) > 0
		conflict := false
		for _, id := range ids {
			source, ok := byID[id]
			version := source.DatasetVersion
			if version == "" {
				version = "unversioned"
			}
			fact.SourceVersions = append(fact.SourceVersions, map[string]string{"id": id, "version": version})
			verified = verified && ok && source.IdentifierValid && source.RecordExists && source.ObjectMatched && source.MaterialVerified && source.ValidationStatus == evidence.ValidationVerified && version != "unversioned" && !blocked[id]
			conflict = conflict || blocked[id] || ok && source.ValidationStatus == evidence.ValidationConflict
		}
		if clinicalRequired {
			verified = verified && clinical(ids, r, sources)
		}
		if value == nil || value == "" {
			fact.Value = nil
			fact.Status = "unknown"
		}
		if conflict {
			fact.Status = "conflicting"
		}
		if verified && fact.Status == "reported" {
			fact.Confirmation = "source_validated"
		}
		out.Facts = append(out.Facts, fact)
	}
	// A false non-pointer boolean may be a decoder default. Preserve it as unknown
	// here rather than inventing a recorded negative premise from an omitted field.
	positiveBool := func(value bool) any {
		if value {
			return true
		}
		return nil
	}
	jsonObject := func(value any) map[string]any {
		raw, _ := json.Marshal(value)
		var out map[string]any
		_ = json.Unmarshal(raw, &out)
		return out
	}
	fields := func(prefix, path string, object map[string]any, names string, ids []string, clinicalRequired bool) {
		for _, name := range strings.Fields(names) {
			add(prefix+"."+name, path+"/"+name, object[name], ids, clinicalRequired)
		}
	}
	// Disease validity and method prerequisites precede repetitive case/region
	// details so a bounded view does not silently discard the highest-impact inputs.
	if r.EngineID == "acmg_amp_2015" || r.EngineID == "svc_v4" {
		for index, a := range r.Package.GeneAssertions {
			ids := []string{}
			for _, source := range r.Package.Sources {
				if source.Kind == "gencc" && source.Version == a.Version {
					ids = append(ids, source.ID)
				}
			}
			sort.Strings(ids)
			fields(fmt.Sprintf("gene_assertions.%d", index), fmt.Sprintf("/evidence_package/gene_assertions/%d", index), jsonObject(a), "gene disease_id classification moi version", ids, false)
		}
	}
	switch r.EngineID {
	case "acmg_amp_2015":
		m := r.Package.Mechanism
		if m == nil {
			fields("mechanism", "/evidence_package/mechanism", nil, "transcript lof_established transcript_relevant nmd_applicable", nil, true)
		} else {
			object := jsonObject(m)
			object["lof_established"] = positiveBool(m.LOFEstablished)
			object["transcript_relevant"] = positiveBool(m.TranscriptRelevant)
			object["nmd_applicable"] = positiveBool(m.NMDApplicable)
			fields("mechanism", "/evidence_package/mechanism", object, "transcript lof_established transcript_relevant nmd_applicable", m.SourceIDs, true)
		}
	case "cnv_loss_2020", "cnv_gain_2020":
		if r.CNV != nil {
			fields("cnv", "/cnv", jsonObject(r.CNV), "orientation breakpoint_precision mosaicism", nil, false)
		}
		d := r.Package.Dosage
		if d == nil {
			fields("dosage", "/evidence_package/dosage", nil, "protein_coding_genes functional_elements complete", nil, false)
			return out
		}
		object := jsonObject(d)
		add("dosage.protein_coding_genes", "/evidence_package/dosage/protein_coding_genes", object["protein_coding_genes"], d.GeneCountSourceIDs, false)
		add("dosage.functional_elements", "/evidence_package/dosage/functional_elements", object["functional_elements"], d.FunctionalSourceIDs, false)
		add("dosage.complete", "/evidence_package/dosage/complete", positiveBool(d.Complete), d.SourceIDs, false)
		fields("dosage.coverage", "/evidence_package/dosage/coverage", jsonObject(d.Coverage), "assembly chrom start end", d.SourceIDs, false)
		if r.CNV != nil {
			// Reuse the existing annotator, including its complete-reference and
			// object-matched source requirements. Never infer overlap from a gene name.
			a := annotateCNV(r, sources)
			var overlap any
			if a.DatasetHash != "" && a.DatasetHash != "unavailable" {
				if r.EngineID == "cnv_loss_2020" {
					overlap = a.CompleteHIOverlap
				} else {
					overlap = a.CompleteTSOverlap
				}
			}
			add("dosage.complete_overlap", "/rule_engine_input/dosage/complete_overlap", overlap, d.SourceIDs, false)
		}
		for index, region := range d.Regions {
			prefix := fmt.Sprintf("dosage.regions.%d", index)
			path := fmt.Sprintf("/evidence_package/dosage/regions/%d", index)
			names := "assembly chrom start end name version"
			if r.EngineID == "cnv_loss_2020" {
				names += " hi_score"
			} else {
				names += " ts_score"
			}
			fields(prefix, path, jsonObject(region), names, d.SourceIDs, false)
		}
	case "svc_v4":
		d := r.Package.Draft
		object := jsonObject(d)
		fields("svc", "/evidence_package/draft_assessment", object, "upstream_commit moi gene_disease_validity", r.Package.DraftSourceIDs, true)
		if d != nil {
			fields("svc.population", "/evidence_package/draft_assessment/population", jsonObject(d.Population), "faf daft hmz_eligible homozygote_count hemizygote_count", r.Package.DraftSourceIDs, true)
			var impact func(string, string, map[string]any, int)
			impact = func(prefix, path string, value map[string]any, depth int) {
				fields(prefix, path, value, "workflow prediction_outcome initial_points gencc_mechanism exon_relevance fxn_points spa_points", r.Package.DraftSourceIDs, true)
				if informative, ok := value["informative"].([]any); ok {
					for index, item := range informative {
						if record, ok := item.(map[string]any); ok {
							fields(fmt.Sprintf("%s.informative.%d", prefix, index), fmt.Sprintf("%s/informative/%d", path, index), record, "classification category", r.Package.DraftSourceIDs, true)
						}
					}
				}
				if depth < 2 {
					for _, child := range []string{"amino_acid", "splice"} {
						if nested, ok := value[child].(map[string]any); ok {
							impact(prefix+"."+child, path+"/"+child, nested, depth+1)
						}
					}
				}
			}
			impact("svc.variant_impact", "/evidence_package/draft_assessment/variant_impact", jsonObject(d.Impact), 0)
			if d.CaseControl != nil {
				cc := jsonObject(d.CaseControl)
				cc["controls_matched"] = positiveBool(d.CaseControl.ControlsMatched)
				cc["ascertainment_bias_considered"] = positiveBool(d.CaseControl.BiasConsidered)
				if d.CaseControl.VariantCount == 0 {
					cc["case_variant_count"] = nil
				}
				if d.CaseControl.CohortSize == 0 {
					cc["case_cohort_size"] = nil
				}
				fields("svc.case_control", "/evidence_package/draft_assessment/case_control", cc, "odds_ratio ci_lower ci_upper case_variant_count case_cohort_size controls_matched ascertainment_bias_considered", r.Package.DraftSourceIDs, true)
			}
			for index, c := range d.Cases {
				prefix := fmt.Sprintf("svc.cases.%d", index)
				path := fmt.Sprintf("/evidence_package/draft_assessment/cases/%d", index)
				// Patient/family identifiers are not needed to describe method inputs.
				fields(prefix, path, jsonObject(c), "sex pheno_specificity_for_mde pheno_severity age_matched_penetrance confirmed_parental_relationship vbc_zygosity", r.Package.DraftSourceIDs, true)
				fields(prefix+".testing", path+"/testing", jsonObject(c.Testing), "diagnostic_yield_for_phenotypes covers_all_genes_relevant_to_mde non_genetic_etiology_excluded", r.Package.DraftSourceIDs, true)
				fields(prefix+".compound_het_variant", path+"/compound_het_variant", jsonObject(c.CompoundHet), "classification phase_in_ref_to_vbc phase_confidence co_occurrence_likelihood", r.Package.DraftSourceIDs, true)
				for relativeIndex, relative := range c.Relatives {
					fields(fmt.Sprintf("%s.relatives.%d", prefix, relativeIndex), fmt.Sprintf("%s/relatives/%d", path, relativeIndex), jsonObject(relative), "parent_of_proband affected_w_mde vbc_exists vbc_zygosity cmp_het_variant_exists severe_phenotype", r.Package.DraftSourceIDs, true)
				}
				for otherIndex, other := range c.Additional {
					fields(fmt.Sprintf("%s.additional_variants.%d", prefix, otherIndex), fmt.Sprintf("%s/additional_variants/%d", path, otherIndex), jsonObject(other), "classification phase_in_ref_to_vbc phase_confidence co_occurrence_likelihood", r.Package.DraftSourceIDs, true)
				}
			}
		}
	default:
		return out
	}
	return out
}
