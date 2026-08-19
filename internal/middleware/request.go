package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"github.com/zhanglei10281852-gif/ai/internal/requestmeta"
)

func RequestContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = newRequestID()
		}
		w.Header().Set("X-Request-ID", requestID)
		next.ServeHTTP(w, r.WithContext(requestmeta.WithRequestID(r.Context(), requestID)))
	})
}

func newRequestID() string {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "req-unknown"
	}
	return "req_" + hex.EncodeToString(raw[:])
}
