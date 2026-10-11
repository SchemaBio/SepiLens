package api

import "net/http"

func (s *Server) dispatchACMG2015Job(w http.ResponseWriter, r *http.Request, req *SNVEvaluationRequest, _ []byte) {
	s.submit(w, r, req.EngineID, envelope{SNV: req}, req.ClinicalContext.VariantOrigin, req.Options)
}
