package api

import (
	"errors"
	"fmt"
	"github.com/pzweuj/sepilens/internal/agent"
	"github.com/pzweuj/sepilens/internal/evidence"
	"github.com/pzweuj/sepilens/internal/llm"
	"github.com/pzweuj/sepilens/internal/rules/cnvgain2020"
	"github.com/pzweuj/sepilens/internal/rules/cnvloss2020"
	"github.com/pzweuj/sepilens/internal/rules/cnvrubric"
	"regexp"
	"strings"
)

type cnvContextProposal struct {
	DiseaseID   string   `json:"disease_id"`
	Inheritance string   `json:"inheritance"`
	Rationale   string   `json:"rationale"`
	SourceIDs   []string `json:"source_ids"`
}

func cnvDefinitions(id string) []cnvrubric.Definition {
	rules := cnvloss2020.Rules
	if id == "cnv_gain_2020" {
		rules = cnvgain2020.Rules
	}
	return cnvrubric.Parse(rules)
}
func validateCNVProposal(id string, p assessmentProposal, sources []evidence.Source) error {
	defs := map[string]cnvrubric.Definition{}
	for _, d := range cnvDefinitions(id) {
		defs[d.Code] = d
	}
	refs := map[string]bool{}
	for _, s := range sources {
		refs[s.ID] = s.IdentifierValid && s.RecordExists
	}
	var failures []string
	add := func(path, reason string) {
		if len(failures) < 24 {
			failures = append(failures, path+": "+reason)
		}
	}
	if p.CNVContext != nil {
		if p.CNVContext.DiseaseID != "" && !regexp.MustCompile(`^(MONDO|OMIM):[A-Za-z0-9_.-]+$`).MatchString(p.CNVContext.DiseaseID) {
			add("cnv_context.disease_id", "MONDO/OMIM identifier or empty string required")
		}
		valid := false
		for _, m := range []string{"unknown", "AD", "AR", "XL", "XLD", "XLR", "SD", "MT"} {
			valid = valid || p.CNVContext.Inheritance == m
		}
		if !valid {
			add("cnv_context.inheritance", "allowed inheritance or unknown required")
		}
		for _, ref := range p.CNVContext.SourceIDs {
			if !refs[ref] {
				add("cnv_context.source_ids", "only supplied traceable sources allowed; use empty list if none")
				break
			}
		}
	}
	seen := map[string]bool{}
	groups := map[string]string{}
	for i, c := range p.Criteria {
		path := fmt.Sprintf("criteria[%d]", i)
		d, ok := defs[c.Code]
		if !ok || seen[c.Code] {
			add(path+".code", "unique code from this engine rubric required")
			continue
		}
		seen[c.Code] = true
		if c.Score != nil && (*c.Score < d.Min || *c.Score > d.Max) {
			add(path+".score_hundredths", "outside this clause rubric bounds; use null for unassessed or unmet clauses")
		}
		if c.Status == "met" {
			if c.Score == nil || len(c.SourceIDs) == 0 || c.OverrideReason == "" {
				add(path, "met requires rubric score, supplied sources and rationale; otherwise use insufficient_data and null score")
			}
			for _, ref := range c.SourceIDs {
				if !refs[ref] {
					add(path+".source_ids", "met requires supplied traceable sources; otherwise use insufficient_data and null score")
					break
				}
			}
			for _, fact := range d.Facts {
				if !c.Facts[fact] {
					add(path+".facts_confirmed", "met requires every rubric prerequisite supported by evidence; never invent facts, use insufficient_data and null score when missing")
					break
				}
			}
			if d.Group != "" && groups[d.Group] != "" {
				add(path, "mutually exclusive met clauses; retain only the supported applicable clause")
			}
			groups[d.Group] = c.Code
		}
	}
	if len(failures) > 0 {
		return errors.Join(agent.ErrOutputInvalid, &llm.SchemaValidationError{Path: "$", Reason: strings.Join(failures, "; ")})
	}
	return nil
}

// Prepare an editable calculator draft from the valid subset. Rejected model
// assertions remain in encrypted run.Raw; neutral defaults never establish facts.
func defaultCNVProposal(id string, p assessmentProposal, sources []evidence.Source) (assessmentProposal, []string) {
	var warnings []string
	refs := map[string]bool{}
	for _, src := range sources {
		refs[src.ID] = src.IdentifierValid && src.RecordExists
	}
	if p.CNVContext != nil {
		cx := *p.CNVContext
		p.CNVContext = &cx
		if cx.DiseaseID != "" && !regexp.MustCompile(`^(MONDO|OMIM):[A-Za-z0-9_.-]+$`).MatchString(cx.DiseaseID) {
			p.CNVContext.DiseaseID = ""
			warnings = append(warnings, "cnv_context.disease_id: unknown default used")
		}
		p.CNVContext.SourceIDs = nil
		for _, ref := range cx.SourceIDs {
			if refs[ref] {
				p.CNVContext.SourceIDs = append(p.CNVContext.SourceIDs, ref)
			} else {
				warnings = append(warnings, "cnv_context.source_ids: unsupported citation omitted")
			}
		}
	}
	defs := map[string]cnvrubric.Definition{}
	for _, d := range cnvDefinitions(id) {
		defs[d.Code] = d
	}
	counts := map[string]int{}
	groups := map[string]int{}
	for _, c := range p.Criteria {
		counts[c.Code]++
		if c.Status == "met" && defs[c.Code].Group != "" {
			groups[defs[c.Code].Group]++
		}
	}
	out := make([]assessmentChange, 0, len(p.Criteria))
	seen := map[string]bool{}
	for i, c := range p.Criteria {
		d, ok := defs[c.Code]
		if !ok || seen[c.Code] {
			warnings = append(warnings, fmt.Sprintf("criteria[%d]: unknown or duplicate clause omitted", i))
			continue
		}
		seen[c.Code] = true
		invalid := counts[c.Code] > 1 || (c.Status == "met" && d.Group != "" && groups[d.Group] > 1)
		if err := validateCNVProposal(id, assessmentProposal{Criteria: []assessmentChange{c}}, sources); err != nil {
			invalid = true
			warnings = append(warnings, fmt.Sprintf("criteria[%d]: invalid clause replaced with unknown default", i))
		}
		if invalid {
			c.Status = "insufficient_data"
			c.Score = nil
			c.SourceIDs = nil
			c.Facts = map[string]bool{}
			c.Change = "downgraded"
			warnings = append(warnings, fmt.Sprintf("criteria[%d]: neutral default applied", i))
		} else if c.Status != "met" {
			c.Score = nil
			c.Facts = map[string]bool{}
		}
		out = append(out, c)
	}
	p.Criteria = out
	return p, warnings
}
