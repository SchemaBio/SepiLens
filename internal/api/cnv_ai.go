package api

import (
	"github.com/pzweuj/sepilens/internal/agent"
	"github.com/pzweuj/sepilens/internal/evidence"
	"github.com/pzweuj/sepilens/internal/rules/cnvgain2020"
	"github.com/pzweuj/sepilens/internal/rules/cnvloss2020"
	"github.com/pzweuj/sepilens/internal/rules/cnvrubric"
	"regexp"
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
	if p.CNVContext != nil {
		if p.CNVContext.DiseaseID != "" && !regexp.MustCompile(`^(MONDO|OMIM):[A-Za-z0-9_.-]+$`).MatchString(p.CNVContext.DiseaseID) {
			return agent.ErrOutputInvalid
		}
		valid := false
		for _, m := range []string{"unknown", "AD", "AR", "XL", "XLD", "XLR", "SD", "MT"} {
			valid = valid || p.CNVContext.Inheritance == m
		}
		if !valid {
			return agent.ErrOutputInvalid
		}
		for _, id := range p.CNVContext.SourceIDs {
			if !refs[id] {
				return agent.ErrOutputInvalid
			}
		}
	}
	seen := map[string]bool{}
	groups := map[string]string{}
	for _, c := range p.Criteria {
		d, ok := defs[c.Code]
		if !ok || seen[c.Code] {
			return agent.ErrOutputInvalid
		}
		seen[c.Code] = true
		if c.Score != nil && (*c.Score < d.Min || *c.Score > d.Max) {
			return agent.ErrOutputInvalid
		}
		if c.Status == "met" {
			if c.Score == nil || len(c.SourceIDs) == 0 || c.OverrideReason == "" {
				return agent.ErrOutputInvalid
			}
			for _, ref := range c.SourceIDs {
				if !refs[ref] {
					return agent.ErrOutputInvalid
				}
			}
			for _, fact := range d.Facts {
				if !c.Facts[fact] {
					return agent.ErrOutputInvalid
				}
			}
			if d.Group != "" && groups[d.Group] != "" {
				return agent.ErrOutputInvalid
			}
			groups[d.Group] = c.Code
		}
	}
	return nil
}
