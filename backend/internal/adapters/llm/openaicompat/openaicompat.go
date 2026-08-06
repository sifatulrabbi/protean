// Package openaicompat implements ports.LLMProvider against any service that
// speaks the OpenAI /chat/completions API. One client serves both providers
// Protean uses (D4): OpenRouter is the default and OpenAI is the secondary,
// and they differ only by base URL, model, and two optional attribution
// headers.
//
// The transport is net/http and a hand-rolled SSE reader — no SDK — because
// the surface Protean needs is one endpoint and the streaming format is six
// lines of parsing. That keeps the dependency list short and the failure modes
// visible.
//
// The API key lives on the host and is never logged, never returned in an
// error, and never crosses into a sandbox (D11).
package openaicompat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/sifatulrabbi/protean/backend/internal/ports"
)

// completionsPath is appended to the base URL.
const completionsPath = "/chat/completions"

// errorBodyLimit caps how much of a failed response is read into the error.
// Enough to carry a provider's message, small enough not to paste a whole
// HTML error page into a log line.
const errorBodyLimit = 4 << 10

// responseHeaderTimeout bounds the wait for the provider to start answering.
// It is deliberately not an http.Client.Timeout: that one bounds the whole
// request including the streamed body, so it would kill a long but healthy
// agent turn. Once the stream is flowing, the caller's context is what ends
// it.
const responseHeaderTimeout = 2 * time.Minute

// Options configures a Client.
type Options struct {
	// BaseURL is the API root, without a trailing slash and without the
	// /chat/completions suffix. Required.
	BaseURL string

	// APIKey authenticates as a Bearer token. Required.
	APIKey string

	// Model is used when a ChatRequest does not name one. Required.
	Model string

	// SiteURL and SiteName are OpenRouter's optional attribution headers
	// (HTTP-Referer and X-Title). They are only sent when set.
	SiteURL  string
	SiteName string

	// HTTPClient overrides the default client, which exists so tests can point
	// at an httptest server and so an operator could inject a proxy.
	HTTPClient *http.Client

	Logger *slog.Logger
}

// Client is the OpenAI-compatible provider adapter.
type Client struct {
	baseURL  string
	apiKey   string
	model    string
	siteURL  string
	siteName string
	http     *http.Client
	log      *slog.Logger
}

var _ ports.LLMProvider = (*Client)(nil)

// Configuration rejections. Match with errors.Is.
var (
	ErrNoAPIKey  = errors.New("openaicompat: api key is empty")
	ErrNoBaseURL = errors.New("openaicompat: base url is empty")
	ErrNoModel   = errors.New("openaicompat: model is empty")
)

// New builds a Client. A missing key, base URL, or model is a constructor
// error rather than a boot failure: the process runs fine without a provider,
// it just cannot invoke the agent.
func New(opts Options) (*Client, error) {
	if opts.APIKey == "" {
		return nil, ErrNoAPIKey
	}
	if opts.BaseURL == "" {
		return nil, ErrNoBaseURL
	}
	if opts.Model == "" {
		return nil, ErrNoModel
	}
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Transport: streamingTransport()}
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	baseURL := strings.TrimRight(opts.BaseURL, "/")
	return &Client{
		baseURL:  baseURL,
		apiKey:   opts.APIKey,
		model:    opts.Model,
		siteURL:  opts.SiteURL,
		siteName: opts.SiteName,
		http:     httpClient,
		log:      logger.With("component", "llm", "base_url", baseURL),
	}, nil
}

// streamingTransport is the process default transport with a bound on how long
// the provider may take to start answering.
func streamingTransport() *http.Transport {
	tr, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return &http.Transport{ResponseHeaderTimeout: responseHeaderTimeout}
	}
	clone := tr.Clone()
	clone.ResponseHeaderTimeout = responseHeaderTimeout
	return clone
}

// Model is the model the client falls back to.
func (c *Client) Model() string { return c.model }

// StreamChat starts one streaming completion.
func (c *Client) StreamChat(ctx context.Context, req ports.ChatRequest) (ports.ChatStream, error) {
	if len(req.Messages) == 0 {
		return nil, errors.New("openaicompat: request has no messages")
	}
	model := req.Model
	if model == "" {
		model = c.model
	}

	body, err := json.Marshal(chatRequest{
		Model:         model,
		Messages:      encodeMessages(req.Messages),
		Tools:         encodeTools(req.Tools),
		Stream:        true,
		StreamOptions: &streamOptions{IncludeUsage: true},
	})
	if err != nil {
		return nil, fmt.Errorf("openaicompat: encode request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+completionsPath, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("openaicompat: build request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	// OpenRouter's attribution headers. They are optional and identify the
	// caller in OpenRouter's dashboards; other providers ignore them.
	if c.siteURL != "" {
		httpReq.Header.Set("HTTP-Referer", c.siteURL)
	}
	if c.siteName != "" {
		httpReq.Header.Set("X-Title", c.siteName)
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openaicompat: %s: %w", model, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		defer resp.Body.Close()
		return nil, newAPIError(resp, model)
	}

	c.log.Debug("llm stream started", "model", model, "messages", len(req.Messages), "tools", len(req.Tools))
	return newStream(resp.Body), nil
}

// APIError is a non-2xx response from the provider. It carries a truncated
// body because that is where providers put the reason, and never carries a
// header, so the key cannot leak into a log.
type APIError struct {
	StatusCode int
	Status     string
	Model      string
	Body       string
}

func (e *APIError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("openaicompat: %s (model %s)", e.Status, e.Model)
	}
	return fmt.Sprintf("openaicompat: %s (model %s): %s", e.Status, e.Model, e.Body)
}

func newAPIError(resp *http.Response, model string) *APIError {
	// One byte past the limit is read so a body sitting exactly on it can be
	// told apart from one that was cut.
	body, _ := io.ReadAll(io.LimitReader(resp.Body, errorBodyLimit+1))
	truncated := len(body) > errorBodyLimit
	if truncated {
		body = body[:errorBodyLimit]
		for len(body) > 0 && !utf8.Valid(body) {
			body = body[:len(body)-1]
		}
	}
	text := strings.TrimSpace(string(body))
	if truncated {
		text += "... (truncated)"
	}
	return &APIError{
		StatusCode: resp.StatusCode,
		Status:     resp.Status,
		Model:      model,
		Body:       text,
	}
}
