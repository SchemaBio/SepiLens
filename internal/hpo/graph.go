package hpo

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
)

var (
	ErrGraphCycle   = errors.New("hpo graph contains a cycle")
	ErrGraphNoTerms = errors.New("hpo graph has no terms")
)

// Graph is the HPO directed acyclic graph used for ancestor/descendant matching.
// The translated knowledge file carries names and definitions but no is_a edges,
// so a graph built from it is structurally flat and reports itself as incomplete.
type Graph struct {
	mu          sync.RWMutex
	terms       map[string]Term
	parents     map[string][]string
	children    map[string][]string
	roots       []string
	altToMain   map[string]string
	ancestors   map[string][]string
	descendants map[string][]string
	depth       map[string]int
	complete    bool
	hash        string
}

// NewGraph builds the DAG. parentEdges may be supplied separately when the term
// source has no hierarchy (e.g. the bundled translation layer).
func NewGraph(terms map[string]Term) (*Graph, error) {
	if len(terms) == 0 {
		return nil, ErrGraphNoTerms
	}
	g := &Graph{
		terms:       make(map[string]Term, len(terms)),
		parents:     make(map[string][]string, len(terms)),
		children:    make(map[string][]string, len(terms)),
		altToMain:   make(map[string]string),
		ancestors:   make(map[string][]string, len(terms)),
		descendants: make(map[string][]string, len(terms)),
		depth:       make(map[string]int, len(terms)),
	}
	for rawID, term := range terms {
		id := normalizeID(rawID)
		if id == "" {
			continue
		}
		term.ID = id
		g.terms[id] = term
		for _, alt := range term.AltIDs {
			if alt = normalizeID(alt); alt != "" && alt != id {
				g.altToMain[alt] = id
			}
		}
		for _, parent := range term.Parents {
			if parent = normalizeID(parent); parent != "" && parent != id {
				g.parents[id] = append(g.parents[id], parent)
			}
		}
	}
	for id, parents := range g.parents {
		sort.Strings(parents)
		g.parents[id] = parents
		for _, parent := range parents {
			g.children[parent] = append(g.children[parent], id)
		}
	}
	for id := range g.children {
		sort.Strings(g.children[id])
	}
	edgeCount := 0
	for _, parents := range g.parents {
		edgeCount += len(parents)
	}
	g.complete = edgeCount > 0
	if err := g.validate(); err != nil {
		return nil, err
	}
	g.computeDepth()
	g.hash = g.fingerprint()
	return g, nil
}

func normalizeID(id string) string { return strings.ToUpper(strings.TrimSpace(id)) }

// Complete reports whether any is_a edges were loaded. Callers must not treat a
// flat graph as a valid ontology: ancestor/descendant matching would be vacuous.
func (g *Graph) Complete() bool { return g != nil && g.complete }

func (g *Graph) Hash() string { return g.hash }

func (g *Graph) TermCount() int {
	if g == nil {
		return 0
	}
	return len(g.terms)
}

func (g *Graph) validate() error {
	// Every parent must resolve to a known term; report the first missing edge.
	for id, parents := range g.parents {
		for _, parent := range parents {
			if _, ok := g.terms[parent]; !ok {
				return fmt.Errorf("hpo graph: %s references unknown parent %s", id, parent)
			}
		}
	}
	// Iterative DFS colouring detects cycles without deep recursion.
	const (
		white = 0
		grey  = 1
		black = 2
	)
	colour := make(map[string]int, len(g.terms))
	type frame struct {
		id   string
		next int
	}
	for root := range g.terms {
		if colour[root] != white {
			continue
		}
		stack := []frame{{id: root}}
		colour[root] = grey
		for len(stack) > 0 {
			top := &stack[len(stack)-1]
			parents := g.parents[top.id]
			if top.next >= len(parents) {
				colour[top.id] = black
				stack = stack[:len(stack)-1]
				continue
			}
			parent := parents[top.next]
			top.next++
			switch colour[parent] {
			case grey:
				return fmt.Errorf("%w: %s -> %s", ErrGraphCycle, top.id, parent)
			case white:
				colour[parent] = grey
				stack = append(stack, frame{id: parent})
			}
		}
	}
	g.roots = g.roots[:0]
	for id := range g.terms {
		if len(g.parents[id]) == 0 {
			g.roots = append(g.roots, id)
		}
	}
	sort.Strings(g.roots)
	return nil
}

func (g *Graph) computeDepth() {
	// Depth is distance to the nearest root, computed breadth-first from roots.
	for _, root := range g.roots {
		g.depth[root] = 0
	}
	queue := append([]string(nil), g.roots...)
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		for _, child := range g.children[id] {
			next := g.depth[id] + 1
			if current, seen := g.depth[child]; !seen || next < current {
				g.depth[child] = next
				queue = append(queue, child)
			}
		}
	}
}

func (g *Graph) fingerprint() string {
	ids := make([]string, 0, len(g.terms))
	for id := range g.terms {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var b strings.Builder
	for _, id := range ids {
		b.WriteString(id)
		b.WriteByte('>')
		for _, parent := range g.parents[id] {
			b.WriteString(parent)
			b.WriteByte(',')
		}
		b.WriteByte(';')
	}
	return fmt.Sprintf("%x", fnv64(b.String()))
}

func fnv64(s string) uint64 {
	const (
		offset = 14695981039346656037
		prime  = 1099511628211
	)
	h := uint64(offset)
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= prime
	}
	return h
}

// Resolve maps an alternate or obsolete identifier to a current term ID.
func (g *Graph) Resolve(id string) (string, bool) {
	if g == nil {
		return "", false
	}
	id = normalizeID(id)
	// An obsolete identifier must map to its replacement before the direct lookup;
	// otherwise a caller keeps a withdrawn term.
	g.mu.RLock()
	term, known := g.terms[id]
	g.mu.RUnlock()
	if known && term.Obsolete {
		if term.ReplacedBy == "" {
			return "", false
		}
		replacement := normalizeID(term.ReplacedBy)
		if _, exists := g.terms[replacement]; exists {
			return replacement, true
		}
		return "", false
	}
	if known {
		return id, true
	}
	if main, ok := g.altToMain[id]; ok {
		return main, true
	}
	return "", false
}

func (g *Graph) Term(id string) (Term, bool) {
	main, ok := g.Resolve(id)
	if !ok {
		return Term{}, false
	}
	return g.terms[main], true
}

// Ancestors returns every proper ancestor, nearest first, excluding the term itself.
func (g *Graph) Ancestors(id string) []string {
	main, ok := g.Resolve(id)
	if !ok {
		return nil
	}
	g.mu.RLock()
	if cached, exists := g.ancestors[main]; exists {
		g.mu.RUnlock()
		return cached
	}
	g.mu.RUnlock()

	seen := map[string]bool{main: true}
	var out []string
	queue := append([]string(nil), g.parents[main]...)
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if seen[current] {
			continue
		}
		seen[current] = true
		out = append(out, current)
		queue = append(queue, g.parents[current]...)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if g.depth[out[i]] != g.depth[out[j]] {
			return g.depth[out[i]] > g.depth[out[j]]
		}
		return out[i] < out[j]
	})

	g.mu.Lock()
	g.ancestors[main] = out
	g.mu.Unlock()
	return out
}

// Descendants returns every proper descendant, excluding the term itself.
func (g *Graph) Descendants(id string) []string {
	main, ok := g.Resolve(id)
	if !ok {
		return nil
	}
	g.mu.RLock()
	if cached, exists := g.descendants[main]; exists {
		g.mu.RUnlock()
		return cached
	}
	g.mu.RUnlock()

	seen := map[string]bool{main: true}
	var out []string
	queue := append([]string(nil), g.children[main]...)
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if seen[current] {
			continue
		}
		seen[current] = true
		out = append(out, current)
		queue = append(queue, g.children[current]...)
	}
	sort.Strings(out)

	g.mu.Lock()
	g.descendants[main] = out
	g.mu.Unlock()
	return out
}

// IsAncestor reports whether ancestor is a strict ancestor of descendant.
func (g *Graph) IsAncestor(ancestor, descendant string) bool {
	ancestor, ok := g.Resolve(ancestor)
	if !ok {
		return false
	}
	descendant, ok = g.Resolve(descendant)
	if !ok {
		return false
	}
	for _, candidate := range g.Ancestors(descendant) {
		if candidate == ancestor {
			return true
		}
	}
	return false
}

// ClosestCommonAncestor returns the deepest shared ancestor, or "" when the only
// shared ancestor is a root above both terms.
func (g *Graph) ClosestCommonAncestor(a, b string) (string, bool) {
	a, ok := g.Resolve(a)
	if !ok {
		return "", false
	}
	b, ok = g.Resolve(b)
	if !ok {
		return "", false
	}
	if a == b {
		return a, true
	}
	depth := map[string]int{a: g.depth[a]}
	for _, ancestor := range g.Ancestors(a) {
		depth[ancestor] = g.depth[ancestor]
	}
	best := ""
	bestDepth := -1
	consider := func(id string) {
		d, exists := depth[id]
		if !exists || d <= bestDepth {
			return
		}
		best = id
		bestDepth = d
	}
	consider(b)
	for _, ancestor := range g.Ancestors(b) {
		consider(ancestor)
	}
	if best == "" || len(g.roots) == 0 {
		return best, best != ""
	}
	for _, root := range g.roots {
		if best == root {
			return "", false
		}
	}
	return best, true
}

// Depth returns the distance to the nearest root.
func (g *Graph) Depth(id string) int {
	main, ok := g.Resolve(id)
	if !ok {
		return -1
	}
	return g.depth[main]
}

// InformationContent is the structural specificity of a term:
// log2(total terms / (1 + descendant count)). It depends only on the DAG and is
// not a substitute for disease-annotation based background frequency.
func (g *Graph) InformationContent(id string) float64 {
	if g == nil || len(g.terms) == 0 {
		return 0
	}
	if _, ok := g.Resolve(id); !ok {
		return 0
	}
	descendants := g.Descendants(id)
	ratio := float64(len(g.terms)) / float64(1+len(descendants))
	if ratio <= 1 {
		return 0
	}
	return math.Log2(ratio)
}

// ObsolescenceMap returns obsolete terms that carry a replacement.
func (g *Graph) ObsolescenceMap() map[string]string {
	out := map[string]string{}
	if g == nil {
		return out
	}
	for id, term := range g.terms {
		if term.Obsolete && term.ReplacedBy != "" {
			out[id] = normalizeID(term.ReplacedBy)
		}
	}
	return out
}
