package knowledge

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pzweuj/sepilens/internal/annotation"
	"github.com/pzweuj/sepilens/internal/engine"
)

func TestAnnotationBundleImportTamperAndNormalization(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC().Truncate(time.Second)
	m := Manifest{Version: "synthetic-v2"}
	for _, f := range []struct{ kind, format, url, data string }{
		{"gnomad_tsv", "sepilens-gnomad-v1", "https://storage.googleapis.com/gcp-public-data--gnomad/synthetic/export.tsv", "chrom\tpos\tref\talt\tpopmax_af\tfaf95\tallele_number\tcoverage_adequate\n1\t100\tA\tG\t0.001\t0.00001\t10000\ttrue\n"},
		{"alphamissense_tsv", "alphamissense-2023", "https://storage.googleapis.com/dm_alphamissense/synthetic.tsv", "#metadata\n#CHROM\tPOS\tREF\tALT\ttranscript_id\tprotein_variant\tam_pathogenicity\tam_class\n1\t100\tA\tG\tTX\tp.Val1Glu\t0.99\tlikely_pathogenic\n"},
		{"gencc_tsv", "gencc-legacy-v1", "https://thegencc.org/synthetic.tsv", "gene_symbol\tdisease_curie\tclassification_title\tmoi_title\tsubmitter_title\nG\tOMIM:1\tDefinitive\tAutosomal dominant\ttest\n"},
		{"clinvar_snapshot", "clinvar-variant-summary-v1", "https://ftp.ncbi.nlm.nih.gov/pub/clinvar/synthetic.tsv", "#AlleleID\tVariationID\tAssembly\tChromosome\tPositionVCF\tReferenceAlleleVCF\tAlternateAlleleVCF\tGeneSymbol\tClinicalSignificance\tReviewStatus\tPhenotypeIDS\n1\t2\tGRCh38\t1\t100\tA\tG\tG\tPathogenic\treviewed_by_expert_panel\tOMIM:1\n"},
	} {
		name := f.kind + ".tsv"
		if e := os.WriteFile(filepath.Join(dir, name), []byte(f.data), 0600); e != nil {
			t.Fatal(e)
		}
		a := Asset{Path: name, Kind: f.kind, Format: f.format, URL: f.url, SHA256: engine.Hash([]byte(f.data)), Version: "synthetic", License: "test-only", LicenseConfirmed: true, RetrievedAt: now.Format(time.RFC3339), Complete: true}
		if f.kind != "gencc_tsv" {
			a.Assembly = "GRCh38"
		}
		m.Assets = append(m.Assets, a)
	}
	b, _ := json.Marshal(m)
	manifest := filepath.Join(dir, "manifest.json")
	if e := os.WriteFile(manifest, b, 0600); e != nil {
		t.Fatal(e)
	}
	if e := Import(context.Background(), manifest, dir, &http.Client{Transport: rejectTransport{t}}); e != nil {
		t.Fatal(e)
	}
	bundle, e := Load(dir)
	if e != nil {
		t.Fatal(e)
	}
	q := annotation.Query{Assembly: "GRCh38", Chrom: "1", Pos: 100, Ref: "A", Alt: "G", Gene: "G", DiseaseID: "OMIM:1", MOI: "AD", Transcript: "TX"}
	s, e := bundle.Annotations.Resolve(context.Background(), q, now)
	if e != nil {
		t.Fatal(e)
	}
	if s.GnomAD.Status != annotation.StatusFound || s.ClinVar.Status != annotation.StatusFound || s.AlphaMissense.Status != annotation.StatusFound || s.GenCC.Status != annotation.StatusFound {
		t.Fatal(s)
	}
	caller := *s
	caller.MarkCallerSupplied()
	normalized, e := annotation.NewResolver().Normalize(&caller, q.Assembly)
	if e != nil || *normalized.GnomAD.FAF95 != *s.GnomAD.FAF95 || normalized.GenCC.MOI != s.GenCC.MOI {
		t.Fatal("normalization differs", e)
	}
	indexPath := filepath.Join(dir, bundle.Manifest.AnnotationIndex.Path)
	bundle.Annotations.Close()
	if e = os.WriteFile(indexPath, []byte("tampered-index"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = Load(dir); e == nil {
		t.Fatal("tampered derived index loaded")
	}
}

func TestHPOTranslationMergesOnlyTranslatedFields(t *testing.T) {
	dir := t.TempDir()
	m := Manifest{Version: "synthetic"}
	files := []struct{ kind, format, url, data string }{
		{"hpo_obo", "", "https://raw.githubusercontent.com/obophenotype/human-phenotype-ontology/test/hp.obo", "[Term]\nid: HP:0000001\nname: All\n\n[Term]\nid: HP:0000002\nname: Official\nis_a: HP:0000001 ! All\n"},
		{"hpo_translation", "hpo-translation-v1", "https://raw.githubusercontent.com/pzweuj/DeepHPO/test/terms.json", `{"HP:0000002":{"id":"HP:0000002","name":"Do not overwrite official","name_cn":"中文","definition_cn":"翻译","parents":[]}}`},
		{"hpoa", "hpoa-v1", "https://raw.githubusercontent.com/obophenotype/human-phenotype-ontology/test/phenotype.hpoa", "#database_id\thpo_id\tqualifier\tfrequency\nOMIM:1\tHP:0000002\t\t\n"},
	}
	for n, f := range files {
		p := f.kind + ".txt"
		if e := os.WriteFile(filepath.Join(dir, p), []byte(f.data), 0600); e != nil {
			t.Fatal(e)
		}
		m.Assets = append(m.Assets, Asset{Path: p, Kind: f.kind, Format: f.format, URL: f.url, SHA256: engine.Hash([]byte(f.data)), Version: string(rune('a' + n)), License: "test", LicenseConfirmed: true, RetrievedAt: time.Now().UTC().Format(time.RFC3339)})
	}
	b, _ := json.Marshal(m)
	if e := os.WriteFile(filepath.Join(dir, "manifest.json"), b, 0600); e != nil {
		t.Fatal(e)
	}
	bundle, e := Load(dir)
	if e != nil {
		t.Fatal(e)
	}
	term := bundle.HPO["HP:0000002"]
	if !bundle.Graph.Complete() || term.Name != "Official" || term.NameCN != "中文" || len(term.Parents) != 1 || bundle.HPOVersions.TranslationVersion == "" || bundle.Phenotype == nil {
		t.Fatal(bundle, term)
	}
}

func TestMalformedAnnotationDoesNotPublishIndex(t *testing.T) {
	dir := t.TempDir()
	data := []byte("chrom\tpos\tref\talt\tpopmax_af\tfaf95\tallele_number\tcoverage_adequate\n1\t1\tA\tG\tNaN\t0\t10000\ttrue\n")
	a := Asset{Path: "bad.tsv", Kind: "gnomad_tsv", Format: "sepilens-gnomad-v1", URL: "https://storage.googleapis.com/gcp-public-data--gnomad/bad.tsv", Assembly: "GRCh38", SHA256: engine.Hash(data), Version: "test", License: "test", RetrievedAt: time.Now().UTC().Format(time.RFC3339)}
	m := Manifest{Version: "test", Assets: []Asset{a}}
	b, _ := json.Marshal(m)
	_ = os.WriteFile(filepath.Join(dir, "bad.tsv"), data, 0600)
	_ = os.WriteFile(filepath.Join(dir, "manifest.json"), b, 0600)
	if e := Import(context.Background(), filepath.Join(dir, "manifest.json"), dir, &http.Client{Transport: rejectTransport{t}}); e == nil {
		t.Fatal("invalid numeric input imported")
	}
	files, _ := filepath.Glob(filepath.Join(dir, "annotations-*.sqlite"))
	if len(files) > 0 {
		t.Fatal("invalid index published", files)
	}
}

func TestOBOIgnoresTypedefStanzas(t *testing.T) {
	p := filepath.Join(t.TempDir(), "hp.obo")
	data := []byte("format-version: 1.2\n[Term]\nid: HP:0000001\nname: All\n\n[Typedef]\nid: part_of\nname: must not replace a term\n")
	if e := os.WriteFile(p, data, 0600); e != nil {
		t.Fatal(e)
	}
	terms, e := parseOBO(p)
	if e != nil || len(terms) != 1 || terms["HP:0000001"].Name != "All" {
		t.Fatal(terms, e)
	}
}
