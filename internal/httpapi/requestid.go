package httpapi

import (
	"context"
	crand "crypto/rand"
	"encoding/hex"
	"net/http"
)

type ctxKey string

const requestIDKey ctxKey = "request_id"
const headerRequestID = "X-Request-ID"

func WithRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(headerRequestID)
		if id == "" {
			id = newID()
		}
		w.Header().Set(headerRequestID, id)

		ctx := context.WithValue(r.Context(), requestIDKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func RequestID(r *http.Request) string {
	if v := r.Context().Value(requestIDKey); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func newID() string {
	b := make([]byte, 16)
	if _, err := crand.Reader.Read(b); err != nil {
		return "rid-fallback"
	}
	return hex.EncodeToString(b)
}
