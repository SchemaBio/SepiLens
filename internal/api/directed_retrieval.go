package api

import (
	"context"
	"fmt"

	"github.com/pzweuj/sepilens/internal/assessment"
	"github.com/pzweuj/sepilens/internal/evidence"
	"github.com/pzweuj/sepilens/internal/retrieval"
)

type retrievalNeed struct {
	Purpose        string   `json:"purpose"`
	Codes          []string `json:"codes"`
	Supported      bool     `json:"baseline_supported"`
	Databases      []string `json:"databases"`
	SupportedCodes []string `json:"supported_codes"`
}
type retrievalFinding struct {
	Purpose  string   `json:"purpose"`
	Codes    []string `json:"codes"`
	Status   string   `json:"status"`
	Reason   string   `json:"reason"`
	Records  int      `json:"record_count"`
	Requests int      `json:"http_requests"`
}

func missingEvidencePlan(engine string, base assessment.Result) []retrievalNeed {
	met := func(codes ...string) bool {
		for _, c := range base.Criteria {
			if base.EngineID == engine && c.Status == "met" && len(c.SourceIDs) > 0 && assessment.ValidCode(engine, c.Code) {
				for _, code := range codes {
					if c.Code == code {
						return true
					}
				}
			}
		}
		return false
	}
	need := func(purpose string, databases, codes []string) retrievalNeed {
		supported := []string{}
		for _, code := range codes {
			if met(code) {
				supported = append(supported, code)
			}
		}
		// One supported clause does not resolve the other clauses in a question.
		return retrievalNeed{Purpose: purpose, Databases: databases, Codes: codes, Supported: len(codes) > 0 && len(supported) == len(codes), SupportedCodes: supported}
	}
	conflict := need("variant_conflict", []string{"clinvar"}, nil)
	functional := need("functional_evidence", []string{"pubmed"}, []string{"PVS1", "PS3", "BS3"})
	disease := need("gene_disease_validity", []string{"pubmed"}, nil)
	cases := need("case_segregation", []string{"pubmed"}, []string{"PS2", "PM6", "PP1", "4A", "4B", "4C", "4F", "4G", "4H"})
	var plan []retrievalNeed
	switch engine {
	case "cnv_loss_2020", "cnv_gain_2020":
		cases = need("case_segregation", []string{"pubmed"}, []string{"4A", "4B", "4C", "4F", "4G", "4H"})
		plan = []retrievalNeed{need("dosage_sensitivity", []string{"clingen_dosage", "pubmed"}, []string{"2A"}), conflict, cases}
	case "svc_v4":
		functional = need("functional_evidence", []string{"pubmed"}, []string{"NUL", "CDS", "PFD"})
		cases = need("case_segregation", []string{"pubmed"}, []string{"CLN", "LOC"})
		plan = []retrievalNeed{disease, functional, cases}
	case "acmg_amp_2015":
		plan = []retrievalNeed{conflict, functional, disease}
	default:
		return nil
	}
	hasConflict := base.EngineID == engine && base.Status == "conflicting"
	for _, c := range base.Criteria {
		hasConflict = hasConflict || base.EngineID == engine && c.Status == "conflicting"
	}
	if hasConflict {
		next := []retrievalNeed{conflict}
		for _, item := range plan {
			if item.Purpose != "variant_conflict" {
				next = append(next, item)
			}
		}
		if len(next) > 3 {
			next = next[:3]
		}
		plan = next
	}
	return plan
}

// This cache is scoped to one immutable request; it is not shared between users,
// organizations, jobs or clinical objects. All HTTP calls share the old budget.
type directedRetrieval struct {
	Query    retrieval.Query
	Needs    []retrievalNeed
	Findings []retrievalFinding
	Cursor   int
	Sources  []evidence.Source
	Fetch    func(context.Context, retrieval.Query) ([]evidence.Source, bool, error)
	Count    func() int
	Publish  func([]evidence.Source)
}

func (d *directedRetrieval) Next(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	for d.Cursor < len(d.Needs) {
		need := d.Needs[d.Cursor]
		d.Cursor++
		finding := retrievalFinding{Purpose: need.Purpose, Codes: need.Codes}
		if need.Supported {
			finding.Codes = need.SupportedCodes
			finding.Status = "supported"
			finding.Reason = "independent_baseline"
			d.Findings = append(d.Findings, finding)
			continue
		}
		query := d.Query
		query.Mode = retrieval.ModeTargeted
		query.Purpose = need.Purpose
		query.Databases = need.Databases
		query.PreserveRecords = true
		// Broaden gene-level questions without asserting variant equivalence. The
		// fixed coordinates/assembly remain in the query and audit context.
		if need.Purpose == "gene_disease_validity" || need.Purpose == "case_segregation" {
			query.HGVS = ""
		}
		before := d.Count()
		found, available, err := d.Fetch(ctx, query)
		finding.Requests = d.Count() - before
		finding.Records = len(found)
		d.Sources = evidence.MergeSourceRecords(d.Sources, found)
		if d.Publish != nil {
			d.Publish(d.Sources)
		}
		switch {
		case !available || err != nil:
			finding.Status = "unavailable"
			finding.Reason = "retrieval_unavailable"
		case len(found) == 0:
			finding.Status = "not_found"
			finding.Reason = "query_returned_no_records"
		default:
			finding.Status = "pending_review"
			finding.Reason = "clinical_applicability_unverified"
			for _, source := range found {
				if source.ValidationStatus == evidence.ValidationConflict {
					finding.Status = "conflicting"
					finding.Reason = "source_conflict_recorded"
					break
				}
			}
		}
		d.Findings = append(d.Findings, finding)
		return err
	}
	return nil
}
func (d *directedRetrieval) Unavailable() bool {
	for _, finding := range d.Findings {
		if finding.Status == "unavailable" {
			return true
		}
	}
	return false
}
func (d *directedRetrieval) View() map[string]any {
	status := "not_found"
	if d.Unavailable() {
		status = "unavailable"
	} else {
		for _, finding := range d.Findings {
			if finding.Status == "conflicting" {
				status = "conflicting"
				break
			}
			if finding.Status == "pending_review" || finding.Status == "supported" {
				status = "available"
			}
		}
	}
	return map[string]any{"version": "directed-retrieval-v1", "status": status, "needs": d.Needs, "findings": d.Findings, "remaining_questions": len(d.Needs) - d.Cursor, "sources": groupedModelSources(d.Sources)}
}

func groupedModelSources(sources []evidence.Source) []map[string]any {
	out := []map[string]any{}
	view := func(source evidence.Source) map[string]any {
		return map[string]any{"id": source.ID, "type": source.Type, "title": source.Title, "pmid": source.PMID, "doi": source.DOI, "clinvar_id": source.ClinVarID, "dataset_version": source.DatasetVersion, "record_exists": source.RecordExists, "object_matched": source.ObjectMatched, "material_verified": source.MaterialVerified, "validation_status": source.ValidationStatus, "snippet": source.Snippet}
	}
	for index, group := range evidence.GroupSourceDocuments(sources) {
		primary := view(group.Records[0])
		document := evidence.PublicDocumentKey(group)
		if document == "" {
			document = fmt.Sprintf("document_%d", index+1)
		}
		primary["document_id"] = document
		primary["duplicate_records"] = len(group.Records) - 1
		primary["case_identity"] = "not_established_by_retrieval"
		related := []map[string]any{}
		for _, source := range group.Records[1:] {
			related = append(related, view(source))
		}
		primary["related_records"] = related
		out = append(out, primary)
	}
	return out
}
