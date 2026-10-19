// Package phenotype implements SepiLens' independent, versioned engineering
// phenotype score. It is not a clinical validation or a LIRICAL implementation.
package phenotype

import (
	"bufio"
	"errors"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pzweuj/sepilens/internal/hpo"
)

const SpecVersion = "sepi-pheno-0.1"

type Observation struct {
	ID       string `json:"id"`
	Polarity string `json:"polarity,omitempty"`
	Onset    string `json:"onset,omitempty"`
}
type Input struct {
	Terms  []Observation `json:"hpo_terms"`
	Source string        `json:"source"`
}
type Association struct {
	DiseaseID         string  `json:"disease_id"`
	HPO               string  `json:"hpo"`
	Frequency         float64 `json:"frequency"`
	ExplicitFrequency bool    `json:"explicit_frequency"`
	Excluded          bool    `json:"excluded"`
}
type TermMatch struct {
	HPO         string  `json:"hpo"`
	Polarity    string  `json:"polarity"`
	Relation    string  `json:"match"`
	DiseaseTerm string  `json:"disease_term,omitempty"`
	Frequency   float64 `json:"frequency"`
	Background  float64 `json:"background"`
	LR          float64 `json:"lr"`
	IC          float64 `json:"information_content"`
	Conflict    bool    `json:"conflict"`
	Ignored     bool    `json:"ignored"`
}
type DiseaseScore struct {
	DiseaseID     string      `json:"disease_id"`
	LR            float64     `json:"lr_total"`
	LogLR         float64     `json:"log_lr_total"`
	Rank          int         `json:"rank"`
	Margin        *float64    `json:"margin,omitempty"`
	SpecificTerms int         `json:"specific_terms"`
	Terms         []TermMatch `json:"terms"`
}
type MatchResult struct {
	DatasetVersion     string         `json:"dataset_version"`
	OntologyVersion    string         `json:"ontology_version,omitempty"`
	TranslationVersion string         `json:"translation_version,omitempty"`
	SpecVersion        string         `json:"spec_version"`
	Scores             []DiseaseScore `json:"scores"`
	Missing            []string       `json:"missing_information,omitempty"`
}

type Dataset struct {
	ValidUntil         time.Time
	OntologyVersion    string
	TranslationVersion string
	Graph              *hpo.Graph
	Diseases           map[string][]Association
	Background         map[string]float64
	Version            string
}

func Frequency(s string) (float64, bool, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return defaultFrequency, false, nil
	}
	if f, ok := frequencyMapping()[s]; ok {
		return f, true, nil
	}
	var f float64
	var err error
	if parts := strings.Split(s, "/"); len(parts) == 2 {
		n, e := strconv.ParseFloat(parts[0], 64)
		d, e2 := strconv.ParseFloat(parts[1], 64)
		if e != nil || e2 != nil || d <= 0 {
			return 0, false, errors.New("invalid HPOA frequency fraction")
		}
		f = n / d
	} else if strings.HasSuffix(s, "%") {
		f, err = strconv.ParseFloat(strings.TrimSuffix(s, "%"), 64)
		f /= 100
	} else {
		f, err = strconv.ParseFloat(s, 64)
	}
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) || f < 0 || f > 1 {
		return 0, false, errors.New("invalid HPOA frequency")
	}
	return f, true, nil
}

func ParseHPOA(r io.Reader) ([]Association, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 4096), 2<<20)
	var header []string
	var out []Association
	for sc.Scan() {
		line := strings.TrimSuffix(sc.Text(), "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		cols := strings.Split(strings.TrimPrefix(line, "#"), "\t")
		if header == nil {
			if !strings.Contains(line, "database_id\t") || !strings.Contains(line, "hpo_id") {
				if strings.HasPrefix(line, "#") {
					continue
				}
				return nil, errors.New("unsupported HPOA header")
			}
			header = cols
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		if len(cols) != len(header) {
			return nil, errors.New("invalid HPOA columns")
		}
		row := map[string]string{}
		for n, h := range header {
			row[h] = cols[n]
		}
		f, explicit, err := Frequency(row["frequency"])
		if err != nil {
			return nil, err
		}
		if row["database_id"] == "" || !strings.HasPrefix(row["hpo_id"], "HP:") {
			return nil, errors.New("invalid HPOA identifiers")
		}
		out = append(out, Association{DiseaseID: row["database_id"], HPO: row["hpo_id"], Frequency: f, ExplicitFrequency: explicit, Excluded: row["qualifier"] == "NOT"})
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if header == nil {
		return nil, errors.New("HPOA header missing")
	}
	return out, nil
}

func NewDataset(g *hpo.Graph, rows []Association, version string) (*Dataset, error) {
	if g == nil || !g.Complete() {
		return nil, errors.New("complete HPO ontology required")
	}
	d := &Dataset{Graph: g, Diseases: map[string][]Association{}, Background: map[string]float64{}, Version: version}
	for _, a := range rows {
		id, ok := g.Resolve(a.HPO)
		if !ok {
			return nil, errors.New("HPOA references unknown or unresolved HPO term")
		}
		a.HPO = id
		d.Diseases[a.DiseaseID] = append(d.Diseases[a.DiseaseID], a)
	}
	if len(d.Diseases) == 0 {
		return nil, errors.New("HPOA contains no diseases")
	}
	counts := map[string]int{}
	for _, aa := range d.Diseases {
		seen := map[string]bool{}
		for _, a := range aa {
			if a.Excluded {
				continue
			}
			seen[a.HPO] = true
			for _, id := range g.Ancestors(a.HPO) {
				seen[id] = true
			}
		}
		for id := range seen {
			counts[id]++
		}
	}
	for id, n := range counts {
		d.Background[id] = clamp(float64(n)/float64(len(d.Diseases)), backgroundFloor, backgroundCeiling)
	}
	return d, nil
}
func clamp(n, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, n)) }

// distance is the shortest parent path, including for a multi-parent DAG.
func (d *Dataset) distance(child, parent string) int {
	queue := []string{child}
	dist := map[string]int{child: 0}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if id == parent {
			return dist[id]
		}
		t, _ := d.Graph.Term(id)
		for _, p := range t.Parents {
			if _, ok := dist[p]; !ok {
				dist[p] = dist[id] + 1
				queue = append(queue, p)
			}
		}
	}
	return 1000
}
func (d *Dataset) match(q Observation, disease string) TermMatch {
	bg := d.Background[q.ID]
	if bg == 0 {
		bg = backgroundFloor
	}
	m := TermMatch{HPO: q.ID, Polarity: q.Polarity, Background: bg, IC: -math.Log2(bg), LR: 1, Relation: "unrelated"}
	if q.Polarity != "present" && q.Polarity != "absent" {
		m.Ignored = true
		return m
	}
	aa := d.Diseases[disease]
	for _, a := range aa {
		if a.Excluded && (a.HPO == q.ID || d.Graph.IsAncestor(a.HPO, q.ID)) {
			m.Relation = "excluded"
			m.DiseaseTerm = a.HPO
			if q.Polarity == "absent" {
				m.LR = exclusionSupport
			} else {
				m.LR = clamp(exclusionPenalty/bg, minimumLR, maximumLR)
				m.Conflict = true
			}
			return m
		}
	}
	if q.Polarity == "absent" {
		m.Ignored = true
		for _, a := range aa {
			if a.Excluded || !a.ExplicitFrequency || a.HPO != q.ID {
				continue
			}
			m.Ignored = false
			m.Relation = "exact"
			m.DiseaseTerm = a.HPO
			m.Frequency = a.Frequency
			m.LR = clamp(math.Max(falseNegativeFloor, 1-a.Frequency)/(1-bg), minimumLR, maximumLR)
			m.Conflict = a.Frequency >= .5
			if m.Conflict {
				m.LR = math.Min(1, m.LR)
			}
			return m
		}
		return m
	}
	p := falsePositiveFloor
	bestTier := 0
	for _, a := range aa {
		if a.Excluded {
			continue
		}
		candidate := falsePositiveFloor
		relation := "unrelated"
		tier := 0
		switch {
		case a.HPO == q.ID:
			candidate = a.Frequency
			relation = "exact"
			tier = 4
		case d.Graph.IsAncestor(q.ID, a.HPO):
			candidate = a.Frequency
			relation = "ancestor"
			tier = 3
		case d.Graph.IsAncestor(a.HPO, q.ID):
			branches := map[string]bool{}
			for _, x := range aa {
				if x.Excluded {
					continue
				}
				for _, id := range d.Graph.Ancestors(x.HPO) {
					term, _ := d.Graph.Term(id)
					for _, parent := range term.Parents {
						if parent == a.HPO {
							branches[id] = true
						}
					}
				}
				term, _ := d.Graph.Term(x.HPO)
				for _, parent := range term.Parents {
					if parent == a.HPO {
						branches[x.HPO] = true
					}
				}
			}
			candidate = a.Frequency / float64(1+len(branches)) * math.Pow(.5, float64(d.distance(q.ID, a.HPO)-1))
			relation = "descendant"
			tier = 2
		default:
			if lca, ok := d.Graph.ClosestCommonAncestor(q.ID, a.HPO); ok && d.Graph.Depth(lca) > 0 {
				candidate = a.Frequency * math.Pow(2, -float64(d.distance(q.ID, lca)))
				relation = "common_ancestor"
				tier = 1
			}
		}
		if tier > bestTier || (tier == bestTier && candidate > p) {
			p = candidate
			bestTier = tier
			m.Relation = relation
			m.DiseaseTerm = a.HPO
			m.Frequency = a.Frequency
		}
	}
	m.LR = clamp(p/bg, minimumLR, maximumLR)
	return m
}

func (d *Dataset) Match(in Input, candidates []string) (MatchResult, error) {
	out := MatchResult{SpecVersion: SpecVersion, DatasetVersion: d.Version, OntologyVersion: d.OntologyVersion, TranslationVersion: d.TranslationVersion}
	seen := map[string]bool{}
	terms := append([]Observation(nil), in.Terms...)
	for n, t := range terms {
		id, ok := d.Graph.Resolve(t.ID)
		if !ok {
			return out, errors.New("unknown or unresolved HPO term")
		}
		if seen[id] {
			return out, errors.New("duplicate canonical HPO term")
		}
		seen[id] = true
		terms[n].ID = id
		if t.Polarity == "" {
			terms[n].Polarity = "present"
		}
	}
	sort.Slice(terms, func(i, j int) bool { return terms[i].ID < terms[j].ID })
	seen = map[string]bool{}
	for _, id := range candidates {
		if seen[id] {
			continue
		}
		seen[id] = true
		if len(d.Diseases[id]) == 0 {
			out.Missing = append(out.Missing, "no explicit HPOA disease mapping: "+id)
			continue
		}
		s := DiseaseScore{DiseaseID: id}
		for _, t := range terms {
			m := d.match(t, id)
			if t.Polarity == "absent" && in.Source != "clinician_verified" {
				m.Ignored = true
				m.LR = 1
				m.Conflict = false
			}
			s.Terms = append(s.Terms, m)
			if !m.Ignored {
				s.LogLR += math.Log(m.LR)
				if t.Polarity == "present" && m.IC >= minInformationContent && m.LR > 1 {
					s.SpecificTerms++
				}
			}
		}
		s.LR = math.Exp(clamp(s.LogLR, -700, 700))
		out.Scores = append(out.Scores, s)
	}
	sort.Slice(out.Scores, func(i, j int) bool {
		if out.Scores[i].LogLR == out.Scores[j].LogLR {
			return out.Scores[i].DiseaseID < out.Scores[j].DiseaseID
		}
		return out.Scores[i].LogLR > out.Scores[j].LogLR
	})
	for n := range out.Scores {
		s := &out.Scores[n]
		s.Rank = n + 1
		for k := 0; k < n; k++ {
			if math.Abs(s.LogLR-out.Scores[k].LogLR) < 1e-12 {
				s.Rank = out.Scores[k].Rank
				break
			}
		}
		if len(out.Scores) > 1 {
			rival := 0
			if n == 0 {
				rival = 1
			}
			v := math.Exp(clamp(s.LogLR-out.Scores[rival].LogLR, -700, 700))
			s.Margin = &v
		}
	}
	return out, nil
}
