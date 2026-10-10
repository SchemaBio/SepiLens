package retrieval

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/pzweuj/sepilens/internal/evidence"
)

type Mode string

const (
	ModeOff      Mode = "off"
	ModeTargeted Mode = "targeted"
	ModeDeep     Mode = "deep"
)

var (
	ErrTimeoutExceeded = errors.New("retrieval budget timeout exceeded")
	ErrBudgetExceeded  = errors.New("retrieval request count budget exceeded")
)

type Query struct {
	Assembly    string `json:"assembly"`
	Chrom       string `json:"chrom"`
	Pos         int64  `json:"pos,omitempty"`
	Start       int64  `json:"start,omitempty"`
	End         int64  `json:"end,omitempty"`
	Ref         string `json:"ref,omitempty"`
	Alt         string `json:"alt,omitempty"`
	Gene        string `json:"gene,omitempty"`
	DiseaseID   string `json:"disease_id,omitempty"`
	HGVS        string `json:"hgvs,omitempty"`
	VariantType string `json:"variant_type"` // "snv", "cnv_loss", "cnv_gain"
	Mode        Mode   `json:"mode"`
}

type Retriever interface {
	Name() string
	IsOnline() bool
	Retrieve(ctx context.Context, q Query) ([]evidence.Source, error)
}

type CoordinatorConfig struct {
	MaxTargetedRequests int
	MaxDeepRequests     int
	Timeout             time.Duration
}

type SearchCoordinator struct {
	cfg        CoordinatorConfig
	retrievers []Retriever
	sanitizer  *evidence.Sanitizer
	verifier   *evidence.SourceVerifier
}

func NewSearchCoordinator(cfg CoordinatorConfig, retrievers []Retriever) *SearchCoordinator {
	if cfg.MaxTargetedRequests <= 0 {
		cfg.MaxTargetedRequests = 3
	}
	if cfg.MaxDeepRequests <= 0 {
		cfg.MaxDeepRequests = 10
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 15 * time.Second
	}
	return &SearchCoordinator{
		cfg:        cfg,
		retrievers: retrievers,
		sanitizer:  evidence.NewSanitizer(4000),
		verifier:   evidence.NewSourceVerifier(),
	}
}

func (sc *SearchCoordinator) Coordinate(ctx context.Context, q Query) (evidence.ProvenanceMetadata, []evidence.Source, error) {
	start := time.Now()
	timeoutCtx, cancel := context.WithTimeout(ctx, sc.cfg.Timeout)
	defer cancel()

	maxReqs := 0
	switch q.Mode {
	case ModeOff:
		maxReqs = 0
	case ModeDeep:
		maxReqs = sc.cfg.MaxDeepRequests
	default:
		// ModeTargeted is default
		maxReqs = sc.cfg.MaxTargetedRequests
	}

	var allSources []evidence.Source
	requestCount := 0

	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, r := range sc.retrievers {
		if q.Mode == ModeOff && r.IsOnline() {
			// Mode "off" strictly skips all online network retrievers
			continue
		}

		if r.IsOnline() && requestCount >= maxReqs {
			break
		}
		if r.IsOnline() {
			requestCount++
		}

		wg.Add(1)
		go func(ret Retriever) {
			defer wg.Done()
			sources, err := ret.Retrieve(timeoutCtx, q)
			if err == nil && len(sources) > 0 {
				mu.Lock()
				allSources = append(allSources, sources...)
				mu.Unlock()
			}
		}(r)
	}

	wg.Wait()

	// Deduplication and sanitation
	dedupMap := make(map[string]bool)
	var finalSources []evidence.Source

	for _, src := range allSources {
		key := src.PMID
		if key == "" {
			key = src.ClinVarID
		}
		if key == "" {
			key = src.URL
		}
		if key == "" {
			key = src.ID
		}

		if key != "" && dedupMap[key] {
			continue
		}
		if key != "" {
			dedupMap[key] = true
		}

		// Sanitize external snippet
		if src.Snippet != "" {
			src.Snippet = sc.sanitizer.SanitizeText(src.Snippet)
		}
		// Verify source status
		sc.verifier.VerifySource(&src)

		finalSources = append(finalSources, src)
	}

	prov := evidence.ProvenanceMetadata{
		RetrievalMode:      string(q.Mode),
		RetrievalTimestamp: time.Now().UTC().Format(time.RFC3339),
		DatasetVersions: map[string]string{
			"clingen_dosage": "2026-Q1",
			"clinvar":        "2026-10",
		},
		RequestsCount: requestCount,
		DurationMS:    time.Since(start).Milliseconds(),
	}

	return prov, finalSources, nil
}
