// Package assessment evaluates immutable, externally supplied evidence. It has
// no database, filesystem, network or provider dependency.
package assessment

import (
	"github.com/pzweuj/sepilens/internal/annotation"
	"github.com/pzweuj/sepilens/internal/hpo"
	"github.com/pzweuj/sepilens/internal/phenotype"
	"github.com/pzweuj/sepilens/internal/rules/acmg2015"
	"github.com/pzweuj/sepilens/internal/rules/svcv4"
	"github.com/pzweuj/sepilens/internal/variant/cnv"
)

const SchemaVersion = "sepilens-evaluation-2.0"

type Request struct {
	SchemaVersion string                        `json:"schema_version"`
	EngineID      string                        `json:"engine_id"`
	Mode          string                        `json:"mode"`
	Variant       *acmg2015.SNVVariantInput     `json:"variant,omitempty"`
	CNV           *Region                       `json:"cnv,omitempty"`
	Context       acmg2015.ClinicalContextInput `json:"clinical_context"`
	Package       Package                       `json:"evidence_package"`
	Options       Options                       `json:"options"`
}
type Options struct {
	AllowExperimental bool   `json:"allow_experimental,omitempty"`
	ProviderProfileID string `json:"provider_profile_id,omitempty"`
	MaxSearches       int    `json:"max_searches,omitempty"`
	CombinationMethod string `json:"combination_method,omitempty"`
}
type Region struct {
	cnv.CNVInput
	CoordinateSystem string `json:"coordinate_system"`
}
type Package struct {
	AIContext      *AIContext             `json:"ai_context,omitempty"`
	ContentHash    string                 `json:"content_hash,omitempty"`
	Sources        []Source               `json:"sources"`
	Annotations    *annotation.Set        `json:"annotations,omitempty"`
	GeneAssertions []annotation.GenCC     `json:"gene_assertions,omitempty"`
	Criteria       []Criterion            `json:"criteria,omitempty"`
	Mechanism      *Mechanism             `json:"mechanism,omitempty"`
	Dosage         *Dosage                `json:"dosage,omitempty"`
	Phenotype      *Phenotype             `json:"phenotype,omitempty"`
	Draft          *svcv4.DraftAssessment `json:"draft_assessment,omitempty"`
	DraftSourceIDs []string               `json:"draft_source_ids,omitempty"`
}

// Imported context is model input, never verified evidence for medical rules.
type AIContext struct {
	VariantAnnotations map[string]any   `json:"variant_annotations"`
	Sample             *AISampleContext `json:"sample"`
}

type AISampleContext struct {
	Gender               string              `json:"gender"`
	Age                  *int                `json:"age"`
	SampleType           string              `json:"sample_type"`
	ClinicalDiagnosis    AIClinicalDiagnosis `json:"clinical_diagnosis"`
	HPOTerms             []AIHPOTerm         `json:"hpo_terms"`
	FamilyHistory        AIFamilyHistory     `json:"family_history"`
	FatherAffectedStatus string              `json:"father_affected_status"`
	MotherAffectedStatus string              `json:"mother_affected_status"`
}
type AIHPOTerm struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type AIClinicalDiagnosis struct {
	MainDiagnosis  string      `json:"mainDiagnosis"`
	Symptoms       []string    `json:"symptoms"`
	HPOTerms       []AIHPOTerm `json:"hpoTerms,omitempty"`
	OnsetAge       string      `json:"onsetAge,omitempty"`
	DiseaseHistory string      `json:"diseaseHistory,omitempty"`
}
type AIFamilyHistory struct {
	HasHistory      bool               `json:"hasHistory"`
	AffectedMembers []AIAffectedMember `json:"affectedMembers,omitempty"`
	PedigreeNote    string             `json:"pedigreeNote,omitempty"`
}
type AIAffectedMember struct {
	Relation  string `json:"relation"`
	Condition string `json:"condition"`
	OnsetAge  string `json:"onsetAge,omitempty"`
}

func InferClinicalContext(r Request) bool {
	return r.EngineID == "acmg_amp_2015" && r.Mode == "ai_assisted" && r.Package.AIContext != nil
}

type Object struct {
	Assembly    string `json:"assembly"`
	Chrom       string `json:"chrom"`
	Pos         int64  `json:"pos,omitempty"`
	Ref         string `json:"ref,omitempty"`
	Alt         string `json:"alt,omitempty"`
	Start       int64  `json:"start,omitempty"`
	End         int64  `json:"end,omitempty"`
	Gene        string `json:"gene,omitempty"`
	DiseaseID   string `json:"disease_id"`
	Inheritance string `json:"inheritance"`
}
type Source struct {
	ID               string `json:"id"`
	Dataset          string `json:"dataset"`
	Kind             string `json:"kind"`
	Version          string `json:"version"`
	RetrievedAt      string `json:"retrieved_at"`
	LicenseConfirmed bool   `json:"license_confirmed"`
	RecordID         string `json:"record_id"`
	Object           Object `json:"object"`
	Complete         bool   `json:"complete"`
	Clinical         bool   `json:"clinical"`
}
type Criterion struct {
	Code      string          `json:"code"`
	Status    string          `json:"status"`
	Strength  string          `json:"strength,omitempty"`
	Score     *int64          `json:"score_hundredths,omitempty"`
	Points    *svcv4.Point    `json:"points,omitempty"`
	SourceIDs []string        `json:"source_ids"`
	CaseIDs   []string        `json:"case_ids,omitempty"`
	Facts     map[string]bool `json:"facts"`
	Reason    string          `json:"reason,omitempty"`
	Missing   []string        `json:"missing_information,omitempty"`
}
type Mechanism struct {
	SourceIDs          []string `json:"source_ids"`
	LOFEstablished     bool     `json:"lof_established"`
	TranscriptRelevant bool     `json:"transcript_relevant"`
	NMDApplicable      bool     `json:"nmd_applicable"`
	Transcript         string   `json:"transcript"`
}
type Dosage struct {
	SourceIDs           []string           `json:"source_ids"`
	Regions             []cnv.DosageRegion `json:"regions"`
	Coverage            Object             `json:"coverage"`
	Complete            bool               `json:"complete"`
	ProteinCodingGenes  *int               `json:"protein_coding_genes,omitempty"`
	GeneCountSourceIDs  []string           `json:"gene_count_source_ids,omitempty"`
	FunctionalElements  *bool              `json:"functional_elements,omitempty"`
	FunctionalSourceIDs []string           `json:"functional_source_ids,omitempty"`
}
type Phenotype struct {
	SourceIDs              []string                `json:"source_ids"`
	ClinicalSourceIDs      []string                `json:"clinical_source_ids"`
	Input                  phenotype.Input         `json:"input"`
	Terms                  map[string]hpo.Term     `json:"terms"`
	Associations           []phenotype.Association `json:"associations"`
	Background             map[string]float64      `json:"background"`
	CandidateDiseases      []string                `json:"candidate_diseases"`
	GeneAssertionsComplete bool                    `json:"gene_assertions_complete"`
	OntologyComplete       bool                    `json:"ontology_complete"`
	Version                string                  `json:"version"`
	OntologyVersion        string                  `json:"ontology_version"`
}
type Result struct {
	EngineID       string           `json:"engine_id"`
	Status         string           `json:"status"`
	Classification string           `json:"classification"`
	Criteria       []Criterion      `json:"criteria"`
	Missing        []string         `json:"missing_information"`
	Computation    any              `json:"computation"`
	SectionScores  map[string]int64 `json:"section_scores_hundredths,omitempty"`
	Phenotype      any              `json:"phenotype,omitempty"`
	Experimental   any              `json:"experimental_result,omitempty"`
}
