package modeltest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Response captures the raw outcome of a non-streaming HTTP call.
type Response struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

// Text returns the body as a UTF-8 string.
func (r *Response) Text() string {
	if r == nil {
		return ""
	}
	return string(r.Body)
}

// JSON unmarshals the body into a fresh generic structure.
func (r *Response) JSON() (interface{}, error) {
	if r == nil || len(r.Body) == 0 {
		return nil, fmt.Errorf("empty response body")
	}
	var out interface{}
	if err := json.Unmarshal(r.Body, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Client is a thin HTTP wrapper around the running CLIProxyAPI server. It
// always talks to the local process and never routes through an outbound
// proxy (tests must not go through Clash/http_proxy/etc.).
type Client struct {
	BaseURL    string
	APIKey     string
	UserAgent  string
	HTTP       *http.Client
	MaxTimeout time.Duration
}

// NewClient returns a Client configured for the given base URL + API key.
func NewClient(baseURL, apiKey string) *Client {
	transport := &http.Transport{
		Proxy:                 nil, // never route through the OS/HTTP proxy
		DisableCompression:    false,
		ResponseHeaderTimeout: 120 * time.Second,
	}
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		APIKey:     apiKey,
		UserAgent:  "cliproxy-modeltest/0.1",
		HTTP:       &http.Client{Transport: transport, Timeout: 0},
		MaxTimeout: 120 * time.Second,
	}
}

// SetTimeout applies a per-request timeout (0 disables the timeout).
func (c *Client) SetTimeout(d time.Duration) {
	c.MaxTimeout = d
}

func (c *Client) fullURL(path string) (string, error) {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path, nil
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	base := c.BaseURL
	if base == "" {
		return "", fmt.Errorf("base url is empty")
	}
	u, err := url.Parse(base + path)
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

func (c *Client) applyAuth(h http.Header, geminiKeyHeader bool) {
	if c.APIKey == "" {
		return
	}
	h.Set("Authorization", "Bearer "+c.APIKey)
	h.Set("x-api-key", c.APIKey)
	if geminiKeyHeader {
		h.Set("x-goog-api-key", c.APIKey)
	}
}

// Get issues a GET request.
func (c *Client) Get(ctx context.Context, path string, extra http.Header, geminiKeyHeader bool) (*Response, error) {
	return c.do(ctx, http.MethodGet, path, nil, extra, geminiKeyHeader, false)
}

// PostJSON issues a POST with a JSON-encoded body.
func (c *Client) PostJSON(ctx context.Context, path string, body interface{}, extra http.Header, geminiKeyHeader bool) (*Response, error) {
	return c.do(ctx, http.MethodPost, path, body, extra, geminiKeyHeader, false)
}

// StreamPostJSON issues a POST asking for text/event-stream and iterates
// SSE events into `onEvent`. The call returns when the stream ends or an
// error is encountered. A non-2xx response returns an error with the body.
func (c *Client) StreamPostJSON(ctx context.Context, path string, body interface{}, extra http.Header, geminiKeyHeader bool, onEvent func(SSEEvent)) error {
	u, err := c.fullURL(path)
	if err != nil {
		return err
	}
	buf, err := encodeJSON(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("User-Agent", c.UserAgent)
	c.applyAuth(req.Header, geminiKeyHeader)
	for k, vs := range extra {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("stream POST %s: status %d: %s", u, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	parser := NewSSEParser()
	buf2 := make([]byte, 4096)
	for {
		n, rerr := resp.Body.Read(buf2)
		if n > 0 {
			for _, ev := range parser.Feed(string(buf2[:n])) {
				if onEvent != nil {
					onEvent(ev)
				}
			}
		}
		if rerr == io.EOF {
			for _, ev := range parser.Close() {
				if onEvent != nil {
					onEvent(ev)
				}
			}
			return nil
		}
		if rerr != nil {
			return rerr
		}
	}
}

func (c *Client) do(ctx context.Context, method, path string, body interface{}, extra http.Header, geminiKeyHeader, expectStream bool) (*Response, error) {
	u, err := c.fullURL(path)
	if err != nil {
		return nil, err
	}
	var reader io.Reader
	if body != nil {
		buf, err := encodeJSON(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(buf)
	}

	callCtx := ctx
	if c.MaxTimeout > 0 {
		var cancel context.CancelFunc
		callCtx, cancel = context.WithTimeout(ctx, c.MaxTimeout)
		defer cancel()
	}
	req, err := http.NewRequestWithContext(callCtx, method, u, reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if expectStream {
		req.Header.Set("Accept", "text/event-stream")
	} else {
		req.Header.Set("Accept", "application/json")
	}
	req.Header.Set("User-Agent", c.UserAgent)
	c.applyAuth(req.Header, geminiKeyHeader)
	for k, vs := range extra {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	return &Response{
		StatusCode: resp.StatusCode,
		Header:     resp.Header.Clone(),
		Body:       bodyBytes,
	}, nil
}

func encodeJSON(v interface{}) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	out := buf.Bytes()
	// json.Encoder appends a trailing \n; trim it for stable comparisons.
	if len(out) > 0 && out[len(out)-1] == '\n' {
		out = out[:len(out)-1]
	}
	return out, nil
}
