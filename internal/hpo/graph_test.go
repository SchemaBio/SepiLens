package hpo_test

import (
	"errors"
	"testing"

	"github.com/pzweuj/sepilens/internal/hpo"
)

func diamond() map[string]hpo.Term {
	return map[string]hpo.Term{
		"HP:0000001": {ID: "HP:0000001", Name: "All"},
		"HP:0000118": {ID: "HP:0000118", Name: "Phenotypic abnormality", Parents: []string{"HP:0000001"}},
		"HP:0000119": {ID: "HP:0000119", Name: "Abnormality of the genitourinary system", Parents: []string{"HP:0000118"}},
		"HP:0000077": {ID: "HP:0000077", Name: "Abnormality of the kidney", Parents: []string{"HP:0000119"}},
		"HP:0000107": {ID: "HP:0000107", Name: "Renal cyst", Parents: []string{"HP:0000077"}},
		"HP:0000083": {ID: "HP:0000083", Name: "Renal insufficiency", Parents: []string{"HP:0000077"}},
	}
}

func TestGraphAncestorsAndDescendants(t *testing.T) {
	g, err := hpo.NewGraph(diamond())
	if err != nil {
		t.Fatal(err)
	}
	if !g.Complete() {
		t.Fatal("graph with is_a edges must be complete")
	}
	ancestors := g.Ancestors("HP:0000107")
	if len(ancestors) != 4 || ancestors[0] != "HP:0000077" {
		t.Fatalf("unexpected ancestors %v", ancestors)
	}
	descendants := g.Descendants("HP:0000077")
	if len(descendants) != 2 {
		t.Fatalf("unexpected descendants %v", descendants)
	}
	if !g.IsAncestor("HP:0000118", "HP:0000107") || g.IsAncestor("HP:0000107", "HP:0000118") {
		t.Fatal("ancestor direction wrong")
	}
	if g.IsAncestor("HP:0000107", "HP:0000107") {
		t.Fatal("a term must not be its own ancestor")
	}
}

func TestGraphCommonAncestorAndDepth(t *testing.T) {
	g, _ := hpo.NewGraph(diamond())
	lca, ok := g.ClosestCommonAncestor("HP:0000107", "HP:0000083")
	if !ok || lca != "HP:0000077" {
		t.Fatalf("lca %q ok=%v", lca, ok)
	}
	if _, ok := g.ClosestCommonAncestor("HP:0000107", "HP:0000001"); ok {
		t.Fatal("root-only shared ancestor must not count")
	}
	if g.Depth("HP:0000107") != 4 {
		t.Fatalf("depth %d", g.Depth("HP:0000107"))
	}
}

func TestGraphDetectsCycle(t *testing.T) {
	terms := map[string]hpo.Term{
		"HP:0000001": {ID: "HP:0000001", Name: "A", Parents: []string{"HP:0000002"}},
		"HP:0000002": {ID: "HP:0000002", Name: "B", Parents: []string{"HP:0000001"}},
	}
	if _, err := hpo.NewGraph(terms); !errors.Is(err, hpo.ErrGraphCycle) {
		t.Fatalf("cycle not detected: %v", err)
	}
}

func TestGraphRejectsSelfEdgeAndDisconnectedHierarchy(t *testing.T) {
	if _, e := hpo.NewGraph(map[string]hpo.Term{"HP:0000001": {Parents: []string{"HP:0000001"}}}); !errors.Is(e, hpo.ErrGraphCycle) {
		t.Fatal("self cycle accepted", e)
	}
	terms := diamond()
	terms["HP:9999999"] = hpo.Term{ID: "HP:9999999"}
	if _, e := hpo.NewGraph(terms); e == nil {
		t.Fatal("disconnected active root accepted")
	}
}

func TestGraphRejectsUnknownParent(t *testing.T) {
	terms := map[string]hpo.Term{
		"HP:0000002": {ID: "HP:0000002", Name: "B", Parents: []string{"HP:9999999"}},
	}
	if _, err := hpo.NewGraph(terms); err == nil {
		t.Fatal("dangling parent accepted")
	}
}

func TestFlatGraphIsIncomplete(t *testing.T) {
	terms := map[string]hpo.Term{
		"HP:0000001": {ID: "HP:0000001", Name: "All"},
		"HP:0000107": {ID: "HP:0000107", Name: "Renal cyst"},
	}
	g, err := hpo.NewGraph(terms)
	if err != nil {
		t.Fatal(err)
	}
	if g.Complete() {
		t.Fatal("translation-only data must not claim a hierarchy")
	}
	if len(g.Ancestors("HP:0000107")) != 0 {
		t.Fatal("flat graph produced ancestors")
	}
}

func TestAltAndObsoleteResolution(t *testing.T) {
	terms := map[string]hpo.Term{
		"HP:0000001": {ID: "HP:0000001", Name: "All"},
		"HP:0000107": {ID: "HP:0000107", Name: "Renal cyst", Parents: []string{"HP:0000001"}, AltIDs: []string{"HP:0009999"}},
		"HP:0000083": {ID: "HP:0000083", Name: "Renal insufficiency", Obsolete: true, ReplacedBy: "HP:0000107"},
	}
	g, err := hpo.NewGraph(terms)
	if err != nil {
		t.Fatal(err)
	}
	if id, ok := g.Resolve("HP:0009999"); !ok || id != "HP:0000107" {
		t.Fatalf("alt id %q %v", id, ok)
	}
	if id, ok := g.Resolve("HP:0000083"); !ok || id != "HP:0000107" {
		t.Fatalf("replaced by %q %v", id, ok)
	}
}

func TestInformationContentIsMonotonic(t *testing.T) {
	g, _ := hpo.NewGraph(diamond())
	if g.InformationContent("HP:0000107") <= g.InformationContent("HP:0000077") {
		t.Fatal("specific term must be more informative than its parent")
	}
	if g.InformationContent("HP:0000001") != 0 {
		t.Fatalf("root IC must be zero, got %v", g.InformationContent("HP:0000001"))
	}
}

func TestGraphHashChangesWithEdges(t *testing.T) {
	a, _ := hpo.NewGraph(diamond())
	terms := diamond()
	term := terms["HP:0000083"]
	term.Parents = []string{"HP:0000119"}
	terms["HP:0000083"] = term
	b, _ := hpo.NewGraph(terms)
	if a.Hash() == b.Hash() {
		t.Fatal("hash ignores edge changes")
	}
}
