package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zhanglei10281852-gif/ai/internal/clock"
	"github.com/zhanglei10281852-gif/ai/internal/domain"
	"github.com/zhanglei10281852-gif/ai/internal/repository"
	"github.com/zhanglei10281852-gif/ai/internal/requestmeta"
	"github.com/zhanglei10281852-gif/ai/internal/service"
	"github.com/zhanglei10281852-gif/ai/internal/storage/sqlite"
	"golang.org/x/crypto/bcrypt"
)

type httpFixture struct {
	server   *httptest.Server
	store    *sqlite.Store
	services *service.Services
	token    string
}

func newHTTPFixture(t *testing.T) *httpFixture {
	t.Helper()
	ctx := context.Background()
	now := time.Date(2026, 8, 18, 8, 0, 0, 0, time.UTC)
	store, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "http.db"))
	if err != nil {
		t.Fatal(err)
	}
	fixed := clock.NewFixed(now)
	services := service.New(store, fixed, 4*time.Hour, 30*time.Minute)
	hash, err := bcrypt.GenerateFromPassword([]byte("very-secure-password"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	user := domain.User{ID: "usr_http", Email: "ops@example.test", DisplayName: "HTTP Ops", PasswordHash: string(hash), Role: domain.RoleMLEngineer, Status: domain.UserActive, Version: 1, CreatedAt: now, UpdatedAt: now}
	if err := store.WithTx(ctx, func(tx repository.Tx) error { return tx.InsertUser(ctx, user) }); err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(New(services, store, nil))
	fixture := &httpFixture{server: httpServer, store: store, services: services}
	t.Cleanup(func() { httpServer.Close(); _ = store.Close() })
	return fixture
}

func (f *httpFixture) request(t *testing.T, method, path string, body any, token string) *http.Response {
	t.Helper()
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	req, err := http.NewRequest(method, f.server.URL+path, bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func readResponse(t *testing.T, response *http.Response) map[string]any {
	t.Helper()
	defer response.Body.Close()
	var body map[string]any
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body
}

func TestHealthAndReadyEndpoints(t *testing.T) {
	f := newHTTPFixture(t)
	for _, path := range []string{"/healthz", "/readyz"} {
		response := f.request(t, http.MethodGet, path, nil, "")
		if response.StatusCode != http.StatusOK {
			t.Fatalf("%s status = %d", path, response.StatusCode)
		}
		if response.Header.Get("X-Request-ID") == "" {
			t.Fatalf("%s missing request id", path)
		}
		_ = readResponse(t, response)
	}
}

func TestProtectedEndpointRequiresBearerToken(t *testing.T) {
	f := newHTTPFixture(t)
	response := f.request(t, http.MethodPost, "/api/v1/workspaces", map[string]any{}, "")
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d", response.StatusCode)
	}
	body := readResponse(t, response)
	if !strings.Contains(body["error"].(map[string]any)["code"].(string), "unauthenticated") {
		t.Fatalf("body = %+v", body)
	}
}

func TestLoginReturnsTokenAndCreatesWorkspace(t *testing.T) {
	f := newHTTPFixture(t)
	response := f.request(t, http.MethodPost, "/api/v1/auth/login", map[string]any{"email": "ops@example.test", "password": "very-secure-password"}, "")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d", response.StatusCode)
	}
	body := readResponse(t, response)
	token, ok := body["token"].(string)
	if !ok || token == "" {
		t.Fatalf("login body = %+v", body)
	}
	response = f.request(t, http.MethodPost, "/api/v1/workspaces", map[string]any{"code": "MESH-HTTP", "name": "HTTP workspace", "minimum_score": 0.8, "maximum_score": 0.99, "max_execution_hours": 12, "review_hours": 4, "business_timezone": "Asia/Shanghai"}, token)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create workspace status = %d", response.StatusCode)
	}
	created := readResponse(t, response)
	if created["code"] != "MESH-HTTP" || created["status"] != string(domain.WorkspaceDraft) {
		t.Fatalf("created workspace = %+v", created)
	}
}

func TestLoginRejectsInvalidCredentials(t *testing.T) {
	f := newHTTPFixture(t)
	response := f.request(t, http.MethodPost, "/api/v1/auth/login", map[string]any{"email": "ops@example.test", "password": "wrong-password"}, "")
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d", response.StatusCode)
	}
	_ = readResponse(t, response)
}

func TestCreateUserRequiresMLEngineerRole(t *testing.T) {
	f := newHTTPFixture(t)
	ctx := context.Background()
	adminCtx := requestmeta.WithPrincipal(ctx, domain.Principal{UserID: "bootstrap-admin", Role: domain.RoleMLEngineer})
	if _, err := f.services.Auth.CreateUser(adminCtx, "data@example.test", "Data Engineer", "very-secure-password", domain.RoleDataEngineer); err != nil {
		t.Fatal(err)
	}
	login, err := f.services.Auth.Login(ctx, service.LoginInput{Email: "data@example.test", Password: "very-secure-password"})
	if err != nil {
		t.Fatal(err)
	}

	response := f.request(t, http.MethodPost, "/api/v1/users", map[string]any{
		"email":        "reviewer@example.test",
		"display_name": "Reviewer",
		"password":     "very-secure-password",
		"role":         domain.RoleRiskReviewer,
	}, login.Token)
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d", response.StatusCode)
	}
	body := readResponse(t, response)
	errorBody := body["error"].(map[string]any)
	if errorBody["code"] != "forbidden" {
		t.Fatalf("body = %+v", body)
	}
}

func TestUnknownJSONFieldAndOversizedBodyAreRejected(t *testing.T) {
	f := newHTTPFixture(t)
	login := f.request(t, http.MethodPost, "/api/v1/auth/login", map[string]any{"email": "ops@example.test", "password": "very-secure-password"}, "")
	body := readResponse(t, login)
	token := body["token"].(string)
	unknown := f.request(t, http.MethodPost, "/api/v1/workspaces", map[string]any{"code": "X", "unexpected": true}, token)
	if unknown.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown field status = %d", unknown.StatusCode)
	}
	_ = readResponse(t, unknown)
	large := strings.Repeat("x", 2<<20)
	req, err := http.NewRequest(http.MethodPost, f.server.URL+"/api/v1/workspaces", strings.NewReader(`{"code":"`+large+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("large body status = %d", response.StatusCode)
	}
	_ = readResponse(t, response)
}

func TestRequestIDIsPropagated(t *testing.T) {
	f := newHTTPFixture(t)
	req, err := http.NewRequest(http.MethodGet, f.server.URL+"/healthz", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Request-ID", "req-fixed")
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.Header.Get("X-Request-ID") != "req-fixed" {
		t.Fatalf("request id = %q", response.Header.Get("X-Request-ID"))
	}
}
