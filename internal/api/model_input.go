package api

import "github.com/pzweuj/sepilens/internal/evidence"

func mergeSources(a, b []evidence.Source) []evidence.Source {
	byID := map[string]bool{}
	for _, s := range a {
		byID[s.ID] = true
	}
	for _, s := range b {
		if !byID[s.ID] {
			a = append(a, s)
			byID[s.ID] = true
		}
	}
	return a
}
