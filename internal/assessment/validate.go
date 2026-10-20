package assessment

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/pzweuj/sepilens/internal/annotation"
	"github.com/pzweuj/sepilens/internal/config"
	"github.com/pzweuj/sepilens/internal/engine"
	"github.com/pzweuj/sepilens/internal/evidence"
	"github.com/pzweuj/sepilens/internal/rules/acmg2015"
	"github.com/pzweuj/sepilens/internal/rules/cnvgain2020"
	"github.com/pzweuj/sepilens/internal/rules/cnvloss2020"
	"github.com/pzweuj/sepilens/internal/rules/cnvrubric"
	"github.com/pzweuj/sepilens/internal/rules/svcv4"
)

func HashPackage(p Package) string {
	p.ContentHash = ""
	b, _ := json.Marshal(p)
	return engine.Hash(b)
}

func HashRequest(r Request) string {
	r.Package.ContentHash = ""
	b, _ := json.Marshal(r)
	return engine.Hash(b)
}
func ObjectFor(r Request) Object {
	o := Object{DiseaseID: r.Context.DiseaseID, Inheritance: r.Context.Inheritance}
	if r.Variant != nil {
		v := r.Variant
		o.Assembly = v.Assembly
		o.Chrom = v.Chrom
		o.Pos = v.Pos
		o.Ref = v.Ref
		o.Alt = v.Alt
		o.Gene = v.Gene
	}
	if r.CNV != nil {
		v := r.CNV
		o.Assembly = v.Assembly
		o.Chrom = v.Chrom
		o.Start = v.Start
		o.End = v.End
	}
	return o
}
func contains(a []string, s string) bool {
	for _, v := range a {
		if v == s {
			return true
		}
	}
	return false
}
func ValidCode(id, code string) bool {
	if id == "acmg_amp_2015" {
		_, ok := acmg2015.CanonicalCriteria[acmg2015.CriterionCode(code)]
		return ok
	}
	if id == "svc_v4" {
		return contains([]string{"POP", "CLN", "LOC", "NUL", "CDS", "PFD"}, code)
	}
	rules := cnvloss2020.Rules
	if id == "cnv_gain_2020" {
		rules = cnvgain2020.Rules
	}
	for _, d := range cnvrubric.Parse(rules) {
		if d.Code == code {
			return true
		}
	}
	return false
}
func Validate(r *Request) error {
	bad := func(s string) error { return errors.New(s) }
	if r.SchemaVersion != SchemaVersion || !contains([]string{"local", "ai_assisted"}, r.Mode) {
		return bad("unsupported schema_version or mode")
	}
	if !contains([]string{"acmg_amp_2015", "svc_v4", "cnv_loss_2020", "cnv_gain_2020"}, r.EngineID) {
		return bad("unsupported engine_id")
	}
	if r.Context.DiseaseID == "" || r.Context.Inheritance == "" || !contains([]string{"germline_confirmed", "somatic_confirmed", "unknown"}, r.Context.VariantOrigin) || r.Context.SampleType == "" {
		return bad("clinical context required")
	}
	if r.Context.VariantOrigin == "somatic_confirmed" {
		return bad("constitutional rules do not support somatic variants")
	}
	if !regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*:[A-Za-z0-9_.-]+$`).MatchString(r.Context.DiseaseID) || !contains([]string{"AD", "AR", "XL", "XLD", "XLR", "SD", "MT"}, r.Context.Inheritance) || !contains([]string{"blood", "saliva", "buccal", "fibroblast", "tissue", "other", "unknown"}, r.Context.SampleType) {
		return bad("invalid structured clinical context")
	}
	if r.Options.MaxSearches < 0 || r.Options.MaxSearches > 100 || !contains([]string{"", "acmg_2015_table", "svi_points"}, r.Options.CombinationMethod) {
		return bad("invalid options")
	}
	if r.Mode == "local" && r.Options.ProviderProfileID != "" {
		return bad("local mode cannot select a provider")
	}
	sequence := r.EngineID == "acmg_amp_2015" || r.EngineID == "svc_v4"
	if sequence {
		if r.Variant == nil || r.CNV != nil {
			return bad("sequence engine requires variant only")
		}
		v := r.Variant
		if !regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,80}$`).MatchString(v.Gene) || len(v.Ref) > 10000 || len(v.Alt) > 10000 {
			return bad("invalid gene or allele length")
		}
		for _, text := range []string{v.Transcript, v.HGVSc, v.HGVSp, v.Consequence} {
			if len(text) > 512 || strings.ContainsAny(text, " \t\r\n\"<>{}") {
				return bad("invalid structured variant annotation")
			}
		}
		if v.Pos < 1 || v.Gene == "" || !regexp.MustCompile(`^[ACGTN]+$`).MatchString(v.Ref) || !regexp.MustCompile(`^[ACGTN]+$`).MatchString(v.Alt) || v.Ref == v.Alt {
			return bad("invalid variant")
		}
		if v.AlleleFrequency != nil && (*v.AlleleFrequency < 0 || *v.AlleleFrequency > 1) {
			return bad("invalid allele frequency")
		}
	} else {
		if r.CNV == nil || r.Variant != nil {
			return bad("CNV engine requires cnv only")
		}
		v := r.CNV
		if v.Start < 1 || v.End < v.Start || v.CoordinateSystem != "1-based-inclusive" || v.CopyNumber < 0 {
			return bad("CNV requires positive 1-based-inclusive coordinates")
		}
		if (r.EngineID == "cnv_loss_2020" && v.Type != "loss") || (r.EngineID == "cnv_gain_2020" && v.Type != "gain") {
			return bad("CNV type and engine mismatch")
		}
		if !contains([]string{"", "exact", "uncertain"}, v.BreakpointPrecision) || !contains([]string{"", "present", "absent", "unknown"}, v.Mosaicism) || !contains([]string{"", "tandem", "unknown"}, v.Orientation) {
			return bad("invalid CNV applicability")
		}
	}
	o := ObjectFor(*r)
	if !contains([]string{"GRCh37", "GRCh38"}, o.Assembly) || !regexp.MustCompile(`^(chr)?([1-9]|1[0-9]|2[0-2]|X|Y|M|MT)$`).MatchString(o.Chrom) {
		return bad("canonical assembly and chromosome required")
	}
	if r.Package.ContentHash != "" && r.Package.ContentHash != HashPackage(r.Package) {
		return bad("evidence package content hash mismatch")
	}
	seen := map[string]bool{}
	for _, s := range r.Package.Sources {
		if s.ID == "" || seen[s.ID] || s.Dataset == "" || s.Kind == "" || s.Version == "" || s.RecordID == "" {
			return bad("invalid or duplicate source")
		}
		seen[s.ID] = true
		if _, e := time.Parse(time.RFC3339, s.RetrievedAt); e != nil {
			return bad("source timestamp required")
		}
		if s.Object != o {
			return bad("source object mismatch")
		}
	}
	if a := r.Package.Annotations; a != nil {
		if err := a.Validate(o.Assembly); err != nil {
			return err
		}
	}
	seen = map[string]bool{}
	for _, c := range r.Package.Criteria {
		if !ValidCode(r.EngineID, c.Code) || seen[c.Code] || !contains([]string{"met", "not_met", "not_applicable", "insufficient_data", "conflicting"}, c.Status) {
			return bad("unknown, duplicate or invalid criterion")
		}
		knownFacts := map[string]bool{"evidence_independent": true}
		if r.EngineID == "acmg_amp_2015" {
			for _, f := range acmg2015.RequiredFacts[acmg2015.CriterionCode(c.Code)] {
				knownFacts[f] = true
			}
		} else if r.CNV != nil {
			rules := cnvloss2020.Rules
			if r.EngineID == "cnv_gain_2020" {
				rules = cnvgain2020.Rules
			}
			for _, d := range cnvrubric.Parse(rules) {
				if d.Code == c.Code {
					for _, f := range d.Facts {
						knownFacts[f] = true
					}
				}
			}
		}
		for f := range c.Facts {
			if !knownFacts[f] {
				return bad("unknown criterion fact")
			}
		}
		seen[c.Code] = true
		if r.EngineID == "acmg_amp_2015" && !acmg2015.AllowedStrength(acmg2015.CriterionCode(c.Code), acmg2015.Strength(c.Strength)) {
			return bad("invalid criterion strength")
		}
		if r.EngineID == "svc_v4" {
			return bad("SVC criteria must be encoded through draft_assessment")
		}
	}
	if r.EngineID == "svc_v4" {
		if !r.Options.AllowExperimental {
			return bad("SVC requires allow_experimental")
		}
		if r.Package.Draft != nil {
			if r.Package.Draft.MOI != r.Context.Inheritance {
				return bad("draft MOI mismatch")
			}
			if _, err := svcv4.Evaluate(r.Package.Draft); err != nil {
				return err
			}
		}
	} else if r.Package.Draft != nil {
		return bad("draft assessment only applies to SVC")
	}
	if d := r.Package.Dosage; d != nil {
		if r.CNV == nil || d.Coverage != o {
			return bad("dosage query scope mismatch")
		}
		if d.ProteinCodingGenes != nil && *d.ProteinCodingGenes < 0 {
			return bad("invalid gene count")
		}
		for _, g := range d.Regions {
			if g.Assembly != o.Assembly || strings.TrimPrefix(g.Chrom, "chr") != strings.TrimPrefix(o.Chrom, "chr") || g.Start < 1 || g.End < g.Start || g.Version == "" || g.Name == "" {
				return bad("invalid dosage record")
			}
			for _, v := range []int{int(g.HIScore), int(g.TSScore)} {
				if v != -1 && v != 0 && v != 1 && v != 2 && v != 3 && v != 30 && v != 40 {
					return bad("invalid dosage score")
				}
			}
		}
	}
	if p := r.Package.Phenotype; p != nil {
		if !contains([]string{"", "caller_supplied", "clinician_verified", "extracted_pending_review"}, p.Input.Source) {
			return bad("invalid phenotype source")
		}
		seenTerms := map[string]bool{}
		for _, term := range p.Input.Terms {
			if !regexp.MustCompile(`^HP:[0-9]{7}$`).MatchString(term.ID) || seenTerms[term.ID] || !contains([]string{"", "present", "absent", "uncertain", "family_history", "inferred"}, term.Polarity) {
				return bad("invalid or duplicate phenotype observation")
			}
			seenTerms[term.ID] = true
		}
		for _, a := range p.Associations {
			if math.IsNaN(a.Frequency) || a.Frequency < 0 || a.Frequency > 1 {
				return bad("invalid phenotype frequency")
			}
		}
		for _, f := range p.Background {
			if f <= 0 || f > 1 || math.IsNaN(f) {
				return bad("invalid phenotype background")
			}
		}
	}
	for _, c := range r.Package.Criteria {
		if r.CNV != nil {
			if c.Score == nil {
				return bad("CNV criterion requires integer hundredths")
			}
			rules := cnvloss2020.Rules
			if r.EngineID == "cnv_gain_2020" {
				rules = cnvgain2020.Rules
			}
			for _, d := range cnvrubric.Parse(rules) {
				if d.Code == c.Code && (*c.Score < d.Min || *c.Score > d.Max) {
					return bad("CNV score outside clause bounds")
				}
			}
		}
	}
	declared := map[string]bool{}
	for _, s := range r.Package.Sources {
		declared[s.ID] = true
	}
	checkRefs := func(ids []string) error {
		seen := map[string]bool{}
		for _, id := range ids {
			if !declared[id] || seen[id] {
				return bad("unknown or duplicate source reference")
			}
			seen[id] = true
		}
		return nil
	}
	refs := [][]string{r.Package.DraftSourceIDs}
	for _, c := range r.Package.Criteria {
		refs = append(refs, c.SourceIDs)
	}
	if m := r.Package.Mechanism; m != nil {
		refs = append(refs, m.SourceIDs)
	}
	if d := r.Package.Dosage; d != nil {
		refs = append(refs, d.SourceIDs, d.GeneCountSourceIDs, d.FunctionalSourceIDs)
	}
	if p := r.Package.Phenotype; p != nil {
		refs = append(refs, p.SourceIDs, p.ClinicalSourceIDs)
	}
	for _, ids := range refs {
		if err := checkRefs(ids); err != nil {
			return err
		}
	}
	return nil
}

// Sources records the service-side policy decision, never a caller's verified flag.
func Sources(r Request, actor string, policies map[string]config.SourcePolicy, at time.Time) ([]evidence.Source, []string) {
	var out []evidence.Source
	var gaps []string
	for _, s := range r.Package.Sources {
		p, ok := policies[s.Dataset]
		days := p.MaxAgeDays
		if days == 0 {
			days = 90
		}
		t, _ := time.Parse(time.RFC3339, s.RetrievedAt)
		valid := ok && contains(p.Versions, s.Version) && contains(p.Actors, actor) && contains(p.Assemblies, s.Object.Assembly) && s.LicenseConfirmed && !t.After(at) && at.Sub(t) <= time.Duration(days)*24*time.Hour && (!s.Clinical || p.Clinical)
		x := evidence.Source{ID: s.ID, Type: evidence.SourceTypeExternalEvidence, Title: s.Dataset, DatasetVersion: s.Version, RetrievedAt: s.RetrievedAt, IdentifierValid: true, RecordExists: true, ObjectMatched: true, ValidationStatus: evidence.ValidationUnverified}
		if valid {
			x.MaterialVerified = true
			x.ValidationStatus = evidence.ValidationVerified
			x.VerifiedBy = actor
			x.VerifiedAt = at.Format(time.RFC3339)
			x.VerificationReason = "authenticated upstream attestation accepted by source policy; not independently fetched"
		} else {
			gaps = append(gaps, fmt.Sprintf("%s: source policy, licence or freshness not satisfied", s.ID))
		}
		out = append(out, x)
	}
	return out, gaps
}
func trusted(ids []string, sources []evidence.Source) bool {
	if len(ids) == 0 {
		return false
	}
	for _, id := range ids {
		found := false
		for _, s := range sources {
			found = found || (s.ID == id && s.ValidationStatus == evidence.ValidationVerified && s.MaterialVerified && s.ObjectMatched)
		}
		if !found {
			return false
		}
	}
	return true
}
func kindIDs(r Request, kind string) []string {
	var ids []string
	for _, s := range r.Package.Sources {
		if s.Kind == kind && !s.Clinical {
			ids = append(ids, s.ID)
		}
	}
	return ids
}
func clinical(ids []string, r Request, sources []evidence.Source) bool {
	if !trusted(ids, sources) {
		return false
	}
	for _, id := range ids {
		found := false
		for _, s := range r.Package.Sources {
			found = found || (s.ID == id && s.Clinical)
		}
		if !found {
			return false
		}
	}
	return true
}

// Normalize copies all caller-owned maps and slices before validation mutates defaults.
func Normalize(r Request) (Request, error) {
	b, e := json.Marshal(r)
	if e != nil {
		return r, e
	}
	var out Request
	e = json.Unmarshal(b, &out)
	if e == nil {
		e = Validate(&out)
	}
	if out.Package.Annotations != nil {
		resolver := annotation.NewResolver()
		resolver.MaxAgeDays = 0 // Freshness belongs to per-source policies.
		a, err := resolver.Normalize(out.Package.Annotations, ObjectFor(out).Assembly)
		if err != nil {
			return out, err
		}
		out.Package.Annotations = a
	}
	return out, e
}
