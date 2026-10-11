package svcv4

import "math/big"

func alternate(c Case) (plp, vus, sameGene bool) {
	for _, v := range c.Additional {
		switch normalized(v.Classification) {
		case "P", "LP":
			plp = true
			sameGene = sameGene || v.Phase != ""
		case "VUS":
			vus = true
		}
	}
	return
}
func thorough(c Case) bool {
	_, vus, _ := alternate(c)
	return c.Testing != nil && c.Testing.CoversAllGenes == "TRUE" && c.Testing.NonGeneticExcluded == "TRUE" && !vus
}
func biallelic(c Case, moi string) (bool, bool) {
	switch moi {
	case "AD", "XLD":
		return false, true
	case "AR":
		return true, true
	case "XLR":
		if c.Sex == "M" {
			return false, true
		}
		return true, c.Sex == "F"
	case "SD":
		return c.Zygosity == "HOM" || (c.Zygosity == "HET" && c.CompoundHet != nil), true
	}
	return false, false
}
func affectedPoints(c Case, bi bool) *Point {
	if c.Phenotype == "" {
		return nil
	}
	plp, vus, _ := alternate(c)
	if !bi {
		if c.Phenotype == "INCONSISTENT" || plp {
			return ptr(0)
		}
		base := Point(Unit)
		if c.Phenotype == "CONSISTENT" {
			base /= 2
		}
		if !thorough(c) || vus {
			base /= 2
		}
		return ptr(base)
	}
	col := "none"
	switch c.Zygosity {
	case "HOM":
		col = "hom"
	case "HET":
		if ch := c.CompoundHet; ch != nil {
			switch normalized(ch.Classification) {
			case "P", "LP":
				col = "assumed_plp"
				if ch.PhaseConfidence == "HIGH" {
					col = "conf_plp"
				}
			case "VUS":
				if ch.PhaseConfidence == "HIGH" {
					col = "conf_vus"
				}
			}
		}
	default:
		return nil
	}
	if plp || c.Phenotype == "INCONSISTENT" {
		return ptr(0)
	}
	row := "B"
	if thorough(c) {
		if col == "hom" {
			row = "A1"
		} else if c.CompoundHet != nil {
			switch c.CompoundHet.CoOccurrence {
			case "LT_0_0001":
				row = "A1"
			case "BETWEEN_0_0001_0_01":
				row = "A2"
			}
		}
	}
	table := map[string]map[string]Point{
		"A1": {"conf_plp": pt(3), "assumed_plp": Point(1500000), "conf_vus": Point(1500000), "hom": pt(1), "none": 0},
		"A2": {"conf_plp": pt(2), "assumed_plp": pt(1), "conf_vus": pt(1), "hom": pt(1), "none": 0},
		"B":  {"conf_plp": pt(1), "assumed_plp": Point(750000), "conf_vus": Point(500000), "hom": Point(500000), "none": 0},
	}
	return ptr(table[row][col])
}
func unaffectedPoints(c Case, moi string) *Point {
	reduced := false
	if oneOf(moi, "AR", "XLD", "XLR") {
		switch c.Zygosity {
		case "HOM", "HEMI":
		case "HET":
			if c.CompoundHet == nil {
				return ptr(0)
			}
			cls := normalized(c.CompoundHet.Classification)
			if !oneOf(cls, "P", "LP") {
				return ptr(0)
			}
			reduced = cls == "LP"
		default:
			return nil
		}
	}
	n := int64(0)
	switch c.Penetrance {
	case "NEAR_100":
		n = -4
	case "PCT_80_100":
		n = -2
	}
	if reduced {
		n /= 2
	}
	return ptr(pt(n))
}
func alternatePoints(c Case) *Point {
	plp, _, sameGene := alternate(c)
	if !plp || c.Severity == "" {
		return nil
	}
	switch c.Severity {
	case "MONO_EQ_EXPECTED":
		return ptr(Point(-500000))
	case "BIALLELIC_LT_EXPECTED":
		if sameGene && oneOf(c.Penetrance, "PCT_80_100", "NEAR_100") {
			return ptr(pt(-1))
		}
	}
	return ptr(0)
}
func deNovo(c Case, bi bool) *Point {
	count := 0
	for _, r := range c.Relatives {
		if r.Parent == "TRUE" {
			if r.Exists != "FALSE" {
				return nil
			}
			count++
		}
	}
	if count < 2 || c.Parentage != "TRUE" {
		return nil
	}
	phenotype := c.Phenotype
	if bi && phenotype == "SPECIFIC" {
		phenotype = "CONSISTENT"
	}
	if phenotype == "SPECIFIC" {
		return ptr(pt(7))
	}
	if phenotype == "CONSISTENT" {
		return ptr(pt(4))
	}
	return ptr(0)
}
func scoreCCS(c *CaseControl) *Point {
	if c == nil || c.OddsRatio == "" || c.VariantCount < 5 || c.CohortSize < 100 || !c.ControlsMatched {
		return nil
	}
	or, _ := number(c.OddsRatio)
	includes := false
	if c.CILower != "" && c.CIUpper != "" {
		l, _ := number(c.CILower)
		h, _ := number(c.CIUpper)
		includes = l.Cmp(big.NewRat(1, 1)) <= 0 && h.Cmp(big.NewRat(1, 1)) >= 0
	}
	if or.Cmp(big.NewRat(5, 1)) > 0 && !includes {
		return ptr(pt(4))
	}
	return ptr(0)
}
func scoreClinical(cases []Case, ccs *CaseControl, moi string, pop map[string]Point) FamilyResult {
	r := empty("CLN")
	add := func(code string, p *Point) {
		if p != nil {
			r.SubCodePoints[code] += *p
		}
	}
	for _, c := range cases {
		if oneOf(c.Phenotype, "SPECIFIC", "CONSISTENT") {
			bi, routed := biallelic(c, moi)
			if routed {
				aff := affectedPoints(c, bi)
				add("CLN_AFF", aff)
				if moi != "AR" {
					add("CLN_ALT", alternatePoints(c))
				}
				if aff != nil && *aff > 0 {
					add("CLN_DNV", deNovo(c, bi))
				}
			}
		} else {
			add("CLN_UAF", unaffectedPoints(c, moi))
		}
	}
	if p := scoreCCS(ccs); p != nil {
		r.SubCodePoints["CLN_CCS"] = *p
		delete(r.SubCodePoints, "CLN_AFF")
		delete(r.SubCodePoints, "CLN_ALT")
		delete(r.SubCodePoints, "CLN_UAF")
	}
	if f, ok := pop["POP_FRQ"]; !ok || !oneOf(f.String(), "0", "-1") {
		delete(r.SubCodePoints, "CLN_AFF")
		delete(r.SubCodePoints, "CLN_DNV")
		r.Missing = append(r.Missing, "POP_FRQ gate: pathogenic case counting requires scored 0 or -1")
	}
	sumCodes(&r)
	return r
}
