package annotation

type Query struct {
	Assembly   string `json:"assembly"`
	Chrom      string `json:"chrom"`
	Pos        int64  `json:"pos"`
	Ref        string `json:"ref"`
	Alt        string `json:"alt"`
	Gene       string `json:"gene"`
	DiseaseID  string `json:"disease_id"`
	MOI        string `json:"moi"`
	Transcript string `json:"transcript,omitempty"`
	Protein    string `json:"protein_change,omitempty"`
}
