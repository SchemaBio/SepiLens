package assessment

import (
	"encoding/json"
	"os"
	"testing"
)

// This fixture is the exact Octopus BuildPayload output, including its fact pack.
// Changes must be reviewed on both sides of the service boundary.
func TestOctopusFactPackWireContract(t *testing.T) {
	raw, err := os.ReadFile("testdata/fact_pack_contract.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Feature string  `json:"feature"`
		Request Request `json:"request"`
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) != 4 {
		t.Fatal("all four assessment engines required")
	}
	for _, c := range cases {
		t.Run(c.Feature, func(t *testing.T) {
			normalized, err := Normalize(c.Request)
			if err != nil {
				t.Fatal(err)
			}
			rebuilt := BuildImportedFactPack(normalized)
			if rebuilt.InputStatus != "matched" {
				t.Fatalf("fact pack not coherent after request normalization: %s", rebuilt.InputStatus)
			}
			for _, fact := range rebuilt.Facts {
				if fact.Status == "verified" || fact.Version != "unversioned" {
					t.Fatal("import promoted to trusted material")
				}
			}
		})
	}
}
