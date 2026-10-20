package api

import (
	"encoding/json"
	"errors"
	"github.com/pzweuj/sepilens/internal/strictjson"
	"io"
	"net/http"
)

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	b, err := io.ReadAll(r.Body)
	if err != nil {
		var size *http.MaxBytesError
		if errors.As(err, &size) {
			WriteError(w, r, 413, ErrCodeBadRequest, "payload too large", false)
		} else {
			WriteError(w, r, 400, ErrCodeBadRequest, "invalid request body", false)
		}
		return false
	}
	if strictjson.Decode(b, v) != nil {
		WriteError(w, r, 400, ErrCodeBadRequest, "invalid or ambiguous JSON", false)
		return false
	}

	return true
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
