// Package api provides transport primitives shared by JSON endpoints.
package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"net/http"
)

type traceKey struct{}

// TraceMiddleware assigns a private server-generated trace ID to one request.
func TraceMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var raw [16]byte
		if _, err := io.ReadFull(rand.Reader, raw[:]); err != nil {
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		traceID := hex.EncodeToString(raw[:])
		w.Header().Set("X-Trace-ID", traceID)
		ctx := context.WithValue(r.Context(), traceKey{}, traceID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// TraceID returns the server-generated request trace ID, if middleware ran.
func TraceID(ctx context.Context) string {
	traceID, _ := ctx.Value(traceKey{}).(string)
	return traceID
}
