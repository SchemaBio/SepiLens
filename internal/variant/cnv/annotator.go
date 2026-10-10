package cnv

import (
	"strings"
)

type DosageScore int

const (
	ScoreNone       DosageScore = 0  // No evidence
	ScoreLittle     DosageScore = 1  // Little evidence
	ScoreEmerging   DosageScore = 2  // Emerging evidence
	ScoreSufficient DosageScore = 3  // Sufficient evidence
	ScoreBenign     DosageScore = 40 // Dosage sensitivity unlikely / benign
)

type DosageRegion struct {
	Name        string      `json:"name"`
	Assembly    string      `json:"assembly"`
	Chrom       string      `json:"chrom"`
	Start       int64       `json:"start"`
	End         int64       `json:"end"`
	HIScore     DosageScore `json:"hi_score"`
	TSScore     DosageScore `json:"ts_score"`
	IsBenign    bool        `json:"is_benign"`
	IsGene      bool        `json:"is_gene"`
}

type CNVInput struct {
	Assembly string `json:"assembly"`
	Chrom    string `json:"chrom"`
	Start    int64  `json:"start"`
	End      int64  `json:"end"`
	Type     string `json:"type"` // "loss" or "gain"
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
}

type Annotator struct {
	curatedRegions []DosageRegion
}

func NewAnnotator() *Annotator {
	return &Annotator{
		curatedRegions: defaultCuratedRegions(),
	}
}

func defaultCuratedRegions() []DosageRegion {
	return []DosageRegion{
		// 17p11.2 / RAI1 (Smith-Magenis syndrome): HI=3
		{Name: "RAI1", Assembly: "GRCh38", Chrom: "17", Start: 17680000, End: 17810000, HIScore: ScoreSufficient, TSScore: ScoreNone, IsGene: true},
		{Name: "17p11.2_region", Assembly: "GRCh38", Chrom: "17", Start: 16700000, End: 20400000, HIScore: ScoreSufficient, TSScore: ScoreNone, IsGene: false},

		// 17p12 / PMP22 (HNPP loss / CMT1A gain): HI=3, TS=3
		{Name: "PMP22", Assembly: "GRCh38", Chrom: "17", Start: 15119000, End: 15260000, HIScore: ScoreSufficient, TSScore: ScoreSufficient, IsGene: true},

		// 22q11.2 (DiGeorge deletion / duplication syndrome): HI=3, TS=3
		{Name: "22q11.2_region", Assembly: "GRCh38", Chrom: "22", Start: 18800000, End: 21500000, HIScore: ScoreSufficient, TSScore: ScoreSufficient, IsGene: false},

		// PTEN: HI=3
		{Name: "PTEN", Assembly: "GRCh38", Chrom: "10", Start: 87860000, End: 87970000, HIScore: ScoreSufficient, TSScore: ScoreNone, IsGene: true},

		// MECP2: HI=3, TS=3
		{Name: "MECP2", Assembly: "GRCh38", Chrom: "X", Start: 154020000, End: 154100000, HIScore: ScoreSufficient, TSScore: ScoreSufficient, IsGene: true},

		// Established Benign Regions
		{Name: "BENIGN_REG_CHR1", Assembly: "GRCh38", Chrom: "1", Start: 72000000, End: 72500000, HIScore: ScoreBenign, TSScore: ScoreBenign, IsBenign: true},
	}
}

func (a *Annotator) Annotate(in CNVInput) AnnotatedCNV {
	chromNorm := strings.TrimPrefix(strings.ToLower(in.Chrom), "chr")
	length := in.End - in.Start + 1
	if length < 0 {
		length = 0
	}

	res := AnnotatedCNV{
		Input:                      in,
		Length:                     length,
		ContainsFunctionalElements: false,
	}

	for _, reg := range a.curatedRegions {
		regChromNorm := strings.TrimPrefix(strings.ToLower(reg.Chrom), "chr")
		if regChromNorm != chromNorm {
			continue
		}

		// Overlap check: start <= reg.End && end >= reg.Start
		if in.Start <= reg.End && in.End >= reg.Start {
			if reg.IsGene {
				res.OverlappingGenes = append(res.OverlappingGenes, reg.Name)
				res.GeneCount++
				res.ContainsFunctionalElements = true
			} else if reg.HIScore > 0 || reg.TSScore > 0 {
				res.ContainsFunctionalElements = true
			}

			// Benign region overlap
			if reg.IsBenign {
				res.OverlapsBenign = true
				if in.Start >= reg.Start && in.End <= reg.End {
					res.CompletelyInBenign = true
				}
			}

			// HI Evaluation
			if reg.HIScore > res.HighestHI {
				res.HighestHI = reg.HIScore
				res.HighestHIGene = reg.Name
			}
			// Complete HI overlap: CNV encompasses entire established HI region/gene
			if reg.HIScore == ScoreSufficient && in.Start <= reg.Start && in.End >= reg.End {
				res.CompleteHIOverlap = true
			}

			// TS Evaluation
			if reg.TSScore > res.HighestTS {
				res.HighestTS = reg.TSScore
				res.HighestTSGene = reg.Name
			}
			// Complete TS overlap: CNV encompasses entire established TS region/gene
			if reg.TSScore == ScoreSufficient && in.Start <= reg.Start && in.End >= reg.End {
				res.CompleteTSOverlap = true
			}

			// Intragenic check
			if reg.IsGene && in.Start > reg.Start && in.End < reg.End {
				res.IsIntragenic = true
			}
		}
	}

	// Default assumption: if length > 50kb and not in gene desert, likely contains functional elements
	if length > 50000 && res.GeneCount > 0 {
		res.ContainsFunctionalElements = true
	}

	return res
}
