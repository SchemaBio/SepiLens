package cnv_test

import (
	"github.com/pzweuj/sepilens/internal/variant/cnv"
	"testing"
)

func TestSnapshotAssemblyAndGeneDedup(t *testing.T) {
	in := cnv.CNVInput{Assembly: "GRCh38", Chrom: "chr1", Start: 10, End: 50, Type: "loss"}
	if cnv.NewAnnotator().Annotate(in).CoverageComplete {
		t.Fatal("no data claims coverage")
	}
	genes := []cnv.DosageRegion{{Name: "G", Assembly: "GRCh38", Chrom: "1", Start: 20, End: 40, IsGene: true, ProteinCoding: true, HIScore: 3}, {Name: "G", Assembly: "GRCh38", Chrom: "1", Start: 20, End: 40, IsGene: true, ProteinCoding: true}, {Name: "NONCODING", Assembly: "GRCh38", Chrom: "1", Start: 20, End: 40, IsGene: true}, {Name: "OTHER", Assembly: "GRCh37", Chrom: "1", Start: 20, End: 40, IsGene: true, ProteinCoding: true}}
	a := cnv.NewSnapshotAnnotator(genes, []string{"GRCh38"}, "synthetic")
	r := a.Annotate(in)
	if r.GeneCount != 1 || !r.CompleteHIOverlap || !r.CoverageComplete || r.Length != 41 {
		t.Fatalf("wrong annotation: %+v", r)
	}
	in.Assembly = "GRCh37"
	if a.Annotate(in).CoverageComplete {
		t.Fatal("assembly coverage leak")
	}
}
