package knowledge

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/pzweuj/sepilens/internal/annotation"
	"github.com/pzweuj/sepilens/internal/engine"
)

func officialAssetURL(u *url.URL) bool {
	switch u.Hostname() {
	case "ftp.ncbi.nlm.nih.gov", "search.clinicalgenome.org", "ftp.clinicalgenome.org", "thegencc.org", "search.thegencc.org":
		return true
	case "storage.googleapis.com":
		return strings.HasPrefix(u.Path, "/dm_alphamissense/") || strings.HasPrefix(u.Path, "/gcp-public-data--gnomad/")
	case "raw.githubusercontent.com":
		return strings.HasPrefix(u.Path, "/obophenotype/human-phenotype-ontology/") || strings.HasPrefix(u.Path, "/monarch-initiative/hpoa/") || strings.HasPrefix(u.Path, "/pzweuj/DeepHPO/")
	case "github.com":
		return strings.HasPrefix(u.Path, "/obophenotype/human-phenotype-ontology/releases/")
	}
	return false
}
func validAnnotationFormat(a Asset) bool {
	return map[string]string{"hpoa": "hpoa-v1", "gencc_tsv": "gencc-legacy-v1", "alphamissense_tsv": "alphamissense-2023", "gnomad_tsv": "sepilens-gnomad-v1", "clinvar_snapshot": "clinvar-variant-summary-v1"}[a.Kind] == a.Format
}
func annotationKind(kind string) string {
	return map[string]string{"gencc_tsv": "gencc", "alphamissense_tsv": "alphamissense", "gnomad_tsv": "gnomad", "clinvar_snapshot": "clinvar"}[kind]
}
func hasAnnotationAssets(m Manifest) bool {
	for _, a := range m.Assets {
		if annotationKind(a.Kind) != "" {
			return true
		}
	}
	return false
}
func annotationBinding(m Manifest) string {
	m.AnnotationIndex = nil
	b, _ := json.Marshal(struct {
		Schema   string
		Manifest Manifest
	}{"annotation-index-v1", m})
	return engine.Hash(b)
}

func buildAnnotationIndex(ctx context.Context, dir string, m *Manifest) error {
	if !hasAnnotationAssets(*m) {
		return nil
	}
	hash := annotationBinding(*m)
	dest := filepath.Join(dir, "annotations-"+hash+".sqlite")
	if _, err := os.Stat(dest); err == nil {
		if m.AnnotationIndex == nil {
			return errors.New("unregistered annotation index already exists")
		}
		if m.AnnotationIndex != nil {
			if err = verify(dest, m.AnnotationIndex.SHA256); err != nil {
				return err
			}
		}
		idx, e := annotation.OpenIndex(dest, hash)
		if e == nil {
			idx.Close()
		}
		if e != nil {
			return e
		}
		return setDerivedIndex(m, dest, hash)
	}
	var assets []annotation.AssetMetadata
	seen := map[string]bool{}
	for _, a := range m.Assets {
		kind := annotationKind(a.Kind)
		if kind == "" {
			continue
		}
		key := kind + ":" + a.Assembly
		if seen[key] {
			return errors.New("duplicate annotation asset")
		}
		seen[key] = true
		assets = append(assets, annotation.AssetMetadata{Kind: kind, Version: a.Version, Assembly: a.Assembly, SHA256: a.SHA256, License: a.License, LicenseConfirmed: a.LicenseConfirmed, RetrievedAt: a.RetrievedAt, MaxAgeDays: a.MaxAgeDays, Complete: a.Complete})
	}
	err := annotation.BuildIndex(ctx, dest, hash, assets, func(emit func(annotation.IndexedRecord) error) error {
		for _, a := range m.Assets {
			if annotationKind(a.Kind) == "" {
				continue
			}
			p, e := path(dir, a.Path)
			if e != nil {
				return e
			}
			if e = verify(p, a.SHA256); e != nil {
				return e
			}
			if e = parseAnnotationTSV(p, a, emit); e != nil {
				return fmt.Errorf("invalid %s snapshot: %w", a.Kind, e)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return setDerivedIndex(m, dest, hash)
}

func setDerivedIndex(m *Manifest, p, hash string) error {
	f, e := os.Open(p)
	if e != nil {
		return e
	}
	defer f.Close()
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil {
		return e
	}
	m.AnnotationIndex = &DerivedIndex{Path: filepath.Base(p), SHA256: hex.EncodeToString(h.Sum(nil)), InputHash: hash}
	return nil
}

// readTSV streams rows and requires an explicit header. Comment metadata before
// the header is allowed; malformed rows never silently become absent records.
func readTSV(p string, required []string, emit func(map[string]string) error) error {
	f, err := reader(p)
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 4096), 4<<20)
	var header []string
	for sc.Scan() {
		line := strings.TrimSuffix(sc.Text(), "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		cols := strings.Split(strings.TrimPrefix(line, "#"), "\t")
		if header == nil {
			present := map[string]bool{}
			for _, h := range cols {
				present[h] = true
			}
			ok := true
			for _, h := range required {
				ok = ok && present[h]
			}
			if !ok {
				if strings.HasPrefix(line, "#") {
					continue
				}
				return errors.New("required TSV columns missing")
			}
			header = cols
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		if len(cols) != len(header) {
			return errors.New("TSV column count mismatch")
		}
		row := map[string]string{}
		for n, h := range header {
			row[h] = cols[n]
		}
		if err = emit(row); err != nil {
			return err
		}
	}
	if err = sc.Err(); err != nil {
		return err
	}
	if header == nil {
		return errors.New("TSV header missing")
	}
	return nil
}

func parseAnnotationTSV(p string, a Asset, emit func(annotation.IndexedRecord) error) error {
	kind := annotationKind(a.Kind)
	required := map[string][]string{
		"gencc":         {"gene_symbol", "disease_curie", "classification_title", "moi_title", "submitter_title"},
		"alphamissense": {"CHROM", "POS", "REF", "ALT", "transcript_id", "protein_variant", "am_pathogenicity", "am_class"},
		"gnomad":        {"chrom", "pos", "ref", "alt", "popmax_af", "faf95", "allele_number", "coverage_adequate"},
		"clinvar":       {"AlleleID", "VariationID", "Assembly", "Chromosome", "PositionVCF", "ReferenceAlleleVCF", "AlternateAlleleVCF", "GeneSymbol", "ClinicalSignificance", "ReviewStatus", "PhenotypeIDS"},
	}[kind]
	return readTSV(p, required, func(r map[string]string) error {
		q := annotation.Query{Assembly: a.Assembly}
		rec := annotation.Record{Source: annotation.SourceSnapshot, Version: a.Version, Assembly: a.Assembly, RetrievedAt: a.RetrievedAt, Status: annotation.StatusFound}
		var payload any
		switch kind {
		case "gencc":
			q.Gene = r["gene_symbol"]
			q.DiseaseID = r["disease_curie"]
			q.MOI = normalizeMOI(r["moi_title"])
			if q.Gene == "" || q.DiseaseID == "" {
				return errors.New("missing gene or disease")
			}
			payload = annotation.GenCC{Record: rec, Gene: q.Gene, DiseaseID: q.DiseaseID, DiseaseName: r["disease_title"], MOI: q.MOI, Classification: r["classification_title"], Submitter: r["submitter_title"]}
		case "alphamissense":
			q.Chrom = r["CHROM"]
			q.Ref = r["REF"]
			q.Alt = r["ALT"]
			q.Transcript = r["transcript_id"]
			q.Protein = r["protein_variant"]
			var err error
			q.Pos, err = strconv.ParseInt(r["POS"], 10, 64)
			if err != nil {
				return errors.New("invalid position")
			}
			s, err := strconv.ParseFloat(r["am_pathogenicity"], 64)
			if err != nil {
				return errors.New("invalid prediction score")
			}
			payload = annotation.AlphaMissense{Record: rec, Score: &s, Class: r["am_class"], Transcript: q.Transcript, Protein: q.Protein}
		case "gnomad":
			q.Chrom = r["chrom"]
			q.Ref = r["ref"]
			q.Alt = r["alt"]
			var err error
			q.Pos, err = strconv.ParseInt(r["pos"], 10, 64)
			if err != nil {
				return errors.New("invalid position")
			}
			g := annotation.GnomAD{Record: rec}
			for name, dest := range map[string]**float64{"popmax_af": &g.PopMaxAF, "faf95": &g.FAF95} {
				if r[name] != "" && r[name] != "." {
					n, e := strconv.ParseFloat(r[name], 64)
					if e != nil {
						return errors.New("invalid frequency")
					}
					*dest = &n
				}
			}
			an, e := strconv.Atoi(r["allele_number"])
			if e != nil || an < 0 {
				return errors.New("invalid allele number")
			}
			g.AlleleNumber = &an
			c, e := strconv.ParseBool(r["coverage_adequate"])
			if e != nil {
				return errors.New("invalid coverage flag")
			}
			g.CoverageAdequate = &c
			payload = g
		case "clinvar":
			if r["Assembly"] != a.Assembly {
				return nil
			}
			q.Chrom = r["Chromosome"]
			q.Ref = r["ReferenceAlleleVCF"]
			q.Alt = r["AlternateAlleleVCF"]
			q.Gene = r["GeneSymbol"]
			if q.Ref == "na" || q.Alt == "na" || r["PositionVCF"] == "-1" {
				return nil
			}
			var err error
			q.Pos, err = strconv.ParseInt(r["PositionVCF"], 10, 64)
			if err != nil {
				return errors.New("invalid ClinVar VCF position")
			}
			if tx := regexp.MustCompile(`\b(NM_[0-9]+\.[0-9]+)\b`).FindStringSubmatch(r["Name"]); len(tx) > 1 {
				q.Transcript = tx[1]
			}
			if protein := regexp.MustCompile(`\((p\.[^)]+)\)`).FindStringSubmatch(r["Name"]); len(protein) > 1 {
				q.Protein = protein[1]
			}
			payload = annotation.ClinVar{Record: rec, VariationID: r["VariationID"], AlleleID: r["AlleleID"], Classification: r["ClinicalSignificance"], ReviewStatus: r["ReviewStatus"], Transcript: q.Transcript, HGVSp: q.Protein, Conditions: strings.FieldsFunc(r["PhenotypeIDS"], func(c rune) bool { return c == '|' || c == ',' || c == ';' }), Conflicting: strings.Contains(strings.ToLower(r["ClinicalSignificance"]), "conflict")}
		}
		b, err := json.Marshal(payload)
		if err != nil {
			return errors.New("invalid annotation values")
		}
		if kind != "gencc" {
			if q.Pos < 1 || q.Chrom == "" || q.Ref == "" || q.Alt == "" {
				return errors.New("missing variant coordinates")
			}
		}
		// Reuse the annotation model's numeric and assembly validation.
		s := &annotation.Set{}
		switch kind {
		case "gencc":
			s.GenCC = &annotation.GenCC{}
			err = json.Unmarshal(b, s.GenCC)
		case "alphamissense":
			s.AlphaMissense = &annotation.AlphaMissense{}
			err = json.Unmarshal(b, s.AlphaMissense)
		case "gnomad":
			s.GnomAD = &annotation.GnomAD{}
			err = json.Unmarshal(b, s.GnomAD)
		case "clinvar":
			s.ClinVar = &annotation.ClinVar{}
			err = json.Unmarshal(b, s.ClinVar)
		}
		if err != nil {
			return err
		}
		if err = s.Validate(a.Assembly); err != nil {
			return err
		}
		return emit(annotation.IndexedRecord{Kind: kind, Query: q, Data: b})
	})
}
func normalizeMOI(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "autosomal dominant":
		return "AD"
	case "autosomal recessive":
		return "AR"
	case "x-linked dominant":
		return "XLD"
	case "x-linked recessive":
		return "XLR"
	}
	return s
}
