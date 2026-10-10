package hpo

type Term struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Definition   string `json:"definition"`
	NameCN       string `json:"name_cn"`
	DefinitionCN string `json:"definition_cn"`
}

type PreprocessResult struct {
	Symptoms         []string `json:"symptoms"`
	MedicalHistory   []string `json:"medical_history"`
	Diagnosis        []string `json:"diagnosis"`
	NegatedSymptoms  []string `json:"negated_symptoms"`
	FamilyHistory    []string `json:"family_history"`
	InferredSymptoms []string `json:"inferred_symptoms,omitempty"`
	RawExtraction    string   `json:"raw_extraction,omitempty"`
	DurationMS       int64    `json:"duration_ms,omitempty"`
}

type CandidateTerm struct {
	Term         Term     `json:"term"`
	Score        float64  `json:"score"`
	MatchedWords []string `json:"matched_words"`
	Source       string   `json:"source"`
}

type Confidence string

const (
	ConfidenceHigh   Confidence = "high"
	ConfidenceMedium Confidence = "medium"
	ConfidenceLow    Confidence = "low"
)

type Polarity string

const (
	PolarityPresent       Polarity = "present"
	PolarityAbsent        Polarity = "absent"
	PolarityUncertain     Polarity = "uncertain"
	PolarityFamilyHistory Polarity = "family_history"
	PolarityInferred      Polarity = "inferred"
)

type ExtractionItem struct {
	HPO             string     `json:"hpo"`
	Name            string     `json:"name"`
	NameCN          string     `json:"name_cn"`
	Definition      string     `json:"definition"`
	DefinitionCN    string     `json:"definition_cn"`
	Confidence      Confidence `json:"confidence"`
	Remark          string     `json:"remark"`
	OriginalSymptom string     `json:"original_symptom,omitempty"`
	Polarity        Polarity   `json:"polarity"`
	ReviewRequired  bool       `json:"review_required"`
}

type ExtractionResult struct {
	Terms          []ExtractionItem `json:"terms"`
	Preprocess     PreprocessResult `json:"preprocess"`
	CandidateCount int              `json:"candidate_count"`
	DurationMS     int64            `json:"duration_ms"`
}
