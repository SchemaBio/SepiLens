package hpo_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/pzweuj/sepilens/internal/hpo"
)

func getKnowledgeDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join("..", "..", "tests", "fixtures", "hpo", "reference")
	return dir
}

func TestIndex_LoadAndSearch(t *testing.T) {
	kDir := getKnowledgeDir(t)
	idx, err := hpo.LoadDefaultIndex(kDir)
	if err != nil {
		t.Fatalf("failed to load default hpo index: %v", err)
	}

	count := idx.TermCount()
	if count < 5 {
		t.Fatalf("expected small regression fixture terms, got %d", count)
	}

	// 1. Direct ID lookup
	term, ok := idx.GetTerm("HP:0001250")
	if !ok || term.Name != "Seizure" {
		t.Fatalf("expected HP:0001250 Seizure, got %+v", term)
	}

	// 2. ID search in query
	resID := idx.Search("HP:0001250", 10)
	if len(resID) == 0 || resID[0].Term.ID != "HP:0001250" {
		t.Fatalf("expected search by ID to return HP:0001250, got %+v", resID)
	}

	// 3. Exact English name search
	resEn := idx.Search("Hypertension", 10)
	if len(resEn) == 0 || resEn[0].Term.ID != "HP:0000822" {
		t.Fatalf("expected HP:0000822 Hypertension as top result, got %+v", resEn)
	}

	// 4. Chinese search: 眩晕
	resCn := idx.Search("眩晕", 10)
	if len(resCn) == 0 {
		t.Fatalf("expected results for 眩晕, got none")
	}
	foundVertigo := false
	for _, c := range resCn {
		if c.Term.NameCN == "眩晕" || strings.Contains(c.Term.NameCN, "眩晕") {
			foundVertigo = true
			break
		}
	}
	if !foundVertigo {
		t.Fatalf("expected 眩晕 in results: %+v", resCn)
	}

	// 5. Chinese complex phrase: 发作性认知障碍
	candidates := idx.SearchCandidates([]string{"认知障碍", "头晕"}, 30)
	if len(candidates) == 0 {
		t.Fatalf("expected candidates for 认知障碍, 头晕, got none")
	}
}

func BenchmarkIndex_Search(b *testing.B) {
	kDir := filepath.Join("..", "..", "tests", "fixtures", "hpo", "reference")
	idx, err := hpo.LoadDefaultIndex(kDir)
	if err != nil {
		b.Fatalf("failed to load index: %v", err)
	}

	queries := []string{
		"眩晕",
		"认知障碍",
		"心肌梗死",
		"Seizure",
		"HP:0001250",
		"多囊性肾发育不良",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q := queries[i%len(queries)]
		_ = idx.Search(q, 20)
	}
}
