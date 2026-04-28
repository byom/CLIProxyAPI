package management

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/modeltest"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/registry"
)

// modelTestSuiteInfo describes a named suite for the UI/client.
type modelTestSuiteInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// modelTestTarget describes a single auth-file entry enriched with its
// currently-served models. The UI uses this payload to render the "Test"
// buttons grouped per auth file.
type modelTestTarget struct {
	Name          string   `json:"name"`
	AuthID        string   `json:"auth_id,omitempty"`
	Provider      string   `json:"provider,omitempty"`
	Type          string   `json:"type,omitempty"`
	Label         string   `json:"label,omitempty"`
	Disabled      bool     `json:"disabled"`
	Unavailable   bool     `json:"unavailable"`
	Status        string   `json:"status,omitempty"`
	StatusMessage string   `json:"status_message,omitempty"`
	Models        []string `json:"models"`
}

type modelTestRunRequest struct {
	Suites         []string `json:"suites,omitempty"`
	OpenAIModel    string   `json:"openai_model,omitempty"`
	ResponsesModel string   `json:"responses_model,omitempty"`
	ClaudeModel    string   `json:"claude_model,omitempty"`
	GeminiModel    string   `json:"gemini_model,omitempty"`
	ImagesModel    string   `json:"images_model,omitempty"`
	APIKey         string   `json:"api_key,omitempty"`
	BaseURL        string   `json:"base_url,omitempty"`
	TimeoutSeconds int      `json:"timeout_seconds,omitempty"`
}

type modelTestRunResponse struct {
	Results []modeltest.CheckResult `json:"results"`
	Summary modelTestSummary        `json:"summary"`
	Suites  []string                `json:"suites"`
}

type modelTestSummary struct {
	Total int `json:"total"`
	Pass  int `json:"pass"`
	Fail  int `json:"fail"`
	Skip  int `json:"skip"`
}

// GetModelTestSuites returns the list of suites the runner can execute.
func (h *Handler) GetModelTestSuites(c *gin.Context) {
	suites := make([]modelTestSuiteInfo, 0, len(modeltest.Registry))
	for _, name := range modeltest.SuiteNames() {
		entry := modeltest.Registry[name]
		suites = append(suites, modelTestSuiteInfo{
			Name:        entry.Name,
			Description: entry.Description,
		})
	}
	c.JSON(http.StatusOK, gin.H{"suites": suites})
}

// GetModelTestTargets returns auth files joined with their served models so
// the UI can render a flat list in one request.
func (h *Handler) GetModelTestTargets(c *gin.Context) {
	if h == nil || h.authManager == nil {
		c.JSON(http.StatusOK, gin.H{"targets": []modelTestTarget{}})
		return
	}
	reg := registry.GetGlobalRegistry()
	auths := h.authManager.List()
	targets := make([]modelTestTarget, 0, len(auths))
	for _, a := range auths {
		if a == nil {
			continue
		}
		name := strings.TrimSpace(a.FileName)
		if name == "" {
			name = strings.TrimSpace(a.ID)
		}
		models := reg.GetModelsForClient(a.ID)
		ids := make([]string, 0, len(models))
		for _, m := range models {
			if strings.TrimSpace(m.ID) != "" {
				ids = append(ids, m.ID)
			}
		}
		sort.Strings(ids)
		providerType := ""
		if a.Attributes != nil {
			providerType = strings.TrimSpace(a.Attributes["type"])
		}
		targets = append(targets, modelTestTarget{
			Name:          name,
			AuthID:        a.ID,
			Provider:      a.Provider,
			Type:          providerType,
			Label:         a.Label,
			Disabled:      a.Disabled,
			Unavailable:   a.Unavailable,
			Status:        string(a.Status),
			StatusMessage: a.StatusMessage,
			Models:        ids,
		})
	}
	sort.Slice(targets, func(i, j int) bool {
		return strings.ToLower(targets[i].Name) < strings.ToLower(targets[j].Name)
	})
	c.JSON(http.StatusOK, gin.H{"targets": targets})
}

// RunModelTests executes one or more suites against the local API surface
// and returns a structured result envelope.
func (h *Handler) RunModelTests(c *gin.Context) {
	if h == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "handler not initialized"})
		return
	}

	req := modelTestRunRequest{}
	if err := c.ShouldBindJSON(&req); err != nil {
		if err != nil && !strings.Contains(err.Error(), "EOF") {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid request body: %v", err)})
			return
		}
	}

	cfg := h.cfg
	if cfg == nil {
		c.JSON(http.StatusPreconditionFailed, gin.H{"error": "server configuration not loaded"})
		return
	}

	apiKey := strings.TrimSpace(req.APIKey)
	if apiKey == "" {
		for _, k := range cfg.APIKeys {
			if trimmed := strings.TrimSpace(k); trimmed != "" {
				apiKey = trimmed
				break
			}
		}
	}
	if apiKey == "" {
		c.JSON(http.StatusPreconditionFailed, gin.H{"error": "no api-key available; configure at least one in config.yaml api-keys or pass api_key in the request"})
		return
	}

	baseURL := strings.TrimSpace(req.BaseURL)
	if baseURL == "" {
		host := strings.TrimSpace(cfg.Host)
		if host == "" || host == "0.0.0.0" || host == "::" {
			host = "127.0.0.1"
		}
		scheme := "http"
		if cfg.TLS.Enable {
			scheme = "https"
		}
		baseURL = fmt.Sprintf("%s://%s:%d", scheme, host, cfg.Port)
	}

	timeout := 120 * time.Second
	if req.TimeoutSeconds > 0 {
		timeout = time.Duration(req.TimeoutSeconds) * time.Second
	}

	settings := modeltest.Settings{
		BaseURL:        baseURL,
		APIKey:         apiKey,
		OpenAIModel:    strings.TrimSpace(req.OpenAIModel),
		ResponsesModel: strings.TrimSpace(req.ResponsesModel),
		ClaudeModel:    strings.TrimSpace(req.ClaudeModel),
		GeminiModel:    strings.TrimSpace(req.GeminiModel),
		ImagesModel:    strings.TrimSpace(req.ImagesModel),
		Timeout:        timeout,
		Suites:         dedupeNonEmpty(req.Suites),
	}

	runner := modeltest.NewRunner(settings)
	overallTimeout := timeout + 30*time.Second
	// The images suite alone can legitimately take several minutes. When the
	// caller includes it, grant the whole request at least 10 minutes.
	if containsSuite(settings.Suites, "images") && overallTimeout < 10*time.Minute {
		overallTimeout = 10 * time.Minute
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), overallTimeout)
	defer cancel()
	results, err := runner.Run(ctx)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	suitesUsed := settings.Suites
	if len(suitesUsed) == 0 {
		suitesUsed = modeltest.SuiteNames()
	}
	summary := summarize(results)
	c.JSON(http.StatusOK, modelTestRunResponse{Results: results, Summary: summary, Suites: suitesUsed})
}

func summarize(results []modeltest.CheckResult) modelTestSummary {
	out := modelTestSummary{Total: len(results)}
	for _, r := range results {
		switch r.Status {
		case modeltest.StatusPass:
			out.Pass++
		case modeltest.StatusFail:
			out.Fail++
		default:
			out.Skip++
		}
	}
	return out
}

func containsSuite(suites []string, target string) bool {
	if len(suites) == 0 {
		// Empty slice means "run every registered suite".
		return target != ""
	}
	for _, s := range suites {
		if strings.EqualFold(strings.TrimSpace(s), target) {
			return true
		}
	}
	return false
}

func dedupeNonEmpty(in []string) []string {
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// marshalIndent is kept available for potential debug logging usage.
var _ = json.MarshalIndent
