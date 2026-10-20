package integration_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/pzweuj/sepilens/internal/hpo"
)

type GoldenCase struct {
	ID                  string   `json:"id"`
	Description         string   `json:"description"`
	ClinicalText        string   `json:"clinical_text"`
	ExpectedPresentIDs  []string `json:"expected_present_ids"`
	ForbiddenNegatedIDs []string `json:"forbidden_negated_ids"`
}

func TestHPO_GoldenCasesRegression(t *testing.T) {
	fixturePath := filepath.Join("..", "fixtures", "hpo", "golden_cases.json")
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("failed to read golden cases fixture: %v", err)
	}

	var cases []GoldenCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatalf("failed to parse golden cases fixture: %v", err)
	}

	kDir := filepath.Join("..", "..", "tests", "fixtures", "hpo", "reference")
	idx, err := hpo.LoadDefaultIndex(kDir)
	if err != nil {
		t.Fatalf("failed to load HPO index: %v", err)
	}

	extractor := hpo.NewExtractor(idx, nil)

	for _, tc := range cases {
		t.Run(tc.ID, func(t *testing.T) {
			res, err := extractor.Extract(context.Background(), tc.ClinicalText, hpo.ExtractOptions{
				MaxCandidates: 50, AllowLocalFallback: true,
			})
			if err != nil {
				t.Fatalf("extraction failed on %s: %v", tc.ID, err)
			}

			// Verify forbidden negated IDs are strictly absent
			for _, term := range res.Terms {
				for _, forbidden := range tc.ForbiddenNegatedIDs {
					if term.HPO == forbidden && term.Polarity == "present" {
						t.Fatalf("REGRESSION FAILURE [%s]: forbidden negated ID %s was returned in present terms!", tc.ID, forbidden)
					}
				}
			}

			// Verify candidate recall contains expected IDs
			candidates := idx.Search(tc.ClinicalText, 50)
			candidateIDs := make(map[string]bool)
			for _, c := range candidates {
				candidateIDs[c.Term.ID] = true
			}
			for _, expID := range tc.ExpectedPresentIDs {
				foundInTerms := false
				for _, term := range res.Terms {
					if term.HPO == expID {
						foundInTerms = true
						break
					}
				}
				if !foundInTerms {
					t.Fatalf("Missing expected term [%s]: expected ID %s was not recalled in top candidate pool", tc.ID, expID)
				}
			}
		})
	}
}
