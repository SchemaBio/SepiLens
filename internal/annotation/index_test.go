package annotation

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestResolverDoesNotMutateAndExpires(t *testing.T) {
	at := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
	in := &Set{GnomAD: &GnomAD{Record: Record{Version: "v4", RetrievedAt: at.AddDate(0, 0, -91).Format(time.RFC3339)}}}
	before, _ := json.Marshal(in)
	r := NewResolver()
	r.Now = func() time.Time { return at }
	out, err := r.Normalize(in, "GRCh38")
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(in)
	if string(before) != string(after) || out.GnomAD.Status != StatusStale {
		t.Fatal("input mutated or old record usable", out)
	}
}
func TestReadOnlyIndexStatusAndAssembly(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	q := Query{Assembly: "GRCh38", Chrom: "7", Pos: 100, Ref: "A", Alt: "G", Gene: "G", DiseaseID: "OMIM:1", MOI: "AR"}
	af := .01
	gn := GnomAD{PopMaxAF: &af}
	b, _ := json.Marshal(gn)
	p := filepath.Join(t.TempDir(), "index.db")
	assets := []AssetMetadata{{Kind: "gnomad", Version: "v4", Assembly: q.Assembly, LicenseConfirmed: true, RetrievedAt: now.Format(time.RFC3339)}}
	if e := BuildIndex(ctx, p, "locked", assets, func(emit func(IndexedRecord) error) error {
		return emit(IndexedRecord{Kind: "gnomad", Query: q, Data: b})
	}); e != nil {
		t.Fatal(e)
	}
	i, e := OpenIndex(p, "locked")
	if e != nil {
		t.Fatal(e)
	}
	defer i.Close()
	s, e := i.Resolve(ctx, q, now)
	if e != nil || s.GnomAD.Status != StatusFound || !reflect.DeepEqual(s.GnomAD.PopMaxAF, &af) || s.Pangolin.Status != StatusUnavailable {
		t.Fatal(s, e)
	}
	q.Pos++
	s, e = i.Resolve(ctx, q, now)
	if e != nil || s.GnomAD.Status != StatusNotFound {
		t.Fatal(s, e)
	}
	q.Assembly = "GRCh37"
	s, e = i.Resolve(ctx, q, now)
	if e != nil || s.GnomAD.Status != StatusUnavailable {
		t.Fatal(s, e)
	}
	if _, e = i.db.Exec("DELETE FROM records"); e == nil {
		t.Fatal("runtime index is writable")
	}
	if wrong, e := OpenIndex(p, "other"); e == nil {
		wrong.Close()
		t.Fatal("wrong dataset accepted")
	}
}

func TestConflictingIndexAndLicense(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	q := Query{Assembly: "GRCh38", Chrom: "1", Pos: 1, Ref: "A", Alt: "G"}
	for _, licensed := range []bool{true, false} {
		p := filepath.Join(t.TempDir(), "i.db")
		assets := []AssetMetadata{{Kind: "clinvar", Version: "test", Assembly: q.Assembly, LicenseConfirmed: licensed, RetrievedAt: now.Format(time.RFC3339)}}
		e := BuildIndex(ctx, p, "h", assets, func(emit func(IndexedRecord) error) error {
			for _, s := range []string{`{"classification":"Pathogenic"}`, `{"classification":"Benign"}`} {
				if e := emit(IndexedRecord{Kind: "clinvar", Query: q, Data: json.RawMessage(s)}); e != nil {
					return e
				}
			}
			return nil
		})
		if e != nil {
			t.Fatal(e)
		}
		i, e := OpenIndex(p, "h")
		if e != nil {
			t.Fatal(e)
		}
		s, e := i.Resolve(ctx, q, now)
		i.Close()
		if e != nil {
			t.Fatal(e)
		}
		want := StatusUnavailable
		if licensed {
			want = StatusConflicting
		}
		if s.ClinVar.Status != want {
			t.Fatal(s.ClinVar)
		}
	}
}
