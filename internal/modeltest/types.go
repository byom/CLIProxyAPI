// Package modeltest exposes an in-process harness for exercising the
// CLIProxyAPI HTTP surfaces against currently logged-in providers. It ports
// the Python api_smoke suites into Go so they can be driven from the embedded
// management UI and from `go test`.
package modeltest

import (
	"context"
	"time"
)

// Status represents the outcome of a single check.
type Status string

const (
	StatusPass Status = "PASS"
	StatusFail Status = "FAIL"
	StatusSkip Status = "SKIP"
)

// CheckResult captures the outcome of a single end-to-end assertion.
type CheckResult struct {
	Name      string                 `json:"name"`
	Status    Status                 `json:"status"`
	ElapsedMs float64                `json:"elapsed_ms"`
	Detail    string                 `json:"detail,omitempty"`
	Error     string                 `json:"error,omitempty"`
	Extra     map[string]interface{} `json:"extra,omitempty"`
	Request   *Trace                 `json:"request,omitempty"`
	Response  *Trace                 `json:"response,omitempty"`
}

// Trace captures a request or response for after-the-fact inspection.
type Trace struct {
	Method  string            `json:"method,omitempty"`
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Status  int               `json:"status,omitempty"`
	// Kind hints at how the UI should render Body: "json", "sse", "text".
	Kind string `json:"kind,omitempty"`
	// Body carries the serialised payload as text. For SSE it is the
	// newline-joined stream; for JSON it is the marshalled payload.
	Body string `json:"body,omitempty"`
}

// WithRequest attaches a request trace and returns the updated result.
func (r CheckResult) WithRequest(t *Trace) CheckResult {
	r.Request = t
	return r
}

// WithResponse attaches a response trace and returns the updated result.
func (r CheckResult) WithResponse(t *Trace) CheckResult {
	r.Response = t
	return r
}

// Suite is the unit of work a caller picks by name. Each suite may produce
// one or more CheckResults.
type Suite struct {
	Name        string
	Description string
	Run         func(ctx context.Context, sc *SuiteContext) []CheckResult
}

// SuiteContext carries the resolved inputs a suite needs: the HTTP client
// pointed at the local server and the model names picked for each protocol.
type SuiteContext struct {
	Client *Client

	OpenAIModel    string
	ResponsesModel string
	ClaudeModel    string
	GeminiModel    string
	ImagesModel    string
}

// NewCheckResult is a small helper that timestamps the result.
func NewCheckResult(name string, status Status, elapsed time.Duration) CheckResult {
	return CheckResult{
		Name:      name,
		Status:    status,
		ElapsedMs: float64(elapsed) / float64(time.Millisecond),
	}
}

// Pass returns a passing CheckResult with an optional detail.
func Pass(name string, elapsed time.Duration, detail string) CheckResult {
	r := NewCheckResult(name, StatusPass, elapsed)
	r.Detail = detail
	return r
}

// Fail returns a failing CheckResult with the given error text and optional
// detail for additional context.
func Fail(name string, elapsed time.Duration, errText string, detail string) CheckResult {
	r := NewCheckResult(name, StatusFail, elapsed)
	r.Error = errText
	r.Detail = detail
	return r
}

// Skip returns a skipped CheckResult with a human-readable reason.
func Skip(name string, reason string) CheckResult {
	r := NewCheckResult(name, StatusSkip, 0)
	r.Detail = reason
	return r
}

// Stopwatch is a tiny helper to measure elapsed time for a block.
type Stopwatch struct {
	start time.Time
}

// StartStopwatch records the start time.
func StartStopwatch() *Stopwatch {
	return &Stopwatch{start: time.Now()}
}

// Elapsed returns the duration since the stopwatch started.
func (s *Stopwatch) Elapsed() time.Duration {
	if s == nil {
		return 0
	}
	return time.Since(s.start)
}
