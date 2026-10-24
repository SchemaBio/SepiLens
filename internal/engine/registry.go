package engine

import (
	"errors"
	"fmt"
	"github.com/pzweuj/sepilens/internal/rules/svcv4"
	"sync"
)

type EngineID string

const (
	EngineHPOExtract  EngineID = "hpo_extract"
	EngineACMG2015    EngineID = "acmg_amp_2015"
	EngineSVCv4       EngineID = "svc_v4"
	EngineCNVLoss2020 EngineID = "cnv_loss_2020"
	EngineCNVGain2020 EngineID = "cnv_gain_2020"
)

type Status string

const (
	StatusRegisteredNotImplemented Status = "registered_not_implemented"
	StatusDisabled                 Status = "disabled"
	StatusEnabled                  Status = "enabled"
	StatusExperimental             Status = "experimental"
)

type Descriptor struct {
	AIAvailable           bool     `json:"ai_available"`
	ExperimentalAvailable bool     `json:"experimental_available"`
	ID                    EngineID `json:"id"`
	Name                  string   `json:"name"`
	GuidelineVersion      string   `json:"guideline_version"`
	TargetType            string   `json:"target_type"`
	Status                Status   `json:"status"`
	Enabled               bool     `json:"enabled"`
	RulesetHash           string   `json:"ruleset_hash"`
}

var (
	ErrUnknownEngine   = errors.New("unknown engine id")
	ErrEngineDisabled  = errors.New("engine is disabled by gate policy")
	ErrRulesetNotReady = errors.New("ruleset is not ready for clinical evaluation")
)

var (
	registryMu sync.RWMutex
	canonical  = map[EngineID]Descriptor{
		EngineHPOExtract: {
			ID:               EngineHPOExtract,
			Name:             "HPO Term Extraction",
			GuidelineVersion: "HPO-2026",
			TargetType:       "phenotype",
			Status:           StatusDisabled,
			Enabled:          false,
			RulesetHash:      "hpo-extract-v0",
		},
		EngineACMG2015: {
			ID:               EngineACMG2015,
			Name:             "ACMG/AMP 2015 Sequence Variant Interpretation",
			GuidelineVersion: "2015",
			TargetType:       "snv_indel",
			Status:           StatusDisabled,
			Enabled:          false,
			RulesetHash:      "acmg-2015-v1",
		},
		EngineSVCv4: {
			ID:                    EngineSVCv4,
			Name:                  "SVC v4 Sequence Variant Classification",
			GuidelineVersion:      "draft-reference-ef66faff51a265fef7b5c4e6439905f3aa540c46",
			TargetType:            "snv_indel",
			Status:                StatusExperimental,
			Enabled:               false,
			ExperimentalAvailable: true,
			RulesetHash:           svcv4.RulesetHash(),
		},
		EngineCNVLoss2020: {
			ID:               EngineCNVLoss2020,
			Name:             "ClinGen/ACMG 2020 Constitutional CNV Loss Rubric",
			GuidelineVersion: "2020",
			TargetType:       "cnv_loss",
			Status:           StatusDisabled,
			Enabled:          false,
			RulesetHash:      "cnv-loss-2020-v1",
		},
		EngineCNVGain2020: {
			ID:               EngineCNVGain2020,
			Name:             "ClinGen/ACMG 2020 Constitutional CNV Gain Rubric",
			GuidelineVersion: "2020",
			TargetType:       "cnv_gain",
			Status:           StatusDisabled,
			Enabled:          false,
			RulesetHash:      "cnv-gain-2020-v1",
		},
	}
)

func Lookup(id EngineID) (Descriptor, error) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	d, ok := canonical[id]
	if !ok {
		return Descriptor{}, fmt.Errorf("%w: %s", ErrUnknownEngine, id)
	}
	return d, nil
}

func EnableEngine(id EngineID, hash string) error {
	registryMu.Lock()
	defer registryMu.Unlock()

	if id == EngineSVCv4 {
		return ErrEngineDisabled
	}
	_, ok := canonical[id]
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownEngine, id)
	}
	return ErrRulesetNotReady
}

func All() []Descriptor {
	registryMu.RLock()
	defer registryMu.RUnlock()

	order := []EngineID{
		EngineHPOExtract,
		EngineACMG2015,
		EngineSVCv4,
		EngineCNVLoss2020,
		EngineCNVGain2020,
	}
	out := make([]Descriptor, 0, len(order))
	for _, id := range order {
		out = append(out, canonical[id])
	}
	return out
}
