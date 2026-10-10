package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

type SourceType string

const (
	SourceTypePubMed          SourceType = "pubmed"
	SourceTypeClinVar         SourceType = "clinvar"
	SourceTypeClinGenDosage   SourceType = "clingen_dosage"
	SourceTypeWebSearch       SourceType = "web_search"
	SourceTypeCuratedSnapshot SourceType = "curated_snapshot"
)

type SourceValidation string

const (
	ValidationVerified   SourceValidation = "verified"
	ValidationUnverified SourceValidation = "unverified"
	ValidationConflict   SourceValidation = "conflicting"
	ValidationRejected   SourceValidation = "rejected"
)

type Source struct {
	ID                string           `json:"id"`
	Type              SourceType       `json:"type"`
	Title             string           `json:"title"`
	URL               string           `json:"url,omitempty"`
	PMID              string           `json:"pmid,omitempty"`
	DOI               string           `json:"doi,omitempty"`
	ClinVarID         string           `json:"clinvar_id,omitempty"`
	RetrievedAt       string           `json:"retrieved_at"`
	FullTextAvailable bool             `json:"full_text_available"`
	ValidationStatus  SourceValidation `json:"validation_status"`
	CaseCount         int              `json:"case_count"`
	Snippet           string           `json:"snippet,omitempty"`
}

type EvidenceStatus string

const (
	StatusProposedMet      EvidenceStatus = "proposed_met"
	StatusMet              EvidenceStatus = "met"
	StatusNotMet           EvidenceStatus = "not_met"
	StatusNotApplicable    EvidenceStatus = "not_applicable"
	StatusInsufficientData EvidenceStatus = "insufficient_data"
	StatusConflicting      EvidenceStatus = "conflicting"
	StatusPendingReview    EvidenceStatus = "pending_review"
)

type EvidenceItem struct {
	ID                 string         `json:"id"`
	EngineID           string         `json:"engine_id"`
	GuidelineVersion   string         `json:"guideline_version"`
	RuleCode           string         `json:"rule_code"`
	Status             EvidenceStatus `json:"status"`
	SuggestedScore     string         `json:"suggested_score,omitempty"`
	ScoreApplied       *string        `json:"score_applied,omitempty"`
	Reason             string         `json:"reason"`
	SourceIDs          []string       `json:"source_ids,omitempty"`
	SourceValidation   string         `json:"source_validation"` // "verified", "unverified"
	ReviewRequired     bool           `json:"review_required"`
	RuleConflicts      []string       `json:"rule_conflicts,omitempty"`
	MissingInformation []string       `json:"missing_information,omitempty"`
}

type ProvenanceMetadata struct {
	RetrievalMode      string            `json:"retrieval_mode"` // "off", "targeted", "deep"
	RetrievalTimestamp string            `json:"retrieval_timestamp"`
	DatasetVersions    map[string]string `json:"dataset_versions,omitempty"`
	TokensUsed         int               `json:"tokens_used"`
	RequestsCount      int               `json:"requests_count"`
	DurationMS         int64             `json:"duration_ms"`
}

type EvidenceSnapshot struct {
	SnapshotID   string             `json:"snapshot_id"`
	EvaluationID string             `json:"evaluation_id"`
	TenantID     string             `json:"tenant_id"`
	Hash         string             `json:"hash"`
	Sources      []Source           `json:"sources"`
	Items        []EvidenceItem     `json:"items"`
	Provenance   ProvenanceMetadata `json:"provenance"`
	CreatedAt    time.Time          `json:"created_at"`
}

type canonicalPayload struct {
	Sources    []Source           `json:"sources"`
	Items      []EvidenceItem     `json:"items"`
	Provenance ProvenanceMetadata `json:"provenance"`
}

func ComputeSnapshotHash(sources []Source, items []EvidenceItem, prov ProvenanceMetadata) (string, error) {
	payload := canonicalPayload{
		Sources:    sources,
		Items:      items,
		Provenance: prov,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal canonical payload: %w", err)
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:]), nil
}
