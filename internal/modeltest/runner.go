package modeltest

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Settings are the resolved inputs needed to run one or more suites.
type Settings struct {
	BaseURL string
	APIKey  string

	OpenAIModel    string
	ResponsesModel string
	ClaudeModel    string
	GeminiModel    string
	ImagesModel    string

	Timeout time.Duration

	// Suites is the ordered list of suite names to execute. An empty slice
	// means "run every registered suite in SuiteOrder()".
	Suites []string
}

// Runner orchestrates suite execution against a local server.
type Runner struct {
	Settings Settings
	Client   *Client
}

// NewRunner builds a Runner with a fresh client.
func NewRunner(s Settings) *Runner {
	client := NewClient(s.BaseURL, s.APIKey)
	if s.Timeout > 0 {
		client.SetTimeout(s.Timeout)
	}
	applyModelDefaults(&s)
	return &Runner{Settings: s, Client: client}
}

// Context builds the SuiteContext for a single suite run.
func (r *Runner) Context() *SuiteContext {
	return &SuiteContext{
		Client:         r.Client,
		OpenAIModel:    r.Settings.OpenAIModel,
		ResponsesModel: r.Settings.ResponsesModel,
		ClaudeModel:    r.Settings.ClaudeModel,
		GeminiModel:    r.Settings.GeminiModel,
		ImagesModel:    r.Settings.ImagesModel,
	}
}

// Run executes the selected suites in order and returns the aggregated results.
func (r *Runner) Run(ctx context.Context) ([]CheckResult, error) {
	names, err := r.selected()
	if err != nil {
		return nil, err
	}
	sc := r.Context()
	results := make([]CheckResult, 0, len(names))
	for _, name := range names {
		suite, ok := Registry[name]
		if !ok {
			results = append(results, Fail(name, 0, fmt.Sprintf("unknown suite %q", name), ""))
			continue
		}
		part := safeInvoke(ctx, suite, sc)
		results = append(results, part...)
	}
	return results, nil
}

// HasFailures reports whether any CheckResult is in FAIL state.
func HasFailures(results []CheckResult) bool {
	for _, r := range results {
		if r.Status == StatusFail {
			return true
		}
	}
	return false
}

func (r *Runner) selected() ([]string, error) {
	if len(r.Settings.Suites) == 0 {
		return SuiteNames(), nil
	}
	var unknown []string
	out := make([]string, 0, len(r.Settings.Suites))
	for _, raw := range r.Settings.Suites {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		if _, ok := Registry[name]; !ok {
			unknown = append(unknown, name)
			continue
		}
		out = append(out, name)
	}
	if len(unknown) > 0 {
		return nil, fmt.Errorf("unknown suite(s): %s", strings.Join(unknown, ", "))
	}
	return out, nil
}

func safeInvoke(ctx context.Context, suite Suite, sc *SuiteContext) (results []CheckResult) {
	defer func() {
		if r := recover(); r != nil {
			results = []CheckResult{Fail(
				suite.Name+": "+suite.Description,
				0,
				fmt.Sprintf("panic: %v", r),
				"",
			)}
		}
	}()
	return suite.Run(ctx, sc)
}

func applyModelDefaults(s *Settings) {
	if strings.TrimSpace(s.OpenAIModel) == "" {
		s.OpenAIModel = "gpt-5.4-mini"
	}
	if strings.TrimSpace(s.ResponsesModel) == "" {
		s.ResponsesModel = s.OpenAIModel
	}
	if strings.TrimSpace(s.ClaudeModel) == "" {
		s.ClaudeModel = "claude-sonnet-4-5-20250929"
	}
	if strings.TrimSpace(s.GeminiModel) == "" {
		s.GeminiModel = "gemini-2.5-pro"
	}
	if strings.TrimSpace(s.ImagesModel) == "" {
		s.ImagesModel = "gpt-image-2"
	}
}
