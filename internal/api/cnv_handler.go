package api

import "net/http"

func (s *Server) dispatchCNVJob(w http.ResponseWriter, r *http.Request, req *CNVEvaluationRequest, _ []byte) {
	s.submit(w, r, req.EngineID, envelope{CNV: req}, req.ClinicalContext.VariantOrigin, req.Options)
}
