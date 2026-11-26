package api

import (
	"net/http"
	"strconv"
)

// page is the envelope returned when pagination query params are present.
type page struct {
	Items  any `json:"items"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
	Total  int `json:"total"`
}

// parsePagination reads limit/offset. ok is false when neither is supplied, so
// existing list responses stay a bare array for backward compatibility.
func parsePagination(r *http.Request) (limit, offset int, ok bool) {
	q := r.URL.Query()
	if q.Get("limit") == "" && q.Get("offset") == "" {
		return 0, 0, false
	}
	limit = 50
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 200 {
		limit = 200
	}
	if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	return limit, offset, true
}
