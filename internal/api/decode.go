package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
)

// maxBodyBytes caps request bodies to protect against oversized payloads.
const maxBodyBytes = 1 << 20 // 1 MiB

// decodeJSON strictly decodes a single JSON request body into dst. It enforces a
// size limit, rejects unknown fields, and rejects trailing data. On failure it
// returns the HTTP status to send and a client-safe message.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) (int, string) {
	if ct := r.Header.Get("Content-Type"); ct != "" {
		mt, _, err := mime.ParseMediaType(ct)
		if err != nil || mt != "application/json" {
			return http.StatusUnsupportedMediaType, "Content-Type must be application/json"
		}
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		var syntax *json.SyntaxError
		var typeErr *json.UnmarshalTypeError
		var tooBig *http.MaxBytesError
		switch {
		case errors.As(err, &syntax):
			return http.StatusBadRequest, fmt.Sprintf("malformed JSON at position %d", syntax.Offset)
		case errors.Is(err, io.ErrUnexpectedEOF):
			return http.StatusBadRequest, "malformed JSON: the body ended early"
		case errors.As(err, &typeErr):
			return http.StatusBadRequest, fmt.Sprintf("field %q must be of type %s", typeErr.Field, typeErr.Type)
		case errors.Is(err, io.EOF):
			return http.StatusBadRequest, "the request body is empty"
		case strings.HasPrefix(err.Error(), "json: unknown field "):
			return http.StatusBadRequest, strings.TrimPrefix(err.Error(), "json: unknown field ")
		case errors.As(err, &tooBig):
			return http.StatusRequestEntityTooLarge, fmt.Sprintf("the body must not exceed %d bytes", tooBig.Limit)
		default:
			return http.StatusBadRequest, "invalid JSON"
		}
	}

	// A second value, or trailing garbage, means the client sent more than one
	// JSON document.
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return http.StatusBadRequest, "the body must contain a single JSON value"
	}
	return 0, ""
}
