package cnv

import (
	"sort"
	"strings"
)

type DosageScore int

const (
	ScoreNone       DosageScore = 0
	ScoreLittle     DosageScore = 1
	ScoreEmerging   DosageScore = 2
	ScoreSufficient DosageScore = 3
	ScoreBenign     DosageScore = 40
)

type Range struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
}
type DosageRegion struct {
	Name          string      `json:"name"`
	Assembly      string      `json:"assembly"`
	Chrom         string      `json:"chrom"`
	Start         int64       `json:"start"`
	End           int64       `json:"end"`
	HIScore       DosageScore `json:"hi_score"`
	TSScore       DosageScore `json:"ts_score"`
	IsBenign      bool        `json:"is_benign"`
	IsGene        bool        `json:"is_gene"`
	ProteinCoding bool        `json:"protein_coding"`
	Exons         []Range     `json:"exons,omitempty"`
	Strand        string      `json:"strand,omitempty"`
	Version       string      `json:"version"`
}
type CNVInput struct {
	CopyNumber          int    `json:"copy_number,omitempty"`
	BreakpointPrecision string `json:"breakpoint_precision,omitempty"`
	Mosaicism           string `json:"mosaicism,omitempty"`
	Orientation         string `json:"orientation,omitempty"`
	Assembly            string `json:"assembly"`
	Chrom               string `json:"chrom"`
	Start               int64  `json:"start"`
	End                 int64  `json:"end"`
	Type                string `json:"type"`
}
type AnnotatedCNV struct {
	Input                      CNVInput    `json:"input"`
	Length                     int64       `json:"length"`
	GeneCount                  int         `json:"gene_count"`
	OverlappingGenes           []string    `json:"overlapping_genes"`
	HighestHI                  DosageScore `json:"highest_hi"`
	HighestHIGene              string      `json:"highest_hi_gene"`
	HighestTS                  DosageScore `json:"highest_ts"`
	HighestTSGene              string      `json:"highest_ts_gene"`
	ContainsFunctionalElements bool        `json:"contains_functional_elements"`
	CompletelyInBenign         bool        `json:"completely_in_benign"`
	OverlapsBenign             bool        `json:"overlaps_benign"`
	IsIntragenic               bool        `json:"is_intragenic"`
	CompleteHIOverlap          bool        `json:"complete_hi_overlap"`
	CompleteTSOverlap          bool        `json:"complete_ts_overlap"`
	CoverageComplete           bool        `json:"coverage_complete"`
	DatasetHash                string      `json:"dataset_hash"`
}
type Annotator struct {
	curatedRegions []DosageRegion
	coverage       map[string]bool
	hash           string
}

func NewAnnotator() *Annotator { return &Annotator{coverage: map[string]bool{}, hash: "unavailable"} }
func NewSnapshotAnnotator(regions []DosageRegion, assemblies []string, hash string) *Annotator {
	a := NewAnnotator()
	a.curatedRegions = append([]DosageRegion(nil), regions...)
	a.hash = hash
	for _, asm := range assemblies {
		a.coverage[asm] = true
	}
	return a
}
func (a *Annotator) DatasetHash() string { return a.hash }
func (a *Annotator) Annotate(in CNVInput) AnnotatedCNV {
	r := AnnotatedCNV{Input: in, Length: in.End - in.Start + 1, CoverageComplete: a.coverage[in.Assembly], DatasetHash: a.hash}
	genes := map[string]bool{}
	for _, g := range a.curatedRegions {
		if g.Assembly != in.Assembly || norm(g.Chrom) != norm(in.Chrom) || in.Start > g.End || in.End < g.Start {
			continue
		}
		if g.IsGene {
			r.ContainsFunctionalElements = true
			if g.ProteinCoding {
				genes[g.Name] = true
			}
			if in.Start >= g.Start && in.End <= g.End {
				r.IsIntragenic = true
			}
		} else if g.HIScore == ScoreSufficient || g.TSScore == ScoreSufficient {
			r.ContainsFunctionalElements = true
		}
		if g.IsBenign {
			r.OverlapsBenign = true
			r.CompletelyInBenign = r.CompletelyInBenign || (in.Start >= g.Start && in.End <= g.End)
		}
		if g.HIScore <= 3 && g.HIScore > r.HighestHI {
			r.HighestHI = g.HIScore
			r.HighestHIGene = g.Name
		}
		if g.TSScore <= 3 && g.TSScore > r.HighestTS {
			r.HighestTS = g.TSScore
			r.HighestTSGene = g.Name
		}
		if g.HIScore == ScoreSufficient && in.Start <= g.Start && in.End >= g.End {
			r.CompleteHIOverlap = true
		}
		if g.TSScore == ScoreSufficient && in.Start <= g.Start && in.End >= g.End {
			r.CompleteTSOverlap = true
		}
	}
	for g := range genes {
		r.OverlappingGenes = append(r.OverlappingGenes, g)
	}
	sort.Strings(r.OverlappingGenes)
	r.GeneCount = len(genes)
	return r
}
func norm(s string) string { return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(s)), "chr") }
