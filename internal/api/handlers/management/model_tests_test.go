package management

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/modeltest"
)

func newTestHandler(cfg *config.Config) *Handler {
	if cfg == nil {
		cfg = &config.Config{}
	}
	return &Handler{cfg: cfg, failedAttempts: make(map[string]*attemptInfo)}
}

func TestModelTestsSuitesEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := newTestHandler(nil)
	router := gin.New()
	router.GET("/v0/management/model-tests/suites", h.GetModelTestSuites)

	req := httptest.NewRequest(http.MethodGet, "/v0/management/model-tests/suites", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d (body=%s)", rr.Code, rr.Body.String())
	}
	var body struct {
		Suites []modelTestSuiteInfo `json:"suites"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Suites) != len(modeltest.SuiteNames()) {
		t.Fatalf("expected %d suites, got %d", len(modeltest.SuiteNames()), len(body.Suites))
	}
	for _, s := range body.Suites {
		if s.Name == "" || s.Description == "" {
			t.Fatalf("empty suite entry: %+v", s)
		}
	}
}

func TestModelTestsRunRejectsMissingKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := newTestHandler(&config.Config{SDKConfig: config.SDKConfig{APIKeys: []string{}}})
	router := gin.New()
	router.POST("/v0/management/model-tests/run", h.RunModelTests)

	reqBody := bytes.NewBufferString(`{"suites":["models"]}`)
	req := httptest.NewRequest(http.MethodPost, "/v0/management/model-tests/run", reqBody)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusPreconditionFailed {
		t.Fatalf("expected 412 when no api-keys configured, got %d (body=%s)", rr.Code, rr.Body.String())
	}
}

func TestModelTestsRunExecutesAgainstLocalStub(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// Start a stub that pretends to be the local /v1/models endpoint.
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer stub-key" {
			http.Error(w, "bad auth", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"alpha"},{"id":"beta"}]}`))
	}))
	defer stub.Close()

	h := newTestHandler(&config.Config{SDKConfig: config.SDKConfig{APIKeys: []string{"stub-key"}}})
	router := gin.New()
	router.POST("/v0/management/model-tests/run", h.RunModelTests)

	payload := map[string]interface{}{
		"suites":          []string{"models"},
		"base_url":        stub.URL,
		"timeout_seconds": 5,
	}
	buf, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/v0/management/model-tests/run", bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d (body=%s)", rr.Code, rr.Body.String())
	}
	var resp modelTestRunResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Summary.Pass != 1 || resp.Summary.Fail != 0 {
		t.Fatalf("unexpected summary: %+v", resp.Summary)
	}
	if len(resp.Results) != 1 || resp.Results[0].Status != modeltest.StatusPass {
		t.Fatalf("expected a single PASS result, got %+v", resp.Results)
	}
}
