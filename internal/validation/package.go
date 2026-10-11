package validation

import (
	"encoding/json"
	"fmt"
	"github.com/pzweuj/sepilens/internal/engine"
	"github.com/pzweuj/sepilens/internal/rules/acmg2015"
	"github.com/pzweuj/sepilens/internal/rules/cnvgain2020"
	"github.com/pzweuj/sepilens/internal/rules/cnvloss2020"
	"github.com/pzweuj/sepilens/internal/rules/cnvrubric"
	"github.com/pzweuj/sepilens/internal/rules/svcv4"
	"os"
	"path/filepath"
	"sort"
)

var RequiredKinds = []string{"positive", "negative", "missing", "boundary", "mutual_exclusion", "cap"}

func Rules(id engine.EngineID) ([]byte, []string, error) {
	var content []byte
	var codes []string
	var e error
	switch id {
	case engine.EngineSVCv4:
		content = svcv4.RulePackage()
		codes = svcv4.DeclaredCodes()
	case engine.EngineACMG2015:
		content, e = json.Marshal(map[string]any{"criteria": acmg2015.CanonicalCriteria, "required_facts": acmg2015.RequiredFacts})
		for c := range acmg2015.CanonicalCriteria {
			codes = append(codes, string(c))
		}
	case engine.EngineCNVLoss2020:
		content = cnvloss2020.Rules
	case engine.EngineCNVGain2020:
		content = cnvgain2020.Rules
	default:
		return nil, nil, engine.ErrEngineDisabled
	}
	if len(codes) == 0 {
		for _, d := range cnvrubric.Parse(content) {
			codes = append(codes, d.Code)
		}
	}
	sort.Strings(codes)
	return content, codes, e
}
func Export(dir string) error {
	for _, id := range []engine.EngineID{engine.EngineACMG2015, engine.EngineCNVLoss2020, engine.EngineCNVGain2020, engine.EngineSVCv4} {
		content, codes, e := Rules(id)
		if e != nil {
			return e
		}
		dest := filepath.Join(dir, string(id))
		if e = os.MkdirAll(dest, 0700); e != nil {
			return e
		}
		if e = os.WriteFile(filepath.Join(dest, "rules.json"), content, 0600); e != nil {
			return e
		}
		source := "https://pmc.ncbi.nlm.nih.gov/articles/PMC4544753/"
		if id == engine.EngineCNVLoss2020 {
			source = "https://cnvcalc.clinicalgenome.org/cnvcalc/cnv-loss"
		}
		if id == engine.EngineCNVGain2020 {
			source = "https://cnvcalc.clinicalgenome.org/cnvcalc/cnv-gain"
		}
		if id == engine.EngineSVCv4 {
			source = "https://github.com/clingen-data-model/svcv4-model/tree/" + svcv4.Commit
		}
		matrix := []map[string]any{}
		for _, code := range codes {
			for _, kind := range RequiredKinds {
				matrix = append(matrix, map[string]any{"code": code, "kind": kind, "status": "pending_independent_validation", "case_ids": []string{}})
			}
		}
		descriptor, _ := engine.Lookup(id)
		b, _ := json.MarshalIndent(map[string]any{"engine_id": id, "version": descriptor.GuidelineVersion, "scope": descriptor.TargetType + "; constitutional germline; GRCh37/GRCh38", "ruleset_hash": engine.Hash(content), "source": source, "clinical_status": "disabled", "experimental_available": id == engine.EngineSVCv4, "validation_report_hash": nil, "approval": nil, "case_matrix": matrix}, "", "  ")
		if e = os.WriteFile(filepath.Join(dest, "manifest.json"), b, 0600); e != nil {
			return e
		}
		fmt.Printf("%s %s\n", id, engine.Hash(content))
	}
	return nil
}
