package retrieval

import (
	"context"
	"fmt"
	"strings"

	"github.com/pzweuj/sepilens/internal/evidence"
)

type ClinGenDosageRetriever struct {
	snapshotData map[string]evidence.Source
}

func NewClinGenDosageRetriever(snapshot map[string]evidence.Source) *ClinGenDosageRetriever {
	if snapshot == nil {
		snapshot = make(map[string]evidence.Source)
	}
	return &ClinGenDosageRetriever{snapshotData: snapshot}
}

func (r *ClinGenDosageRetriever) Name() string   { return "clingen_dosage" }
func (r *ClinGenDosageRetriever) IsOnline() bool { return false } // Offline snapshot

func (r *ClinGenDosageRetriever) Retrieve(ctx context.Context, q Query) ([]evidence.Source, error) {
	geneUpper := strings.ToUpper(strings.TrimSpace(q.Gene))
	if src, ok := r.snapshotData[geneUpper]; ok {
		return []evidence.Source{src}, nil
	}
	return nil, nil
}

type ClinVarRetriever struct {
	isOnline bool
	mockData map[string][]evidence.Source
}

func NewClinVarRetriever(isOnline bool, mock map[string][]evidence.Source) *ClinVarRetriever {
	if mock == nil {
		mock = make(map[string][]evidence.Source)
	}
	return &ClinVarRetriever{isOnline: isOnline, mockData: mock}
}

func (r *ClinVarRetriever) Name() string   { return "clinvar" }
func (r *ClinVarRetriever) IsOnline() bool { return r.isOnline }

func (r *ClinVarRetriever) Retrieve(ctx context.Context, q Query) ([]evidence.Source, error) {
	key := fmt.Sprintf("%s:%s", q.Gene, q.HGVS)
	if sources, ok := r.mockData[key]; ok {
		return sources, nil
	}
	if sources, ok := r.mockData[q.Gene]; ok {
		return sources, nil
	}
	return nil, nil
}

type PubMedRetriever struct {
	isOnline bool
	mockData map[string][]evidence.Source
}

func NewPubMedRetriever(isOnline bool, mock map[string][]evidence.Source) *PubMedRetriever {
	if mock == nil {
		mock = make(map[string][]evidence.Source)
	}
	return &PubMedRetriever{isOnline: isOnline, mockData: mock}
}

func (r *PubMedRetriever) Name() string   { return "pubmed" }
func (r *PubMedRetriever) IsOnline() bool { return r.isOnline }

func (r *PubMedRetriever) Retrieve(ctx context.Context, q Query) ([]evidence.Source, error) {
	key := q.Gene
	if sources, ok := r.mockData[key]; ok {
		return sources, nil
	}
	return nil, nil
}
