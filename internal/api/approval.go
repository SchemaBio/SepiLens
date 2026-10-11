package api

import (
	"encoding/json"
	"errors"
	"github.com/pzweuj/sepilens/internal/engine"
	"github.com/pzweuj/sepilens/internal/validation"
	"os"
)

func (s *Server) LoadRuleApproval(approvalPath, validationPath, publicKey string) error {
	b, e := os.ReadFile(approvalPath)
	if e != nil {
		return e
	}
	var header engine.Approval
	if json.Unmarshal(b, &header) != nil {
		return errors.New("invalid approval")
	}
	content, codes, e := validation.Rules(header.EngineID)
	if e != nil {
		return e
	}
	reportBytes, e := os.ReadFile(validationPath)
	if e != nil {
		return e
	}
	var report validation.Report
	if json.Unmarshal(reportBytes, &report) != nil || report.EngineID != header.EngineID || report.RulesetHash != engine.Hash(content) || len(report.Cases) == 0 || report.DatasetHash == "unavailable" || report.DatasetHash != s.datasetHash() {
		return engine.ErrRulesetNotReady
	}
	known := map[string]validation.CaseResult{}
	for _, c := range report.Cases {
		if !c.Passed || c.ID == "" || c.Source == "" || c.Reviewer == "" {
			return engine.ErrRulesetNotReady
		}
		if _, exists := known[c.ID]; exists {
			return engine.ErrRulesetNotReady
		}
		known[c.ID] = c
	}
	for _, code := range codes {
		kinds := map[string]bool{}
		seen := map[string]bool{}
		for _, id := range report.Coverage[code] {
			c, ok := known[id]
			if !ok || seen[id] {
				return engine.ErrRulesetNotReady
			}
			seen[id] = true
			kinds[c.Kind] = true
		}
		for _, kind := range validation.RequiredKinds {
			if !kinds[kind] {
				return engine.ErrRulesetNotReady
			}
		}
	}
	binary, e := os.Executable()
	if e != nil {
		return e
	}
	binaryData, e := os.ReadFile(binary)
	if e != nil {
		return e
	}
	a, e := engine.LoadApproval(approvalPath, publicKey, engine.Hash(binaryData), content, reportBytes)
	if e != nil {
		return e
	}
	if s.approvals == nil {
		s.approvals = map[engine.EngineID]engine.Approval{}
	}
	s.approvals[a.EngineID] = a
	return nil
}
