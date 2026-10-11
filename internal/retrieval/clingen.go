package retrieval

import (
	"context"
	"fmt"
	"github.com/pzweuj/sepilens/internal/evidence"
	"github.com/pzweuj/sepilens/internal/variant/cnv"
	"strings"
)

type ClinGenRegionRetriever struct {
	regions []cnv.DosageRegion
	hash    string
}

func NewClinGenRegionRetriever(regions []cnv.DosageRegion, hash string) *ClinGenRegionRetriever {
	return &ClinGenRegionRetriever{append([]cnv.DosageRegion(nil), regions...), hash}
}
func (*ClinGenRegionRetriever) Name() string   { return "clingen_dosage" }
func (*ClinGenRegionRetriever) IsOnline() bool { return false }
func (r *ClinGenRegionRetriever) Retrieve(ctx context.Context, q Query) ([]evidence.Source, error) {
	var out []evidence.Source
	for _, g := range r.regions {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		if g.Version == "" || g.Assembly != q.Assembly || g.IsGene {
			continue
		}
		match := strings.EqualFold(g.Name, q.Gene) && q.Gene != ""
		if q.Start > 0 {
			match = strings.TrimPrefix(g.Chrom, "chr") == strings.TrimPrefix(q.Chrom, "chr") && q.Start <= g.End && q.End >= g.Start
		}
		if !match {
			continue
		}
		out = append(out, evidence.Source{ID: fmt.Sprintf("ClinGen:%s:%s:%d-%d", g.Assembly, g.Name, g.Start, g.End), Type: evidence.SourceTypeCuratedSnapshot, Title: g.Name, DatasetVersion: r.hash, IdentifierValid: true, RecordExists: true, ObjectMatched: true, MaterialVerified: false, ValidationStatus: evidence.ValidationUnverified, Snippet: fmt.Sprintf("assembly=%s;coordinates=%s:%d-%d;HI=%d;TS=%d;source_version=%s", g.Assembly, g.Chrom, g.Start, g.End, g.HIScore, g.TSScore, g.Version)})
	}
	return out, nil
}
