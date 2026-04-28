package modeltest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSettingsDefaultSelectsAllSuites(t *testing.T) {
	r := NewRunner(Settings{BaseURL: "http://example.invalid", APIKey: "k"})
	names, err := r.selected()
	if err != nil {
		t.Fatalf("selected returned error: %v", err)
	}
	if len(names) != len(SuiteNames()) {
		t.Fatalf("expected %d suites, got %d", len(SuiteNames()), len(names))
	}
}

func TestSettingsRejectsUnknownSuite(t *testing.T) {
	r := NewRunner(Settings{
		BaseURL: "http://example.invalid", APIKey: "k",
		Suites: []string{"bogus"},
	})
	if _, err := r.selected(); err == nil {
		t.Fatal("expected unknown suite to fail validation")
	}
}

func TestRunnerCapturesUnhandledPanic(t *testing.T) {
	original := Registry["models"].Run
	defer func() {
		entry := Registry["models"]
		entry.Run = original
		Registry["models"] = entry
	}()
	entry := Registry["models"]
	entry.Run = func(ctx context.Context, sc *SuiteContext) []CheckResult {
		panic("boom")
	}
	Registry["models"] = entry

	r := NewRunner(Settings{
		BaseURL: "http://example.invalid", APIKey: "k",
		Suites: []string{"models"},
	})
	results, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("runner returned error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != StatusFail {
		t.Fatalf("expected FAIL, got %s", results[0].Status)
	}
	if !strings.Contains(results[0].Error, "panic: boom") {
		t.Fatalf("expected panic reason in error, got %q", results[0].Error)
	}
	if !HasFailures(results) {
		t.Fatal("HasFailures should report true when a suite failed")
	}
}

func TestApplyModelDefaultsFillsGaps(t *testing.T) {
	s := Settings{}
	applyModelDefaults(&s)
	if s.OpenAIModel == "" {
		t.Fatal("OpenAIModel default not applied")
	}
	if s.ResponsesModel != s.OpenAIModel {
		t.Fatalf("ResponsesModel should default to OpenAIModel, got %q / %q", s.ResponsesModel, s.OpenAIModel)
	}
	if s.ClaudeModel == "" || s.GeminiModel == "" {
		t.Fatal("claude/gemini defaults not applied")
	}
}

func TestSuiteModelsHappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			http.Error(w, "missing auth", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"m-1"},{"id":"m-2"}]}`))
	}))
	defer srv.Close()

	runner := NewRunner(Settings{
		BaseURL: srv.URL,
		APIKey:  "test-key",
		Suites:  []string{"models"},
		Timeout: 5 * time.Second,
	})
	results, err := runner.Run(context.Background())
	if err != nil {
		t.Fatalf("runner error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != StatusPass {
		t.Fatalf("expected PASS, got %s (error=%q)", results[0].Status, results[0].Error)
	}
	if !strings.Contains(results[0].Detail, "2 models") {
		t.Fatalf("unexpected detail: %q", results[0].Detail)
	}
}

func TestSuiteChatJSONValidatesPayloadEndToEnd(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		body, _ := readAllString(r.Body)
		// Ensure the schema we defined actually went out on the wire.
		if !strings.Contains(body, "travel_itinerary") {
			http.Error(w, "schema missing", http.StatusBadRequest)
			return
		}
		if !strings.Contains(body, "response_format") {
			http.Error(w, "response_format missing", http.StatusBadRequest)
			return
		}
		reply := map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]interface{}{"content": validPayload}},
			},
		}
		buf, _ := json.Marshal(reply)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(buf)
	}))
	defer srv.Close()

	runner := NewRunner(Settings{
		BaseURL: srv.URL,
		APIKey:  "k",
		Suites:  []string{"chat_json"},
		Timeout: 5 * time.Second,
	})
	results, err := runner.Run(context.Background())
	if err != nil {
		t.Fatalf("runner error: %v", err)
	}
	if results[0].Status != StatusPass {
		t.Fatalf("expected PASS, got %+v", results[0])
	}
	if !strings.Contains(results[0].Detail, "trip-001") {
		t.Fatalf("expected trip_id in detail, got %q", results[0].Detail)
	}
}

func TestSuiteChatStreamAccumulatesDeltas(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		for _, delta := range []string{"one", "two", "three"} {
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"" + delta + "\\n\"}}]}\n\n"))
			flusher.Flush()
		}
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		flusher.Flush()
	}))
	defer srv.Close()

	runner := NewRunner(Settings{
		BaseURL: srv.URL,
		APIKey:  "k",
		Suites:  []string{"chat_stream"},
		Timeout: 5 * time.Second,
	})
	results, err := runner.Run(context.Background())
	if err != nil {
		t.Fatalf("runner error: %v", err)
	}
	if results[0].Status != StatusPass {
		t.Fatalf("expected PASS, got %+v", results[0])
	}
}

func readAllString(body interface{ Read(p []byte) (int, error) }) (string, error) {
	var b strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := body.Read(buf)
		if n > 0 {
			b.Write(buf[:n])
		}
		if err != nil {
			if err.Error() == "EOF" {
				return b.String(), nil
			}
			return b.String(), err
		}
	}
}
