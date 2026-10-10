package api

import (
	"net/http"
)

func (s *Server) processProviderTest(w http.ResponseWriter, r *http.Request) {
	s.handleProviderTestLogic(w, r)
}

func (s *Server) processGetJob(w http.ResponseWriter, r *http.Request) {
	s.handleGetJobLogic(w, r)
}

func (s *Server) processJobEvents(w http.ResponseWriter, r *http.Request) {
	s.handleJobEventsLogic(w, r)
}

func (s *Server) processJobResult(w http.ResponseWriter, r *http.Request) {
	s.handleJobResultLogic(w, r)
}

func (s *Server) processCancelJob(w http.ResponseWriter, r *http.Request) {
	s.handleCancelJobLogic(w, r)
}

func (s *Server) processAddReview(w http.ResponseWriter, r *http.Request) {
	s.handleAddReviewLogic(w, r)
}
