package retrieval

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"github.com/pzweuj/sepilens/internal/evidence"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type budgetKey struct{}
type Budget struct {
	mu        sync.Mutex
	Max, Used int
	Last      time.Time
}

func WithBudget(ctx context.Context, b *Budget) context.Context {
	return context.WithValue(ctx, budgetKey{}, b)
}
func (b *Budget) Count() int { b.mu.Lock(); defer b.mu.Unlock(); return b.Used }

func (b *Budget) Take(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if b.Used >= b.Max {
		return ErrBudgetExceeded
	}
	wait := time.Until(b.Last.Add(350 * time.Millisecond))
	if wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
	b.Used++
	b.Last = time.Now()
	return nil
}

type OfficialRetriever struct {
	Database string
	HTTP     *http.Client
	BaseURL  string
}

func (r *OfficialRetriever) Name() string   { return r.Database }
func (r *OfficialRetriever) IsOnline() bool { return true }
func (r *OfficialRetriever) get(ctx context.Context, path string, params url.Values) ([]byte, error) {
	if b, ok := ctx.Value(budgetKey{}).(*Budget); ok {
		if e := b.Take(ctx); e != nil {
			return nil, e
		}
	}
	base := r.BaseURL
	if base == "" {
		base = "https://eutils.ncbi.nlm.nih.gov/entrez/eutils/"
	}
	req, e := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(base, "/")+"/"+path+"?"+params.Encode(), nil)
	if e != nil {
		return nil, e
	}
	req.Header.Set("User-Agent", "SepiLens/0.2 (evidence verification)")
	res, e := r.HTTP.Do(req)
	if e != nil {
		return nil, errors.New("official retrieval unavailable")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("official retrieval HTTP %d", res.StatusCode)
	}
	return io.ReadAll(io.LimitReader(res.Body, 8<<20))
}
func (r *OfficialRetriever) Retrieve(ctx context.Context, q Query) ([]evidence.Source, error) {
	out, err := r.retrieveOnce(ctx, q)
	if err != nil || q.Mode != ModeDeep {
		return out, err
	}
	if len(out) == 0 && q.HGVS != "" && q.Gene != "" {
		// Broaden an empty exact query without asserting object equivalence.
		broader := q
		broader.HGVS = ""
		out, err = r.retrieveOnce(ctx, broader)
		if err != nil {
			return out, err
		}
	}
	if r.Database == "pubmed" && len(out) > 0 {
		b, e := r.get(ctx, "elink.fcgi", url.Values{"dbfrom": {"pubmed"}, "db": {"pubmed"}, "id": {out[0].PMID}, "linkname": {"pubmed_pubmed_refs"}, "retmode": {"xml"}})
		if e != nil {
			return out, e
		}
		var links struct {
			IDs []string `xml:"LinkSet>LinkSetDb>Link>Id"`
		}
		if xml.Unmarshal(b, &links) != nil {
			return out, errors.New("malformed reference links")
		}
		if len(links.IDs) > 3 {
			links.IDs = links.IDs[:3]
		}
		if len(links.IDs) > 0 {
			linked := q
			linked.IDs = links.IDs
			linked.Mode = ModeTargeted // One-hop citation traversal stays bounded.
			more, e := r.retrieveOnce(ctx, linked)
			out = append(out, more...)
			if e != nil {
				return out, e
			}
		}
	}
	return out, nil
}

func (r *OfficialRetriever) retrieveOnce(ctx context.Context, q Query) ([]evidence.Source, error) {
	if r.Database != "pubmed" && r.Database != "clinvar" {
		return nil, errors.New("unsupported official database")
	}
	if q.Gene == "" && q.HGVS == "" && q.Chrom == "" {
		return nil, nil
	}
	if r.Database == "pubmed" && q.Gene == "" && q.HGVS == "" && len(q.IDs) == 0 && !(strings.HasPrefix(q.VariantType, "cnv_") && q.Start > 0 && q.End >= q.Start) {
		return nil, nil
	}
	term := strings.TrimSpace(q.Gene + " " + q.HGVS)
	if r.Database == "pubmed" {
		switch q.Purpose {
		case "functional_evidence":
			term += " functional assay splicing"
		case "gene_disease_validity":
			term += " gene disease inheritance"
		case "case_segregation":
			term += " case segregation inheritance"
		case "dosage_sensitivity":
			if q.VariantType == "cnv_loss" {
				term += " haploinsufficiency"
			} else if q.VariantType == "cnv_gain" {
				term += " triplosensitivity"
			}
		}
	}
	if r.Database == "pubmed" && strings.HasPrefix(q.VariantType, "cnv_") && q.Start > 0 && q.End >= q.Start {
		event := "duplication"
		if q.VariantType == "cnv_loss" {
			event = "deletion"
		}
		term = strings.TrimSpace(term + fmt.Sprintf(" chromosome %s %s %d %d", strings.TrimPrefix(q.Chrom, "chr"), event, q.Start, q.End))
	}
	if r.Database == "clinvar" {
		position := fmt.Sprintf("%d", q.Pos)
		if q.Start > 0 && q.End >= q.Start {
			position = fmt.Sprintf("%d:%d", q.Start, q.End)
		}
		if q.Assembly == "GRCh38" {
			term += fmt.Sprintf(" AND %s[chr] AND %s[chrpos38]", strings.TrimPrefix(q.Chrom, "chr"), position)
		} else {
			term += fmt.Sprintf(" AND %s[chr] AND %s[chrpos37]", strings.TrimPrefix(q.Chrom, "chr"), position)
		}
	}
	var b []byte
	var e error
	var search struct {
		Result struct {
			IDs []string `json:"idlist"`
		} `json:"esearchresult"`
	}
	search.Result.IDs = q.IDs
	if len(q.IDs) == 0 {
		b, e = r.get(ctx, "esearch.fcgi", url.Values{"db": {r.Database}, "term": {term}, "retmode": {"json"}, "retmax": {"5"}})
		if e != nil {
			return nil, e
		}
		if json.Unmarshal(b, &search) != nil {
			return nil, errors.New("malformed official search")
		}
	}
	if len(search.Result.IDs) == 0 {
		return nil, nil
	}
	b, e = r.get(ctx, "efetch.fcgi", url.Values{"db": {r.Database}, "id": {strings.Join(search.Result.IDs, ",")}, "retmode": {"xml"}})
	if e != nil {
		return nil, e
	}
	if r.Database == "pubmed" {
		var records struct {
			Articles []struct {
				Citation struct {
					PMID    string `xml:"PMID"`
					Article struct {
						Title    string `xml:"ArticleTitle"`
						Abstract struct {
							Text []string `xml:"AbstractText"`
						} `xml:"Abstract"`
					} `xml:"Article"`
				} `xml:"MedlineCitation"`
				Data struct {
					IDs []struct {
						Type  string `xml:"IdType,attr"`
						Value string `xml:",chardata"`
					} `xml:"ArticleIdList>ArticleId"`
				} `xml:"PubmedData"`
			} `xml:"PubmedArticle"`
		}
		if xml.Unmarshal(b, &records) != nil {
			return nil, errors.New("malformed PubMed record")
		}
		var out []evidence.Source
		for _, a := range records.Articles {
			c := a.Citation
			src := evidence.Source{ID: "PMID-" + c.PMID, Type: evidence.SourceTypePubMed, PMID: c.PMID, Title: c.Article.Title, Snippet: strings.Join(c.Article.Abstract.Text, " "), URL: "https://pubmed.ncbi.nlm.nih.gov/" + c.PMID + "/", IdentifierValid: true, RecordExists: true, RetrievedAt: time.Now().UTC().Format(time.RFC3339), ValidationStatus: evidence.ValidationUnverified}
			var pmc string
			for _, id := range a.Data.IDs {
				if id.Type == "doi" {
					src.DOI = id.Value
				}
				if id.Type == "pmc" {
					pmc = id.Value
				}
			}
			if q.Mode == ModeDeep && pmc != "" {
				text, license, err := r.fulltext(ctx, pmc)
				if err == nil {
					src.Snippet = text
					src.FullTextAvailable = true
					src.License = license
				}
			}
			out = append(out, src)
		}
		return out, nil
	}
	// Exact coordinates/ref/alt are checked in the returned ClinVar sequence locations.
	var set struct {
		Records []struct {
			Accession string `xml:"Accession,attr"`
			Name      string `xml:"VariationName,attr"`
			Locations []struct {
				Assembly string `xml:"Assembly,attr"`
				Chrom    string `xml:"Chr,attr"`
				Start    int64  `xml:"positionVCF,attr"`
				Ref      string `xml:"referenceAlleleVCF,attr"`
				Alt      string `xml:"alternateAlleleVCF,attr"`
			} `xml:"ClassifiedRecord>SimpleAllele>Location>SequenceLocation"`
		} `xml:"VariationArchive"`
	}
	if xml.Unmarshal(b, &set) != nil {
		return nil, errors.New("malformed ClinVar record")
	}
	var out []evidence.Source
	for _, v := range set.Records {
		matched := false
		for _, l := range v.Locations {
			matched = matched || (q.VariantType == "snv" && q.Pos > 0 && l.Assembly == q.Assembly && l.Chrom == strings.TrimPrefix(q.Chrom, "chr") && l.Start == q.Pos && l.Ref == q.Ref && l.Alt == q.Alt)
		}
		out = append(out, evidence.Source{ID: v.Accession, Type: evidence.SourceTypeClinVar, ClinVarID: v.Accession, Title: v.Name, URL: "https://www.ncbi.nlm.nih.gov/clinvar/variation/" + v.Accession + "/", IdentifierValid: true, RecordExists: true, ObjectMatched: matched, ValidationStatus: evidence.ValidationUnverified, RetrievedAt: time.Now().UTC().Format(time.RFC3339)})
	}
	return out, nil
}
func (r *OfficialRetriever) fulltext(ctx context.Context, pmc string) (string, string, error) {
	// Only the Open Access subset may be fetched for this text-mining pipeline.
	b, e := r.get(ctx, "esearch.fcgi", url.Values{"db": {"pmc"}, "term": {pmc + " AND open access[filter]"}, "retmode": {"json"}})
	if e != nil {
		return "", "", e
	}
	var result struct {
		R struct {
			IDs []string `json:"idlist"`
		} `json:"esearchresult"`
	}
	if json.Unmarshal(b, &result) != nil || len(result.R.IDs) == 0 {
		return "", "", errors.New("open access permission unavailable")
	}
	b, e = r.get(ctx, "efetch.fcgi", url.Values{"db": {"pmc"}, "id": {result.R.IDs[0]}, "retmode": {"xml"}})
	if e != nil {
		return "", "", e
	}
	var record struct {
		Articles []struct {
			License struct {
				URL string `xml:"href,attr"`
			} `xml:"front>article-meta>permissions>license"`
			Body string `xml:"body,innerxml"`
		} `xml:"article"`
	}
	if xml.Unmarshal(b, &record) != nil || len(record.Articles) == 0 || !permittedLicense(record.Articles[0].License.URL) {
		return "", "", errors.New("article license unavailable")
	}
	return record.Articles[0].Body, record.Articles[0].License.URL, nil
}

func permittedLicense(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && u.Scheme != "http") || u.Host != "creativecommons.org" {
		return false
	}
	p := strings.TrimSuffix(u.Path, "/")
	return p == "/licenses/by/4.0" || p == "/licenses/by/3.0" || p == "/publicdomain/zero/1.0"
}
