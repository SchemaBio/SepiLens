package annotation

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
	"time"
)

// Opt-in acceptance measurement, kept out of normal CI unit-test workloads.
func TestMillionRecordIndexPerformance(t *testing.T) {
	if os.Getenv("SEPILENS_INDEX_BENCHMARK") != "1" {
		t.Skip("set SEPILENS_INDEX_BENCHMARK=1 for the million-row acceptance measurement")
	}
	ctx := context.Background()
	now := time.Now().UTC()
	path := filepath.Join(t.TempDir(), "million.sqlite")
	const count = 1_000_000
	assets := []AssetMetadata{{Kind: "gnomad", Version: "synthetic", Assembly: "GRCh38", LicenseConfirmed: true, RetrievedAt: now.Format(time.RFC3339)}}
	peak := uint64(0)
	var memory runtime.MemStats
	start := time.Now()
	err := BuildIndex(ctx, path, "million", assets, func(emit func(IndexedRecord) error) error {
		for n := 1; n <= count; n++ {
			if e := emit(IndexedRecord{Kind: "gnomad", Query: Query{Assembly: "GRCh38", Chrom: "1", Pos: int64(n), Ref: "A", Alt: "G"}, Data: json.RawMessage(`{"popmax_af":0.0001,"faf95":0.00001,"allele_number":100000,"coverage_adequate":true}`)}); e != nil {
				return e
			}
			if n%10000 == 0 {
				runtime.ReadMemStats(&memory)
				if memory.Alloc > peak {
					peak = memory.Alloc
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	importMS := float64(time.Since(start).Microseconds()) / 1000
	index, err := OpenIndex(path, "million")
	if err != nil {
		t.Fatal(err)
	}
	defer index.Close()
	query := Query{Assembly: "GRCh38", Chrom: "1", Pos: 499999, Ref: "A", Alt: "G"}
	start = time.Now()
	if _, err = index.Resolve(ctx, query, now); err != nil {
		t.Fatal(err)
	}
	coldMS := float64(time.Since(start).Microseconds()) / 1000
	for n := 0; n < 20; n++ {
		_, _ = index.Resolve(ctx, query, now)
	}
	var durations []float64
	for n := 0; n < 300; n++ {
		query.Pos = int64(1 + (n*7919)%count)
		start = time.Now()
		s, e := index.Resolve(ctx, query, now)
		if e != nil || s.GnomAD.Status != StatusFound {
			t.Fatal(e)
		}
		durations = append(durations, float64(time.Since(start).Microseconds())/1000)
	}
	sort.Float64s(durations)
	p95 := durations[int(float64(len(durations)-1)*.95)]
	info, _ := os.Stat(path)
	report := map[string]any{"rows": count, "samples": len(durations), "os": runtime.GOOS, "arch": runtime.GOARCH, "logical_cpus": runtime.NumCPU(), "cpu": os.Getenv("PROCESSOR_IDENTIFIER"), "go_version": runtime.Version(), "import_ms": importMS, "cold_query_ms": coldMS, "warm_p95_ms": p95, "peak_sampled_go_heap_bytes": peak, "index_bytes": info.Size(), "passed": p95 < 50, "scope": "synthetic indexed records, complete five-source resolution, no network"}
	b, _ := json.MarshalIndent(report, "", "  ")
	t.Log(string(b))
	if p := os.Getenv("SEPILENS_BENCH_REPORT"); p != "" {
		if err = os.WriteFile(p, append(b, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if p95 >= 50 {
		t.Fatal("warm index query p95 exceeds 50ms", p95)
	}
}
