package retrieval

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestOfficialRequestBudgetAndCitationTraversal(t *testing.T) {
	var calls atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		switch {
		case strings.Contains(r.URL.Path, "esearch"):
			fmt.Fprint(w, `{"esearchresult":{"idlist":["123"]}}`)
		case strings.Contains(r.URL.Path, "elink"):
			fmt.Fprint(w, `<eLinkResult><LinkSet><LinkSetDb><Link><Id>456</Id></Link></LinkSetDb></LinkSet></eLinkResult>`)
		default:
			fmt.Fprintf(w, `<PubmedArticleSet><PubmedArticle><MedlineCitation><PMID>%s</PMID><Article><ArticleTitle>TEST study</ArticleTitle></Article></MedlineCitation></PubmedArticle></PubmedArticleSet>`, r.URL.Query().Get("id"))
		}
	}))
	defer s.Close()
	r := &OfficialRetriever{Database: "pubmed", HTTP: s.Client(), BaseURL: s.URL}
	ctx := context.WithValue(context.Background(), budgetKey{}, &Budget{Max: 3})
	sources, err := r.Retrieve(ctx, Query{Gene: "TEST", Mode: ModeDeep})
	if err != ErrBudgetExceeded || calls.Load() != 3 || len(sources) != 1 || sources[0].MaterialVerified {
		t.Fatalf("budget must preserve partial unverified record: calls=%d sources=%+v err=%v", calls.Load(), sources, err)
	}
	calls.Store(0)
	ctx = context.WithValue(context.Background(), budgetKey{}, &Budget{Max: 10})
	sources, err = r.Retrieve(ctx, Query{Gene: "TEST", Mode: ModeDeep})
	if err != nil || len(sources) != 2 || sources[1].PMID != "456" || calls.Load() != 4 {
		t.Fatalf("citation traversal: %+v %v calls=%d", sources, err, calls.Load())
	}
}

func TestClinVarExactMatchAndCNVRegionQuery(t *testing.T) {
	var term string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "esearch") {
			term = r.URL.Query().Get("term")
			fmt.Fprint(w, `{"esearchresult":{"idlist":["123"]}}`)
		} else {
			fmt.Fprint(w, `<VariationArchiveSet><VariationArchive Accession="VCV000000123" VariationName="TEST"><ClassifiedRecord><SimpleAllele><Location><SequenceLocation Assembly="GRCh38" Chr="1" positionVCF="100" referenceAlleleVCF="A" alternateAlleleVCF="G"/></Location></SimpleAllele></ClassifiedRecord></VariationArchive></VariationArchiveSet>`)
		}
	}))
	defer s.Close()
	r := &OfficialRetriever{Database: "clinvar", HTTP: s.Client(), BaseURL: s.URL}
	q := Query{Gene: "TEST", Assembly: "GRCh38", Chrom: "1", Pos: 100, Ref: "A", Alt: "G", VariantType: "snv", Mode: ModeTargeted}
	sources, err := r.Retrieve(context.Background(), q)
	if err != nil || len(sources) != 1 || !sources[0].ObjectMatched || sources[0].MaterialVerified {
		t.Fatalf("exact record match: %+v %v", sources, err)
	}
	q.Alt = "T"
	sources, _ = r.Retrieve(context.Background(), q)
	if sources[0].ObjectMatched {
		t.Fatal("different allele matched")
	}
	q.VariantType = "cnv_gain"
	q.Pos = 0
	q.Start = 50
	q.End = 150
	sources, _ = r.Retrieve(context.Background(), q)
	if !strings.Contains(term, "50:150[chrpos38]") || sources[0].ObjectMatched {
		t.Fatal("CNV must query interval without inferring complex-event equivalence")
	}
}

func TestFullTextLicenseAllowlist(t *testing.T) {
	for _, raw := range []string{"https://creativecommons.org/licenses/by/4.0/", "http://creativecommons.org/licenses/by/3.0/", "https://creativecommons.org/publicdomain/zero/1.0/"} {
		if !permittedLicense(raw) {
			t.Fatalf("permitted license rejected: %s", raw)
		}
	}
	for _, raw := range []string{"", "Open access", "https://creativecommons.org/licenses/by-nc/4.0/", "https://creativecommons.org/licenses/by-nd/4.0/", "https://creativecommons.org.evil.test/licenses/by/4.0/", "https://user@creativecommons.org/licenses/by/4.0/"} {
		if permittedLicense(raw) {
			t.Fatalf("unconfirmed license accepted: %s", raw)
		}
	}
}
