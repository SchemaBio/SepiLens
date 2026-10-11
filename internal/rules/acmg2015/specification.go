package acmg2015

import (
	"strings"
	"sync"
)

type Specification struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Gene           string   `json:"gene"`
	DiseaseID      string   `json:"disease_id"`
	Inheritance    string   `json:"inheritance"`
	BA1Threshold   float64  `json:"ba1_threshold"`
	BS1Threshold   float64  `json:"bs1_threshold"`
	PM2Threshold   float64  `json:"pm2_threshold"`
	PM2Strength    Strength `json:"pm2_strength"`
	PVS1Applicable bool     `json:"pvs1_applicable"`
	IsVCEP         bool     `json:"is_vcep"`
	Version        string   `json:"version"`
}

var Richards2015Baseline = Specification{
	ID:             "richards_2015_baseline",
	Name:           "ACMG/AMP 2015 Baseline Guidelines",
	Gene:           "*",
	DiseaseID:      "*",
	Inheritance:    "*",
	BA1Threshold:   0.05,
	BS1Threshold:   0.01,
	PM2Threshold:   0.0001,
	PM2Strength:    StrengthModerate,
	PVS1Applicable: false,
	IsVCEP:         false,
	Version:        "2015-baseline",
}

var CFTR_VCEP = Specification{
	ID:             "clingen_cftr_vcep_v1",
	Name:           "ClinGen CFTR Variant Curation Expert Panel Specification",
	Gene:           "CFTR",
	DiseaseID:      "MONDO:0009061", // Cystic Fibrosis
	Inheritance:    "AR",
	BA1Threshold:   0.05,
	BS1Threshold:   0.01,
	PM2Threshold:   0.00005,
	PM2Strength:    StrengthSupporting, // ClinGen SVI 2020: PM2 downgraded to Supporting
	PVS1Applicable: true,               // Loss of CFTR channel conductance causes CF
	IsVCEP:         true,
	Version:        "v1.0.0",
}

type SpecificationRegistry struct {
	mu    sync.RWMutex
	specs map[string]Specification
}

func NewSpecificationRegistry() *SpecificationRegistry {
	reg := &SpecificationRegistry{
		specs: make(map[string]Specification),
	}
	// Published VCEP packages must be imported and version-locked explicitly.
	return reg
}

func (r *SpecificationRegistry) Register(spec Specification) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := makeKey(spec.Gene, spec.DiseaseID, spec.Inheritance)
	r.specs[key] = spec
}

func (r *SpecificationRegistry) Resolve(gene, diseaseID, inheritance string) Specification {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Exact match: gene + disease + inheritance
	key := makeKey(gene, diseaseID, inheritance)
	if spec, ok := r.specs[key]; ok {
		return spec
	}

	return Richards2015Baseline
}

func makeKey(gene, diseaseID, inheritance string) string {
	return strings.ToUpper(strings.TrimSpace(gene)) + "|" +
		strings.ToUpper(strings.TrimSpace(diseaseID)) + "|" +
		strings.ToUpper(strings.TrimSpace(inheritance))
}
