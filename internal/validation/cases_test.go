package validation

import (
	"encoding/json"
	"github.com/pzweuj/sepilens/internal/engine"
	"os"
	"path/filepath"
	"testing"
)

func TestIndependentExpectationMismatchFails(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "cases.json")
	out := filepath.Join(dir, "report.json")
	fixture := `[{"id":"synthetic-no-clinical-approval","source":"https://pmc.ncbi.nlm.nih.gov/articles/PMC4544753/","reviewer":"synthetic-test-only","kind":"missing","coverage":["PM2"],"snv":{"variant":{"gene":"TEST","consequence":"stop_gained"},"context":{},"criteria":[],"sources":[]},"expected":{"classification":"Pathogenic","codes":["PVS1"]}}]`
	if e := os.WriteFile(input, []byte(fixture), 0600); e != nil {
		t.Fatal(e)
	}
	if e := Run(engine.EngineACMG2015, input, "", out); e == nil {
		t.Fatal("false expectation passed")
	}
	b, e := os.ReadFile(out)
	if e != nil {
		t.Fatal(e)
	}
	var r Report
	if json.Unmarshal(b, &r) != nil || len(r.Cases) != 1 || r.Cases[0].Passed {
		t.Fatal("report misrepresented failure")
	}
}
