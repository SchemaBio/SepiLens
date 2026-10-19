package api

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/pzweuj/sepilens/internal/annotation"
	"github.com/pzweuj/sepilens/internal/engine"
	"github.com/pzweuj/sepilens/internal/rules/acmg2015"
	"github.com/pzweuj/sepilens/internal/workflow"
)

func TestPVS1PrerequisitesCannotBeInferredFromLabels(t *testing.T) {
	r := &SNVEvaluationRequest{Variant: SNVVariant{Transcript: "NM_000001.1"}, Annotations: &annotation.Set{GenCC: &annotation.GenCC{Classification: "Definitive"}}}
	o := deriveOptions(r)
	if o.PVS1MechanismConfirmed || o.TranscriptRelevant {
		t.Fatal("PVS1 prerequisites inferred from labels")
	}
}

func TestSnapshotFreshnessUsesPinnedAssessmentClock(t *testing.T) {
	now := time.Now().UTC()
	asOf := now.AddDate(0, 0, -2)
	q := annotation.Query{Assembly: "GRCh38", Chrom: "1", Pos: 1, Ref: "A", Alt: "G", Gene: "G"}
	p := filepath.Join(t.TempDir(), "clock.db")
	assets := []annotation.AssetMetadata{{Kind: "gnomad", Version: "test", Assembly: q.Assembly, LicenseConfirmed: true, RetrievedAt: now.AddDate(0, 0, -91).Format(time.RFC3339)}}
	if e := annotation.BuildIndex(context.Background(), p, "clock", assets, func(emit func(annotation.IndexedRecord) error) error {
		return emit(annotation.IndexedRecord{Kind: "gnomad", Query: q, Data: json.RawMessage(`{"faf95":0.00001,"allele_number":10000,"coverage_adequate":true}`)})
	}); e != nil {
		t.Fatal(e)
	}
	index, e := annotation.OpenIndex(p, "clock")
	if e != nil {
		t.Fatal(e)
	}
	defer index.Close()
	s := &Server{annotationIndex: index, acmgEvaluator: acmg2015.NewEvaluator(nil), approvals: map[engine.EngineID]engine.Approval{engine.EngineACMG2015: {ContentHash: "synthetic"}}}
	in := envelope{AssessmentTime: asOf, DatasetHash: "clock", RulesetHash: "synthetic", SNV: &SNVEvaluationRequest{Mode: ModeLocal, Variant: SNVVariant{Assembly: q.Assembly, Chrom: q.Chrom, Pos: q.Pos, Ref: q.Ref, Alt: q.Alt, Gene: q.Gene}, ClinicalContext: SNVContext{DiseaseID: "OMIM:1", Inheritance: "AD"}, Options: EvaluationOptions{AnnotationSource: "sepilens_snapshot"}}}
	b, _ := json.Marshal(in)
	_, snap, e := s.execute(context.Background(), workflow.Task{JobID: "j", TenantID: "t", EngineID: string(engine.EngineACMG2015), Kind: "production", Payload: b})
	if e != nil {
		t.Fatal(e)
	}
	var a annotation.Set
	if e = json.Unmarshal(snap.Annotations, &a); e != nil || a.GnomAD.Status != annotation.StatusFound {
		t.Fatal("clock was not pinned", a, e)
	}
}
func TestPublicAnnotationSourceClaimsAreUntrusted(t *testing.T) {
	for _, source := range []annotation.Source{annotation.SourceCaller, annotation.SourceSnapshot, annotation.SourceAPI} {
		r := &SNVEvaluationRequest{EngineID: "acmg_amp_2015", Variant: SNVVariant{Assembly: "GRCh38", Chrom: "1", Pos: 1, Ref: "A", Alt: "G", Gene: "TEST"}, ClinicalContext: SNVContext{DiseaseID: "OMIM:1", Inheritance: "AD", VariantOrigin: "germline_confirmed", SampleType: "blood"}, Annotations: &annotation.Set{Source: source, GnomAD: &annotation.GnomAD{Record: annotation.Record{Source: source, Version: "synthetic"}}}}
		if _, _, msg, ok := validateSNVAdmission(r); !ok {
			t.Fatal(msg)
		}
		if r.snapshotResolved || r.Annotations.Source != annotation.SourceCaller || r.Annotations.GnomAD.Source != annotation.SourceCaller {
			t.Fatal("trust claim accepted", r)
		}
	}
}
