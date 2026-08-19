package middleware

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zhanglei10281852-gif/ai/internal/requestmeta"
)

func TestRequestContextGeneratesAndPreservesID(t *testing.T) {
	handler := RequestContext(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(requestmeta.RequestID(r.Context())))
	}))
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	generated := response.Header().Get("X-Request-ID")
	if !strings.HasPrefix(generated, "req_") || response.Body.String() != generated {
		t.Fatalf("generated header=%q body=%q", generated, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("X-Request-ID", "req-client")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Header().Get("X-Request-ID") != "req-client" || response.Body.String() != "req-client" {
		t.Fatalf("preserved header=%q body=%q", response.Header().Get("X-Request-ID"), response.Body.String())
	}
}

func TestRecoveryConvertsPanicToJSON(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	handler := Recovery(logger, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") }))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/panic", nil))
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", response.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if body["error"].(map[string]any)["code"] != "internal_error" {
		t.Fatalf("body = %+v", body)
	}
	if !strings.Contains(logs.String(), "http panic recovered") || !strings.Contains(logs.String(), "/panic") {
		t.Fatalf("logs = %s", logs.String())
	}
}

func TestLoggingCapturesStatusAndPath(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	handler := Logging(logger, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	request := httptest.NewRequest(http.MethodPost, "/jobs", nil)
	request = request.WithContext(requestmeta.WithRequestID(request.Context(), "req-log"))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d", response.Code)
	}
	for _, fragment := range []string{"req-log", "POST", "/jobs", "202"} {
		if !strings.Contains(logs.String(), fragment) {
			t.Fatalf("logs %q missing %q", logs.String(), fragment)
		}
	}
}

func TestStatusWriterDefaultsToOK(t *testing.T) {
	response := httptest.NewRecorder()
	writer := &statusWriter{ResponseWriter: response, status: http.StatusOK}
	_, _ = writer.Write([]byte("ok"))
	if writer.status != http.StatusOK || response.Code != http.StatusOK {
		t.Fatalf("writer status=%d response=%d", writer.status, response.Code)
	}
}
