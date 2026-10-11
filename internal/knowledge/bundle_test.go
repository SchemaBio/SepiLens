package knowledge

import (
	"context"
	"encoding/json"
	"github.com/pzweuj/sepilens/internal/engine"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func syntheticBundle(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := []struct {
		path, kind, data, url string
		complete              bool
	}{{"hpo.obo", "hpo_obo", "format-version: 1.2\n[Term]\nid: HP:0000001\nname: Test phenotype\nsynonym: \"Test synonym\" EXACT []\n\n[Term]\nid: HP:0000002\nname: Obsolete term\nis_obsolete: true\nreplaced_by: HP:0000001\n", "https://raw.githubusercontent.com/obophenotype/human-phenotype-ontology/vtest/hp.obo", false}, {"genes.gff", "refseq_gff", "##gff-version 3\nNC_000001.11\tRefSeq\tregion\t1\t12\t.\t+\t.\tchromosome=1\nNC_000001.11\tRefSeq\tgene\t2\t8\t.\t+\t.\tID=gene-G;Name=G;gene_biotype=protein_coding\nNC_000001.11\tRefSeq\tmRNA\t2\t8\t.\t+\t.\tID=tx;Parent=gene-G\nNC_000001.11\tRefSeq\texon\t2\t5\t.\t+\t.\tID=exon;Parent=tx\n", "https://ftp.ncbi.nlm.nih.gov/genomes/synthetic/genes.gff", true}, {"dosage.tsv", "clingen_tsv", "#Gene Symbol\tGRCh38 Location\tHaploinsufficiency Score\tTriplosensitivity Score\nG\tchr1:2-8\t3\t0\n", "https://ftp.clinicalgenome.org/synthetic/dosage.tsv", true}, {"reference.fna", "reference_fasta", ">NC_000001.11\nACGTAC\nGTACGT\n", "https://ftp.ncbi.nlm.nih.gov/genomes/synthetic/reference.fna", true}}
	manifest := Manifest{Version: "synthetic-offline-test"}
	for _, f := range files {
		p := filepath.Join(dir, f.path)
		if e := os.WriteFile(p, []byte(f.data), 0600); e != nil {
			t.Fatal(e)
		}
		a := Asset{Path: f.path, Kind: f.kind, URL: f.url, Version: "synthetic", License: "test-only-no-redistribution", SHA256: engine.Hash([]byte(f.data)), Complete: f.complete}
		if f.kind != "hpo_obo" {
			a.Assembly = "GRCh38"
		}
		manifest.Assets = append(manifest.Assets, a)
	}
	b, _ := json.Marshal(manifest)
	if e := os.WriteFile(filepath.Join(dir, "manifest.json"), b, 0600); e != nil {
		t.Fatal(e)
	}
	return dir
}
func TestLockedOfflineBundleAndReference(t *testing.T) {
	dir := syntheticBundle(t)
	bundle, e := Load(dir)
	if e != nil {
		t.Fatal(e)
	}
	if len(bundle.Complete) != 1 || len(bundle.Regions) != 2 || len(bundle.Regions[0].Exons) != 1 || bundle.HPO["HP:0000002"].ReplacedBy != "HP:0000001" || bundle.HPO["HP:0000001"].Synonyms[0] != "Test synonym" {
		t.Fatalf("invalid bundle %+v", bundle)
	}
	seq, e := bundle.References["GRCh38"].Bases("chr1", 5, 5)
	if e != nil || seq != "ACGTA" {
		t.Fatal(seq, e)
	}
	if _, e = bundle.References["GRCh38"].Bases("1", 12, 2); e == nil {
		t.Fatal("out of bounds reference accepted")
	}
	if e = os.WriteFile(filepath.Join(dir, "genes.gff"), []byte("tampered"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = Load(dir); e == nil {
		t.Fatal("tampered dataset loaded")
	}
}
func TestNoNetworkForAlreadyImportedAssets(t *testing.T) {
	dir := syntheticBundle(t)
	client := &http.Client{Transport: rejectTransport{t}}
	if e := Import(context.Background(), filepath.Join(dir, "manifest.json"), dir, client); e != nil {
		t.Fatal(e)
	}
}

type rejectTransport struct{ t *testing.T }

func (r rejectTransport) RoundTrip(*http.Request) (*http.Response, error) {
	r.t.Fatal("offline import used network")
	return nil, nil
}
func TestPathsAndIncompleteCoverage(t *testing.T) {
	dir := syntheticBundle(t)
	for _, p := range []string{"../outside", "C:/outside", "/outside", "folder\\outside"} {
		if _, e := path(dir, p); e == nil {
			t.Fatal("unsafe asset path", p)
		}
	}
	b, e := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if e != nil {
		t.Fatal(e)
	}
	b = []byte(strings.ReplaceAll(string(b), `"complete":true`, `"complete":false`))
	if e = os.WriteFile(filepath.Join(dir, "manifest.json"), b, 0600); e != nil {
		t.Fatal(e)
	}
	bundle, e := Load(dir)
	if e != nil || len(bundle.Complete) != 0 {
		t.Fatal("incomplete data claims coverage", e)
	}
}
