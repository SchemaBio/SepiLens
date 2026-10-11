package svcv4

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"sort"
	"strings"
)

//go:embed branches.json
var branchBytes []byte

//go:embed upstream-lock.json
var lockBytes []byte

//go:embed model.go score.go clinical.go
var implementation embed.FS

type branch struct {
	Parent        string `json:"parent_code"`
	PRDLo         Point  `json:"prd_lo"`
	PRDHi         Point  `json:"prd_hi"`
	HeldHi        *Point `json:"held_hi"`
	ParentLo      *Point `json:"parent_lo"`
	ParentHi      *Point `json:"parent_hi"`
	INFLo         *Point `json:"inf_lo"`
	INFHi         *Point `json:"inf_hi"`
	MechanismOnly bool   `json:"sm18_mechanism_only"`
	FXNNA         bool   `json:"fxn_na"`
	Splice        bool   `json:"splice"`
	PRDSPALo      *Point `json:"prd_spa_lo"`
	PRDSPAHi      *Point `json:"prd_spa_hi"`
	PRDSPAFXNHi   *Point `json:"prd_spa_fxn_hi"`
}

var branchTable = func() map[string]map[string]branch {
	var b map[string]map[string]branch
	if json.Unmarshal(branchBytes, &b) != nil {
		panic("invalid embedded SVC draft branches")
	}
	return b
}()

// RulePackage binds the locked parameters and the actual native implementation.
func RulePackage() []byte {
	sources := map[string]string{}
	for _, name := range []string{"model.go", "score.go", "clinical.go"} {
		b, err := implementation.ReadFile(name)
		if err != nil {
			panic(err)
		}
		sources[name] = string(b)
	}
	b, err := json.Marshal(map[string]any{"upstream": json.RawMessage(lockBytes), "branches": json.RawMessage(branchBytes), "native_sources": sources})
	if err != nil {
		panic(err)
	}
	return b
}
func RulesetHash() string { h := sha256.Sum256(RulePackage()); return hex.EncodeToString(h[:]) }
func DeclaredCodes() []string {
	return []string{"POP_FRQ", "POP_HMZ", "CLN_AFF", "CLN_ALT", "CLN_UAF", "CLN_DNV", "CLN_CCS", "LOC_PHE", "LOC_SEG", "NUL_PRD", "NUL_FXN", "NUL_INF", "SPL_PRD", "SPL_SPA", "SPL_FXN", "SPL_INF", "MIS_PRD", "MIS_FXN", "MIS_INF", "CDS_PRD", "CDS_FXN", "CDS_INF"}
}
func fallback(v *Point, d Point) Point {
	if v == nil {
		return d
	}
	return *v
}
func clamp(v *Point, lo, hi Point) *Point {
	if v == nil {
		return nil
	}
	p := *v
	if p < lo {
		p = lo
	}
	if p > hi {
		p = hi
	}
	return ptr(p)
}
func hold(lo, hi Point, parts ...*Point) *Point {
	var sum Point
	found := false
	for _, p := range parts {
		if p != nil {
			sum += *p
			found = true
		}
	}
	if !found {
		return nil
	}
	return clamp(&sum, lo, hi)
}
func empty(code string) FamilyResult {
	return FamilyResult{ParentCode: code, SubCodePoints: map[string]Point{}, Held: map[string]Point{}, Missing: []string{}}
}
func record(r *FamilyResult, code string, p *Point) {
	if p == nil {
		r.Missing = append(r.Missing, code+"_ND")
	} else {
		r.SubCodePoints[code] = *p
	}
}
func sumCodes(r *FamilyResult) {
	if len(r.SubCodePoints) == 0 {
		return
	}
	var n Point
	for _, p := range r.SubCodePoints {
		n += p
	}
	r.Total = &n
}
func scale(p *Point, mech, exon, gdv string, only bool) *Point {
	if p == nil || *p <= 0 {
		return p
	}
	if !oneOf(gdv, "DEFINITIVE", "STRONG", "MODERATE") {
		return ptr(0)
	}
	denominator := int64(1)
	switch mech {
	case "ESTABLISHED":
	case "LIKELY":
		denominator = 2
	case "SUSPECTED":
		denominator = 4
	default:
		return ptr(0)
	}
	if !only {
		if exon == "FEW" || (mech == "SUSPECTED" && exon == "MOST") {
			return ptr(0)
		}
		if exon == "MOST" {
			denominator *= 2
		}
	}
	return ptr(Point(int64(*p) / denominator))
}
func normalized(s string) string {
	switch s {
	case "PATHOGENIC":
		return "P"
	case "LIKELY_PATHOGENIC":
		return "LP"
	case "BENIGN":
		return "B"
	case "LIKELY_BENIGN":
		return "LB"
	}
	return s
}
func standard(nStrong, nWeak int) Point {
	if nStrong+nWeak == 0 {
		return 0
	}
	v := 0
	if nStrong > 0 {
		v = 2 + nStrong - 1
	}
	if nWeak > 0 {
		v += 1 + nWeak - 1
	}
	return pt(int64(v))
}
func doubled(s, w int) Point {
	if s+w == 0 {
		return 0
	}
	first := int64(2)
	if s > 0 {
		first = 4
	}
	return pt(first + 2*int64(s+w-1))
}
func informative(vs []Informative, mis bool) *Point {
	if !mis {
		counts := map[string]int{}
		for _, v := range vs {
			if v.Classification != "" {
				counts[normalized(v.Classification)]++
			}
		}
		if len(counts) == 0 {
			return nil
		}
		return ptr(standard(counts["P"], counts["LP"]) - standard(counts["B"], counts["LB"]))
	}
	cats := map[string]map[string]int{}
	for _, v := range vs {
		if cats[v.Category] == nil {
			cats[v.Category] = map[string]int{}
		}
		cats[v.Category][normalized(v.Classification)]++
	}
	c1, c2, c3, c4 := cats["SAME_AA_PATHOGENIC"], cats["DISTINCT_AA_PATHOGENIC"], cats["DISTINCT_AA_BENIGN"], cats["SAME_AA_BENIGN"]
	if c1["P"]+c1["LP"]+c2["P"]+c2["LP"]+c3["B"]+c3["LB"]+c4["B"]+c4["LB"] == 0 {
		return nil
	}
	return ptr(doubled(c1["P"], c1["LP"]) + standard(c2["P"], c2["LP"]) - standard(c3["B"], c3["LB"]) - doubled(c4["B"], c4["LB"]))
}
func scoreImpact(a *Impact, gdv string) FamilyResult {
	if a == nil {
		return empty("PFD")
	}
	if a.Workflow == "missense" {
		aa, sp := a.AminoAcid, a.Splice
		if aa == nil {
			aa = &Impact{Workflow: "missense_amino_acid"}
		}
		if sp == nil {
			sp = &Impact{Workflow: "missense_splice"}
		}
		amino := scoreImpact(aa, gdv)
		spl := scoreImpact(sp, gdv)
		selected, path := amino, "AMINO_ACID"
		if spl.Total != nil && *spl.Total > 0 && (amino.Total == nil || *spl.Total > *amino.Total) {
			selected, path = spl, "SPLICE"
		}
		selected.ComparedPaths = []FamilyResult{amino, spl}
		selected.SelectedPath = path
		return selected
	}
	if a.Workflow == "exon_duplication" && a.Outcome == "WHOLE_GENE_NA" {
		r := empty("CDS")
		r.Missing = append(r.Missing, "WHOLE_GENE_NA")
		return r
	}
	b := branchTable[a.Workflow][a.Outcome]
	r := empty(b.Parent)
	if b.Splice || a.Workflow == "missense_splice" {
		b.Splice = true
		r.ParentCode = "SPL"
	}
	if a.Workflow == "missense_amino_acid" {
		b = branch{Parent: "MIS", PRDLo: pt(-4), PRDHi: pt(4), HeldHi: ptr(pt(6)), ParentHi: ptr(pt(9))}
		r.ParentCode = "MIS"
	}
	if a.Workflow == "inframe_indel" {
		b = branch{Parent: "CDS", PRDLo: pt(-1), PRDHi: pt(6)}
		r.ParentCode = "CDS"
		if a.Outcome == "SIMPLE_SEQUENCE_REPEAT" {
			b.PRDHi = 0
			b.HeldHi = ptr(pt(8))
		}
	}
	prd := a.Initial
	if a.Workflow == "missense_amino_acid" {
		if prd != nil && *prd > 0 {
			switch a.Exon {
			case "MOST":
				prd = ptr(*prd / 2)
			case "FEW":
				prd = ptr(0)
			}
		}
	} else if !(a.Workflow == "inframe_indel" && a.Outcome == "SIMPLE_SEQUENCE_REPEAT") {
		prd = scale(prd, a.Mechanism, a.Exon, gdv, b.MechanismOnly)
	}
	prd = clamp(prd, b.PRDLo, b.PRDHi)
	record(&r, "PRD", prd)
	inf := clamp(informative(a.Informative, a.Workflow == "missense_amino_acid"), fallback(b.INFLo, pt(-8)), fallback(b.INFHi, pt(8)))
	record(&r, "INF", inf)
	var combined *Point
	if b.Splice {
		record(&r, "SPA", a.SpliceAssay)
		first := hold(fallback(b.PRDSPALo, pt(-8)), fallback(b.PRDSPAHi, pt(10)), prd, a.SpliceAssay)
		if first != nil {
			r.Held["PRD+SPA"] = *first
		}
		record(&r, "FXN", a.Functional)
		combined = hold(pt(-8), fallback(b.PRDSPAFXNHi, pt(9)), first, a.Functional)
		if combined != nil {
			r.Held["PRD+SPA+FXN"] = *combined
		}
	} else if b.FXNNA {
		combined = prd
	} else {
		record(&r, "FXN", a.Functional)
		combined = hold(pt(-8), fallback(b.HeldHi, pt(9)), prd, a.Functional)
		if combined != nil {
			r.Held["PRD+FXN"] = *combined
		}
	}
	r.Total = hold(fallback(b.ParentLo, pt(-8)), fallback(b.ParentHi, pt(10)), combined, inf)
	return r
}
func scorePopulation(p *Population, moi string) FamilyResult {
	r := empty("POP")
	if p == nil {
		return r
	}
	var frq *Point
	if p.FAF != "" && p.DAFT != "" {
		faf, _ := number(p.FAF)
		daft, _ := number(p.DAFT)
		if daft.Sign() > 0 {
			fold := new(big.Rat).Quo(faf, daft)
			n := int64(-6)
			switch {
			case fold.Cmp(big.NewRat(3, 2)) < 0:
				n = 0
			case fold.Cmp(big.NewRat(5, 1)) < 0:
				n = -1
			case fold.Cmp(big.NewRat(15, 1)) < 0:
				n = -3
			}
			frq = ptr(pt(n))
		}
	}
	record(&r, "POP_FRQ", frq)
	var hmz *Point
	if p.HMZEligible == "TRUE" {
		count, present := 0, false
		if p.HomozygoteCount != nil {
			count += *p.HomozygoteCount
			present = true
		}
		if oneOf(moi, "XLD", "XLR") && p.HemizygoteCount != nil {
			count += *p.HemizygoteCount
			present = true
		}
		if present {
			weight := Point(Unit / 2)
			if moi == "AD" {
				weight = Point(Unit)
			}
			hmz = ptr(-weight * Point(max(count-1, 0)))
		}
	}
	record(&r, "POP_HMZ", hmz)
	sumCodes(&r)
	return r
}

// Diagnostic yield input is intentionally strict: ambiguous ratios/ranges are rejected.
func yieldPercent(s string) (*big.Rat, error) {
	s = strings.TrimSpace(s)
	percent := strings.HasSuffix(s, "%")
	s = strings.TrimSuffix(s, "%")
	r, e := number(s)
	if e != nil {
		return nil, errors.New("diagnostic yield requires one explicit decimal percent")
	}
	if !percent && r.Cmp(big.NewRat(1, 1)) < 0 {
		r.Mul(r, big.NewRat(100, 1))
	}
	if r.Cmp(big.NewRat(100, 1)) > 0 {
		return nil, errors.New("diagnostic yield exceeds 100 percent")
	}
	return r, nil
}
func nonSeg(c Case, moi string) bool {
	for _, r := range c.Relatives {
		if r.Affected == "TRUE" && r.Exists == "FALSE" {
			return true
		}
		if moi != "AR" && c.Penetrance == "NEAR_100" && r.Affected == "FALSE" && r.Exists == "TRUE" {
			return true
		}
	}
	return false
}
func scoreLocus(c Case, moi string) FamilyResult {
	r := empty("LOC")
	ns := nonSeg(c, moi)
	if c.Testing != nil && c.Testing.DiagnosticYield != "" {
		pct, _ := yieldPercent(c.Testing.DiagnosticYield)
		n := int64(4)
		switch {
		case pct.Cmp(big.NewRat(33, 1)) < 0:
			n = 0
		case pct.Cmp(big.NewRat(50, 1)) <= 0:
			n = 1
		case pct.Cmp(big.NewRat(68, 1)) < 0:
			n = 2
		case pct.Cmp(big.NewRat(82, 1)) < 0:
			n = 3
		}
		if ns {
			n = 0
		}
		r.SubCodePoints["LOC_PHE"] = pt(n)
	}
	if ns {
		n := int64(0)
		if oneOf(moi, "AD", "XLD", "XLR") || (moi == "AR" && c.Zygosity == "HOM") {
			n = -4
		}
		r.SubCodePoints["LOC_SEG"] = pt(n)
	} else {
		var total Point
		count := 0
		for _, v := range c.Relatives {
			var p *Point
			if v.Affected == "TRUE" && v.Exists == "TRUE" {
				switch moi {
				case "AD":
					if v.Zygosity == "HET" {
						p = ptr(pt(1))
					}
				case "AR":
					if v.Zygosity == "HOM" || v.CompoundHet == "TRUE" {
						p = ptr(pt(2))
					}
				case "SD":
					if v.Severe == "TRUE" && (v.Zygosity == "HOM" || v.CompoundHet == "TRUE") {
						p = ptr(pt(2))
					} else if v.Zygosity == "HET" {
						p = ptr(pt(1))
					}
				case "XLD", "XLR":
					if oneOf(v.Zygosity, "HET", "HOM", "HEMI") || v.CompoundHet == "TRUE" {
						p = ptr(pt(1))
					}
				}
			} else if v.Affected == "FALSE" {
				if moi == "AR" && (v.Exists == "FALSE" || (v.Exists == "TRUE" && v.Zygosity == "HET")) {
					p = ptr(Point(400000))
				} else if moi != "AR" && c.Penetrance == "NEAR_100" && v.Exists == "FALSE" {
					p = ptr(pt(1))
				}
			}
			if p != nil {
				total += *p
				count++
			}
		}
		if count > 0 {
			r.SubCodePoints["LOC_SEG"] = *clamp(&total, 0, pt(4))
		}
	}
	sumCodes(&r)
	if r.Total != nil && *r.Total > pt(4) {
		r.Held["raw_sum"] = *r.Total
		r.Total = ptr(pt(4))
	}
	return r
}
func Classify(p Point) (string, string) {
	switch {
	case p >= pt(10):
		return "pathogenic", ""
	case p >= pt(6):
		return "likely_pathogenic", ""
	case p >= pt(4):
		return "uncertain_significance", "VUS-high"
	case p >= pt(2):
		return "uncertain_significance", "VUS-mid"
	case p > pt(-1):
		return "uncertain_significance", "VUS-low"
	case p > pt(-4):
		return "likely_benign", ""
	default:
		return "benign", ""
	}
}
func Evaluate(a *DraftAssessment) (DraftResult, error) {
	out := DraftResult{Version: Version, UpstreamCommit: Commit, RulesetHash: RulesetHash(), ClinicalUse: "forbidden", Classification: "insufficient_data", Families: []FamilyResult{}, Missing: []string{}, Assumptions: []string{"non-authoritative upstream reference; CSpec remains authoritative", "analyst-coded predictive, functional and splice-assay values are not inferred or independently verified", "upstream known-gaps apply; reference parity is not independent clinical validation"}}
	if err := Validate(a); err != nil {
		return out, err
	}
	pop := scorePopulation(a.Population, a.MOI)
	cln := scoreClinical(a.Cases, a.CaseControl, a.MOI, pop.SubCodePoints)
	out.Families = append(out.Families, pop, cln)
	if a.LocusCaseID != "" {
		for _, c := range a.Cases {
			if c.ID == a.LocusCaseID {
				out.Families = append(out.Families, scoreLocus(c, a.MOI))
			}
		}
		out.Assumptions = append(out.Assumptions, "LOC_SEG entry gate and source ambiguities follow upstream reference assumptions")
	}
	if a.Impact != nil {
		out.Families = append(out.Families, scoreImpact(a.Impact, a.GeneDiseaseValidity))
	}
	var total Point
	scored := false
	for _, f := range out.Families {
		out.Missing = append(out.Missing, f.Missing...)
		if f.Total != nil {
			total += *f.Total
			scored = true
		}
	}
	if scored {
		out.Total = &total
		out.Classification, out.VUSSubclass = Classify(total)
	} else {
		out.Missing = append(out.Missing, "no scored evidence; missing data is not zero-point evidence")
	}
	sort.Strings(out.Missing)
	return out, nil
}
