package engine_test

import (
	"errors"
	"testing"

	"github.com/pzweuj/sepilens/internal/engine"
)

func TestEngineRegistry_CompletenessAndInvariants(t *testing.T) {
	all := engine.All()
	if len(all) != 5 {
		t.Fatalf("expected 5 engines, got %d", len(all))
	}

	seen := make(map[engine.EngineID]bool)
	for _, desc := range all {
		if seen[desc.ID] {
			t.Fatalf("duplicate engine registered: %s", desc.ID)
		}
		seen[desc.ID] = true

		if desc.ID == engine.EngineSVCv4 {
			if desc.Enabled {
				t.Fatalf("svc_v4 must not be enabled")
			}
			if desc.Status != engine.StatusExperimental || !desc.ExperimentalAvailable {
				t.Fatalf("svc_v4 must advertise experimental availability only, got %+v", desc)
			}
		}
	}

	if engine.EngineCNVLoss2020 == engine.EngineCNVGain2020 {
		t.Fatalf("cnv_loss and cnv_gain must have distinct identifiers")
	}

	for _, id := range []engine.EngineID{
		engine.EngineHPOExtract,
		engine.EngineACMG2015,
		engine.EngineSVCv4,
		engine.EngineCNVLoss2020,
		engine.EngineCNVGain2020,
	} {
		desc, err := engine.Lookup(id)
		if err != nil {
			t.Fatalf("lookup failed for canonical engine %s: %v", id, err)
		}
		if desc.ID != id {
			t.Fatalf("lookup mismatched id: expected %s, got %s", id, desc.ID)
		}
	}

	_, err := engine.Lookup("invalid_engine_id")
	if !errors.Is(err, engine.ErrUnknownEngine) {
		t.Fatalf("expected ErrUnknownEngine, got %v", err)
	}

	// Invariant: svc_v4 must NEVER be enableable
	errSVC := engine.EnableEngine(engine.EngineSVCv4, "hash-test")
	if !errors.Is(errSVC, engine.ErrEngineDisabled) {
		t.Fatalf("expected ErrEngineDisabled for svc_v4, got %v", errSVC)
	}

	// Enabling HPO extract updates its status
	errHPO := engine.EnableEngine(engine.EngineHPOExtract, "hpo-v1-deephpo")
	if !errors.Is(errHPO, engine.ErrRulesetNotReady) {
		t.Fatalf("unvalidated HPO enable bypass: %v", errHPO)
	}
	descHPO, _ := engine.Lookup(engine.EngineHPOExtract)
	if descHPO.Enabled || descHPO.Status != engine.StatusDisabled {
		t.Fatalf("unvalidated HPO enabled: %+v", descHPO)
	}
}
