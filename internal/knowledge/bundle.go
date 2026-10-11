package knowledge

import (
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/pzweuj/sepilens/internal/engine"
	"github.com/pzweuj/sepilens/internal/hpo"
	"github.com/pzweuj/sepilens/internal/variant/cnv"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Asset struct {
	Path          string `json:"path"`
	URL           string `json:"url"`
	SHA256        string `json:"sha256"`
	Version       string `json:"version"`
	License       string `json:"license"`
	Kind          string `json:"kind"`
	Assembly      string `json:"assembly,omitempty"`
	Complete      bool   `json:"complete,omitempty"`
	RetrievedAt   string `json:"retrieved_at,omitempty"`
	DerivedPath   string `json:"derived_path,omitempty"`
	DerivedSHA256 string `json:"derived_sha256,omitempty"`
}
type Manifest struct {
	Version string  `json:"version"`
	Assets  []Asset `json:"assets"`
}
type Bundle struct {
	Manifest   Manifest
	Hash       string
	Regions    []cnv.DosageRegion
	Complete   []string
	Dir        string
	HPO        map[string]hpo.Term
	References map[string]*Reference
}

func path(dir, name string) (string, error) {
	if name == "" || strings.HasPrefix(name, "/") || filepath.IsAbs(name) || strings.Contains(name, "\\") || strings.Contains(name, ":") {
		return "", errors.New("asset path must be relative")
	}
	clean := filepath.Clean(name)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("asset path escapes bundle")
	}
	p := filepath.Join(dir, clean)
	parent := filepath.Dir(p)
	for {
		if info, e := os.Lstat(parent); e == nil && info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("symlinked asset directory rejected")
		}
		if parent == dir || parent == filepath.Dir(parent) {
			break
		}
		parent = filepath.Dir(parent)
	}
	if info, e := os.Lstat(p); e == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("symlinked asset rejected")
	}
	return p, nil
}
func ReadManifest(file string) (Manifest, []byte, error) {
	b, e := os.ReadFile(file)
	var m Manifest
	if e != nil {
		return m, b, e
	}
	d := json.NewDecoder(strings.NewReader(string(b)))
	d.DisallowUnknownFields()
	if e = d.Decode(&m); e != nil {
		return m, b, e
	}
	if m.Version == "" || len(m.Assets) == 0 {
		return m, b, errors.New("versioned assets required")
	}
	seen := map[string]bool{}
	for _, a := range m.Assets {
		u, e := url.Parse(a.URL)
		official := e == nil && u.Scheme == "https" && (u.Hostname() == "ftp.ncbi.nlm.nih.gov" || u.Hostname() == "search.clinicalgenome.org" || u.Hostname() == "ftp.clinicalgenome.org" || (u.Hostname() == "raw.githubusercontent.com" && strings.HasPrefix(u.Path, "/obophenotype/human-phenotype-ontology/")))
		if !official || u.User != nil || u.Fragment != "" || a.Version == "" || a.License == "" || len(a.SHA256) != 64 || seen[a.Path] {
			return m, b, errors.New("asset requires official HTTPS source, version, checksum and license")
		}
		if _, e = hex.DecodeString(a.SHA256); e != nil {
			return m, b, e
		}
		switch a.Kind {
		case "hpo_obo", "refseq_gff", "clingen_tsv", "reference_fasta":
		default:
			return m, b, errors.New("unsupported asset kind")
		}
		if a.Kind != "hpo_obo" && a.Assembly != "GRCh37" && a.Assembly != "GRCh38" {
			return m, b, errors.New("assembly required")
		}
		seen[a.Path] = true
		if a.RetrievedAt != "" {
			if _, e := time.Parse(time.RFC3339, a.RetrievedAt); e != nil {
				return m, b, errors.New("invalid source access date")
			}
		}
	}
	return m, b, nil
}
func Import(ctx context.Context, manifest, dir string, client *http.Client) error {
	m, b, e := ReadManifest(manifest)
	if e != nil {
		return e
	}
	if e = os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	for _, a := range m.Assets {
		p, e := path(dir, a.Path)
		if e != nil {
			return e
		}
		if e = os.MkdirAll(filepath.Dir(p), 0700); e != nil {
			return e
		}
		if _, e = os.Stat(p); e == nil {
			if e = verify(p, a.SHA256); e != nil {
				return e
			}
			continue
		}
		req, e := http.NewRequestWithContext(ctx, "GET", a.URL, nil)
		if e != nil {
			return e
		}
		res, e := client.Do(req)
		if e != nil {
			return errors.New("official asset download failed")
		}
		if res.StatusCode != 200 {
			res.Body.Close()
			return fmt.Errorf("asset download HTTP %d", res.StatusCode)
		}
		tmp := p + ".partial"
		f, e := os.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			res.Body.Close()
			return e
		}
		h := sha256.New()
		_, e = io.Copy(io.MultiWriter(f, h), io.LimitReader(res.Body, 8<<30))
		res.Body.Close()
		syncErr := f.Sync()
		f.Close()
		if e != nil || syncErr != nil || hex.EncodeToString(h.Sum(nil)) != a.SHA256 {
			os.Remove(tmp)
			return errors.New("asset checksum or download failed")
		}
		if e = os.Rename(tmp, p); e != nil {
			return e
		}
	}
	for i := range m.Assets {
		if m.Assets[i].RetrievedAt == "" {
			m.Assets[i].RetrievedAt = time.Now().UTC().Format(time.RFC3339)
		}
	}
	for i, a := range m.Assets {
		if a.Kind == "reference_fasta" && strings.HasSuffix(a.Path, ".gz") {
			p, e := path(dir, a.Path)
			if e != nil {
				return e
			}
			a.DerivedPath = a.Path + ".fna"
			dest, e := path(dir, a.DerivedPath)
			if e != nil {
				return e
			}
			input, e := reader(p)
			if e != nil {
				return e
			}
			file, e := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
			if e != nil {
				input.Close()
				return e
			}
			h := sha256.New()
			_, e = io.Copy(io.MultiWriter(file, h), input)
			input.Close()
			syncErr := file.Sync()
			file.Close()
			if e != nil {
				return e
			}
			if syncErr != nil {
				return syncErr
			}
			a.DerivedSHA256 = hex.EncodeToString(h.Sum(nil))
			m.Assets[i] = a
		}
	}
	b, e = json.MarshalIndent(m, "", "  ")
	if e != nil {
		return e
	}
	if e = os.WriteFile(filepath.Join(dir, "manifest.json"), b, 0600); e != nil {
		return e
	}
	bundle, e := Load(dir)
	if e != nil {
		return e
	}
	for _, a := range m.Assets {
		if a.Kind == "hpo_obo" {
			p, _ := path(dir, a.Path)
			terms, e := parseOBO(p)
			if e != nil {
				return e
			}
			b, _ := json.Marshal(terms)
			if e = os.MkdirAll(filepath.Join(dir, "hpo"), 0700); e != nil {
				return e
			}
			if e = os.WriteFile(filepath.Join(dir, "hpo", "hpo_terms_cn.json"), b, 0600); e != nil {
				return e
			}
			var names []string
			for _, term := range terms {
				names = append(names, term.Name)
			}
			sort.Strings(names)
			if e = os.WriteFile(filepath.Join(dir, "hpo", "hpo_names.txt"), []byte(strings.Join(names, "\n")), 0600); e != nil {
				return e
			}
		}
	}
	_ = bundle
	return nil
}
func verify(p, want string) error {
	f, e := os.Open(p)
	if e != nil {
		return e
	}
	defer f.Close()
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil {
		return e
	}
	if hex.EncodeToString(h.Sum(nil)) != want {
		return errors.New("asset checksum mismatch")
	}
	return nil
}
func reader(p string) (io.ReadCloser, error) {
	f, e := os.Open(p)
	if e != nil {
		return nil, e
	}
	if strings.HasSuffix(p, ".gz") {
		g, e := gzip.NewReader(f)
		if e != nil {
			f.Close()
			return nil, e
		}
		return &compressed{g, f}, nil
	}
	return f, nil
}

type compressed struct {
	*gzip.Reader
	f *os.File
}

func (c *compressed) Close() error { c.Reader.Close(); return c.f.Close() }
func Load(dir string) (*Bundle, error) {
	m, b, e := ReadManifest(filepath.Join(dir, "manifest.json"))
	if e != nil {
		return nil, e
	}
	r := &Bundle{Manifest: m, Hash: engine.Hash(b), Dir: dir, References: map[string]*Reference{}}
	coverage := map[string]map[string]bool{}
	for _, a := range m.Assets {
		p, e := path(dir, a.Path)
		if e != nil {
			return nil, e
		}
		if e = verify(p, a.SHA256); e != nil {
			return nil, e
		}
		switch a.Kind {
		case "hpo_obo":
			terms, e := parseOBO(p)
			if e != nil {
				return nil, e
			}
			r.HPO = terms
		case "reference_fasta":
			if strings.HasSuffix(p, ".gz") {
				if a.DerivedPath == "" || a.DerivedSHA256 == "" {
					return nil, errors.New("compressed reference must be imported")
				}
				p, e = path(dir, a.DerivedPath)
				if e != nil {
					return nil, e
				}
				if e = verify(p, a.DerivedSHA256); e != nil {
					return nil, e
				}
			}
			ref, e := openReference(p)
			if e != nil {
				return nil, e
			}
			r.References[a.Assembly] = ref
		case "refseq_gff":
			regions, e := parseGFF(p, a)
			if e != nil {
				return nil, e
			}
			r.Regions = append(r.Regions, regions...)
		case "clingen_tsv":
			regions, e := parseDosage(p, a)
			if e != nil {
				return nil, e
			}
			r.Regions = append(r.Regions, regions...)
		}
		if coverage[a.Assembly] == nil {
			coverage[a.Assembly] = map[string]bool{}
		}
		if a.Complete {
			coverage[a.Assembly][a.Kind] = true
		}
	}
	for asm, kinds := range coverage {
		if kinds["refseq_gff"] && kinds["clingen_tsv"] && r.References[asm] != nil {
			r.Complete = append(r.Complete, asm)
		}
	}
	sort.Strings(r.Complete)
	return r, nil
}
func attrs(s string) map[string]string {
	r := map[string]string{}
	for _, p := range strings.Split(s, ";") {
		kv := strings.SplitN(p, "=", 2)
		if len(kv) == 2 {
			x, _ := url.QueryUnescape(kv[1])
			r[kv[0]] = x
		}
	}
	return r
}
func parseGFF(p string, a Asset) ([]cnv.DosageRegion, error) {
	f, e := reader(p)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 4096), 2<<20)
	seq := map[string]string{}
	genes := map[string]int{}
	parents := map[string]string{}
	var out []cnv.DosageRegion
	for sc.Scan() {
		l := sc.Text()
		if strings.HasPrefix(l, "#") {
			continue
		}
		c := strings.Split(l, "\t")
		if len(c) != 9 {
			continue
		}
		at := attrs(c[8])
		start, e := strconv.ParseInt(c[3], 10, 64)
		if e != nil {
			return nil, e
		}
		end, e := strconv.ParseInt(c[4], 10, 64)
		if e != nil || start < 1 || end < start {
			return nil, errors.New("invalid GFF coordinates")
		}
		if c[2] == "region" && at["chromosome"] != "" {
			seq[c[0]] = at["chromosome"]
		}
		chrom := seq[c[0]]
		if chrom == "" {
			chrom = c[0]
		}
		switch c[2] {
		case "gene":
			id := at["ID"]
			name := at["Name"]
			if name == "" {
				name = id
			}
			genes[id] = len(out)
			out = append(out, cnv.DosageRegion{Name: name, Assembly: a.Assembly, Chrom: chrom, Start: start, End: end, IsGene: true, ProteinCoding: at["gene_biotype"] == "protein_coding", Strand: c[6], Version: a.Version})
		case "mRNA", "transcript", "ncRNA":
			parents[at["ID"]] = at["Parent"]
		case "exon":
			for _, parent := range strings.Split(at["Parent"], ",") {
				gene := parents[parent]
				if gene == "" {
					gene = parent
				}
				if i, ok := genes[gene]; ok {
					out[i].Exons = append(out[i].Exons, cnv.Range{Start: start, End: end})
				}
			}
		}
	}
	if len(out) == 0 {
		return nil, errors.New("RefSeq file contains no genes")
	}
	return out, sc.Err()
}
func parseDosage(p string, a Asset) ([]cnv.DosageRegion, error) {
	f, e := reader(p)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 4096), 2<<20)
	var headers []string
	var out []cnv.DosageRegion
	for sc.Scan() {
		line := sc.Text()
		if headers == nil {
			if strings.Contains(line, "Haploinsufficiency") && strings.Contains(line, "Triplosensitivity") {
				headers = strings.Split(strings.TrimPrefix(line, "#"), "\t")
			}
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		cols := strings.Split(line, "\t")
		get := func(fragment string) string {
			for i, h := range headers {
				if strings.Contains(strings.ToLower(h), strings.ToLower(fragment)) && i < len(cols) {
					return cols[i]
				}
			}
			return ""
		}
		location := get(a.Assembly + " Location")
		if location == "" {
			location = get("Genomic Location")
		}
		loc := strings.SplitN(location, ":", 2)
		if len(loc) != 2 {
			continue
		}
		ends := strings.SplitN(loc[1], "-", 2)
		if len(ends) != 2 {
			continue
		}
		start, e := strconv.ParseInt(strings.ReplaceAll(ends[0], ",", ""), 10, 64)
		if e != nil {
			return nil, e
		}
		end, e := strconv.ParseInt(strings.ReplaceAll(ends[1], ",", ""), 10, 64)
		if e != nil || start < 1 || end < start {
			return nil, errors.New("invalid dosage coordinates")
		}
		parseScore := func(v string) (int, error) {
			if v == "" || v == "Not yet evaluated" {
				return -1, nil
			}
			n, e := strconv.Atoi(v)
			if e != nil {
				return 0, errors.New("invalid dosage score")
			}
			switch n {
			case 0, 1, 2, 3, 30, 40:
				return n, nil
			}
			return 0, errors.New("unknown dosage score")
		}
		hi, e := parseScore(get("Haploinsufficiency Score"))
		if e != nil {
			return nil, e
		}
		ts, e := parseScore(get("Triplosensitivity Score"))
		if e != nil {
			return nil, e
		}
		name := get("Gene Symbol")
		if name == "" {
			name = get("ISCA Region Name")
		}
		out = append(out, cnv.DosageRegion{Name: name, Assembly: a.Assembly, Chrom: loc[0], Start: start, End: end, HIScore: cnv.DosageScore(hi), TSScore: cnv.DosageScore(ts), Version: a.Version})
	}
	if headers == nil {
		return nil, errors.New("unsupported ClinGen TSV columns")
	}
	return out, sc.Err()
}
func parseOBO(p string) (map[string]hpo.Term, error) {
	f, e := reader(p)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	terms := map[string]hpo.Term{}
	var term hpo.Term
	flush := func() {
		if term.ID != "" {
			terms[term.ID] = term
		}
		term = hpo.Term{}
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 4096), 2<<20)
	for sc.Scan() {
		l := sc.Text()
		if l == "[Term]" {
			flush()
			continue
		}
		kv := strings.SplitN(l, ": ", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "id":
			term.ID = kv[1]
		case "name":
			term.Name = kv[1]
		case "def":
			term.Definition = kv[1]
		case "is_obsolete":
			term.Obsolete = kv[1] == "true"
		case "replaced_by":
			term.ReplacedBy = kv[1]
		case "synonym":
			if strings.HasPrefix(kv[1], "\"") {
				end := strings.LastIndex(kv[1], "\"")
				if end > 0 {
					syn, e := strconv.Unquote(kv[1][:end+1])
					if e != nil {
						return nil, e
					}
					term.Synonyms = append(term.Synonyms, syn)
				}
			}
		}
	}
	flush()
	return terms, sc.Err()
}
