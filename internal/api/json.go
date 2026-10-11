package api

import (
	"encoding/json"
	"io"
	"net/http"
)

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(v) != nil {
		WriteError(w, r, 400, ErrCodeBadRequest, "invalid JSON payload", false)
		return false
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		WriteError(w, r, 400, ErrCodeBadRequest, "one JSON value required", false)
		return false
	}
	return true
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
