package api

import (
	"net/http"
)

func (s *Server) handleProviderTest(w http.ResponseWriter, r *http.Request) {
	s.processProviderTest(w, r)
}

func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	s.processGetJob(w, r)
}

func (s *Server) handleJobEvents(w http.ResponseWriter, r *http.Request) {
	s.processJobEvents(w, r)
}

func (s *Server) handleJobResult(w http.ResponseWriter, r *http.Request) {
	s.processJobResult(w, r)
}

func (s *Server) handleCancelJob(w http.ResponseWriter, r *http.Request) {
	s.processCancelJob(w, r)
}

func (s *Server) handleAddReview(w http.ResponseWriter, r *http.Request) {
	s.processAddReview(w, r)
}
