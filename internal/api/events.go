package api

import (
	"fmt"
	"github.com/go-chi/chi/v5"
	"github.com/pzweuj/sepilens/internal/jobs"
	"net/http"
	"strconv"
	"time"
)

func (s *Server) handleJobEventsLogic(w http.ResponseWriter, r *http.Request) {
	tenant, id := GetTenantID(r.Context()), chi.URLParam(r, "job_id")
	if _, e := s.store.GetJob(r.Context(), tenant, id); e != nil {
		WriteError(w, r, 404, ErrCodeJobNotFound, "job not found", false)
		return
	}
	after := 0
	if raw := r.Header.Get("Last-Event-ID"); raw != "" {
		v, e := strconv.Atoi(raw)
		if e != nil || v < 0 {
			WriteError(w, r, 400, ErrCodeBadRequest, "invalid event cursor", false)
			return
		}
		after = v
	}
	flush, ok := w.(http.Flusher)
	if !ok {
		WriteError(w, r, 500, ErrCodeInternalError, "stream unavailable", false)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(200)
	flush.Flush()
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		events, e := s.store.GetEvents(r.Context(), id, after)
		if e != nil {
			return
		}
		for _, ev := range events {
			if _, e = fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", ev.Seq, ev.Stage, ev.PayloadJSON); e != nil {
				return
			}
			after = ev.Seq
		}
		flush.Flush()
		j, e := s.store.GetJob(r.Context(), tenant, id)
		if e != nil {
			return
		}
		if j.Status == jobs.StatusSucceeded || j.Status == jobs.StatusFailed || j.Status == jobs.StatusCancelled {
			if len(events) == 0 {
				fmt.Fprintf(w, "event: terminal\ndata: {\"status\":%q}\n\n", j.Status)
				flush.Flush()
			}
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-tick.C:
		case <-heartbeat.C:
			if _, e = fmt.Fprint(w, ": heartbeat\n\n"); e != nil {
				return
			}
			flush.Flush()
		}
	}
}
