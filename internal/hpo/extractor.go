package hpo

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/pzweuj/sepilens/internal/llm"
)

var (
	ErrEmptyInput  = errors.New("clinical text input cannot be empty")
	ErrIndexNotSet = errors.New("hpo index is not loaded")
)

type Extractor struct {
	index           *Index
	defaultProvider llm.Provider
}

func NewExtractor(index *Index, defaultProvider llm.Provider) *Extractor {
	return &Extractor{
		index:           index,
		defaultProvider: defaultProvider,
	}
}

type ExtractOptions struct {
	Provider      llm.Provider
	Model         string
	MaxCandidates int
	OnStage       func(stage string, payload any)
}

func (e *Extractor) Extract(ctx context.Context, text string, opts ExtractOptions) (ExtractionResult, error) {
	if e.index == nil {
		return ExtractionResult{}, ErrIndexNotSet
	}
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ExtractionResult{}, ErrEmptyInput
	}

	start := time.Now()
	provider := opts.Provider
	if provider == nil {
		provider = e.defaultProvider
	}
	maxCandidates := opts.MaxCandidates
	if maxCandidates <= 0 {
		maxCandidates = 50
	}

	// If no LLM provider is available, use rule-based fallback
	if provider == nil {
		return e.ruleBasedExtract(trimmed, maxCandidates, start, opts.OnStage)
	}

	// 1. Stage 1: LLM Preprocessing
	if opts.OnStage != nil {
		opts.OnStage("preprocessing", map[string]string{
			"message": "正在使用大语言模型提取症状与否定实体...",
		})
	}

	genReq := llm.GenerateRequest{
		Model:  opts.Model,
		System: PreprocessSystemPrompt,
		Prompt: trimmed,
	}

	prepGen, err := provider.Generate(ctx, genReq)
	if err != nil {
		return e.ruleBasedExtract(trimmed, maxCandidates, start, opts.OnStage)
	}

	prepResult, err := ParsePreprocessJSON(prepGen.Content)
	if err != nil {
		posWords, negWords := parseRuleBasedClauses(trimmed)
		prepResult = PreprocessResult{
			Symptoms:        posWords,
			NegatedSymptoms: negWords,
		}
	}
	prepResult.DurationMS = time.Since(start).Milliseconds()

	if opts.OnStage != nil {
		opts.OnStage("preprocess_done", prepResult)
	}

	// 2. Stage 2: Local Search & Synonym Expansion
	if opts.OnStage != nil {
		opts.OnStage("searching", map[string]string{
			"message": "正在本地知识库检索候选 HPO 术语与同义词...",
		})
	}

	keywords := ExtractQueryKeywords(prepResult)
	if len(keywords) == 0 {
		posWords, _ := parseRuleBasedClauses(trimmed)
		keywords = posWords
	}

	candidates := e.index.SearchCandidates(keywords, maxCandidates)
	if opts.OnStage != nil {
		opts.OnStage("candidates_found", map[string]int{
			"count": len(candidates),
		})
	}

	if len(candidates) == 0 {
		return ExtractionResult{
			Terms:          nil,
			Preprocess:     prepResult,
			CandidateCount: 0,
			DurationMS:     time.Since(start).Milliseconds(),
		}, nil
	}

	// 3. Stage 3: LLM Candidate Matching
	if opts.OnStage != nil {
		opts.OnStage("matching", map[string]string{
			"message": "正在候选术语集中精确匹配最佳 HPO 术语...",
		})
	}

	candidateTable := FormatCandidateTable(candidates)
	matchSystem := BuildMatchSystemPrompt(candidateTable)
	matchPrompt := fmt.Sprintf("# 原始病历文本\n%s\n\n# 提取的阳性症状与诊断\n%s", trimmed, strings.Join(keywords, "、"))

	matchReq := llm.GenerateRequest{
		Model:  opts.Model,
		System: matchSystem,
		Prompt: matchPrompt,
	}

	matchGen, err := provider.Generate(ctx, matchReq)
	if err != nil {
		return e.heuristicMatchCandidates(candidates, prepResult, start)
	}

	items, err := ParseMatchResponse(matchGen.Content, e.index)
	if err != nil || len(items) == 0 {
		return e.heuristicMatchCandidates(candidates, prepResult, start)
	}

	// Negation safety filter
	negatedSet := make(map[string]bool)
	for _, neg := range prepResult.NegatedSymptoms {
		negatedSet[strings.ToLower(strings.TrimSpace(neg))] = true
	}

	var filtered []ExtractionItem
	for _, it := range items {
		itName := strings.ToLower(it.Name)
		itCn := strings.ToLower(it.NameCN)
		origSym := strings.ToLower(it.OriginalSymptom)
		if negatedSet[itName] || negatedSet[itCn] || (origSym != "" && negatedSet[origSym]) {
			continue
		}
		filtered = append(filtered, it)
	}

	if opts.OnStage != nil {
		opts.OnStage("completed", map[string]int{
			"matched_count": len(filtered),
		})
	}

	return ExtractionResult{
		Terms:          filtered,
		Preprocess:     prepResult,
		CandidateCount: len(candidates),
		DurationMS:     time.Since(start).Milliseconds(),
	}, nil
}

var reNegPatterns = regexp.MustCompile(`(?:非|无|未见|未出现|否认|不伴|未查见)([\p{Han}A-Za-z0-9]{2,12})`)

func parseRuleBasedClauses(text string) (posWords []string, negWords []string) {
	// 1. Scan for all negated entities
	for _, match := range reNegPatterns.FindAllStringSubmatch(text, -1) {
		if len(match) > 1 {
			negCore := strings.TrimSpace(match[1])
			// Trim suffix like "型" if from "非ST段抬高型"
			negCore = strings.TrimSuffix(negCore, "型")
			if len([]rune(negCore)) >= 2 {
				negWords = append(negWords, negCore)
			}
		}
	}

	// 2. Scan clauses for positive entities
	clauses := rePunct.Split(text, -1)
	negPrefixes := []string{"无", "未见", "否认", "未出现", "不伴", "排除", "未查见"}

	for _, c := range clauses {
		c = strings.TrimSpace(c)
		if len([]rune(c)) < 2 {
			continue
		}

		isPureNeg := false
		for _, pref := range negPrefixes {
			if strings.HasPrefix(c, pref) {
				isPureNeg = true
				break
			}
		}
		if isPureNeg {
			continue
		}

		// Handle embedded negations like 非ST段抬高型心肌梗死 -> 心肌梗死
		if strings.Contains(c, "非") {
			parts := strings.Split(c, "非")
			if len(parts) > 1 {
				after := parts[1]
				// if contains "型心肌梗死", split "型"
				if idx := strings.Index(after, "型"); idx != -1 && idx+len("型") < len(after) {
					posWords = append(posWords, after[idx+len("型"):])
				}
			}
		}

		posWords = append(posWords, c)
	}
	return deduplicateStrings(posWords), deduplicateStrings(negWords)
}

func (e *Extractor) ruleBasedExtract(text string, maxCandidates int, start time.Time, onStage func(string, any)) (ExtractionResult, error) {
	if onStage != nil {
		onStage("searching", map[string]string{"message": "正在执行纯本地索引规则抽取..."})
	}

	posWords, negWords := parseRuleBasedClauses(text)
	prep := PreprocessResult{
		Symptoms:        posWords,
		NegatedSymptoms: negWords,
		DurationMS:      time.Since(start).Milliseconds(),
	}

	candidates := e.index.SearchCandidates(posWords, maxCandidates)
	return e.heuristicMatchCandidates(candidates, prep, start)
}

func (e *Extractor) heuristicMatchCandidates(candidates []CandidateTerm, prep PreprocessResult, start time.Time) (ExtractionResult, error) {
	negatedSet := make(map[string]bool)
	for _, neg := range prep.NegatedSymptoms {
		neg = strings.ToLower(strings.TrimSpace(neg))
		if neg != "" {
			negatedSet[neg] = true
			if strings.Contains(neg, "型") {
				parts := strings.Split(neg, "型")
				if len([]rune(parts[0])) >= 2 {
					negatedSet[parts[0]] = true
				}
			}
		}
	}

	var items []ExtractionItem
	limit := 10

	for _, c := range candidates {
		if len(items) >= limit {
			break
		}

		itName := strings.ToLower(c.Term.Name)
		itCn := strings.ToLower(c.Term.NameCN)

		isNegated := false
		if negatedSet[itName] || negatedSet[itCn] {
			isNegated = true
		}
		if !isNegated {
			for neg := range negatedSet {
				if itCn != "" && strings.Contains(neg, itCn) {
					isNegated = true
					break
				}
				if itName != "" && strings.Contains(neg, itName) {
					isNegated = true
					break
				}
			}
		}

		// Check if any matched word is in negatedSet
		if !isNegated {
			for _, mw := range c.MatchedWords {
				mwLower := strings.ToLower(mw)
				if negatedSet[mwLower] {
					isNegated = true
					break
				}
				for neg := range negatedSet {
					if mwLower != "" && strings.Contains(neg, mwLower) {
						isNegated = true
						break
					}
				}
				if isNegated {
					break
				}
			}
		}

		if isNegated {
			continue
		}

		conf := ConfidenceMedium
		if c.Score >= 20.0 {
			conf = ConfidenceHigh
		} else if c.Score < 2.0 {
			conf = ConfidenceLow
		}

		matchedWord := ""
		if len(c.MatchedWords) > 0 {
			matchedWord = c.MatchedWords[0]
		}

		items = append(items, ExtractionItem{
			HPO:             c.Term.ID,
			Name:            c.Term.Name,
			NameCN:          c.Term.NameCN,
			Definition:      c.Term.Definition,
			DefinitionCN:    c.Term.DefinitionCN,
			Confidence:      conf,
			Remark:          fmt.Sprintf("【%s】→ 本地索引召回匹配", matchedWord),
			OriginalSymptom: matchedWord,
			Polarity:        PolarityPresent,
			ReviewRequired:  true,
		})
	}

	return ExtractionResult{
		Terms:          items,
		Preprocess:     prep,
		CandidateCount: len(candidates),
		DurationMS:     time.Since(start).Milliseconds(),
	}, nil
}
