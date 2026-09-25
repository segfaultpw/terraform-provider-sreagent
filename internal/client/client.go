// Package client talks to the SRE Agent configuration API (/api/v1/config).
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// DefaultBaseURL is the hosted platform.
const DefaultBaseURL = "https://sreagent.app"

const maxRetryAfter = 60 * time.Second

// Config configures a Client.
type Config struct {
	BaseURL      string
	APIKey       string
	Organization string
	UserAgent    string
	MaxRetries   int
	HTTPClient   *http.Client
	Sleep        func(context.Context, time.Duration) error
}

// Client is safe for concurrent use.
type Client struct {
	base       *url.URL
	apiKey     string
	org        string
	userAgent  string
	maxRetries int
	http       *http.Client
	sleep      func(context.Context, time.Duration) error
}

// Organization is the tenant every answer names.
type Organization struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// Request is one call. Path is relative to /api/v1/config, segments already path-escaped.
type Request struct {
	Method  string
	Path    string
	Query   url.Values
	Body    map[string]any
	IfMatch string
	// Secrets are values that must never appear in a returned error.
	Secrets []string
}

// Response is a 2xx answer with its data unwrapped.
type Response struct {
	Status       int
	ETag         string
	Organization Organization
	Data         json.RawMessage
	Truncated    bool
}

// APIError is a non-2xx answer.
type APIError struct {
	Status   int
	Code     string
	Message  string
	Existing map[string]any
	Current  map[string]any
	// Retried: an earlier attempt of this call ended in a gateway or
	// transport error, so it may have been applied before this answer.
	Retried bool
}

func (e *APIError) Error() string {
	return fmt.Sprintf("the SRE Agent API answered %d %s: %s", e.Status, e.Code, e.Message)
}

func statusIs(err error, status int) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.Status == status
}

// IsNotFound reports a 404.
func IsNotFound(err error) bool { return statusIs(err, http.StatusNotFound) }

// IsConflict reports a 409.
func IsConflict(err error) bool { return statusIs(err, http.StatusConflict) }

// IsPreconditionFailed reports a 412.
func IsPreconditionFailed(err error) bool { return statusIs(err, http.StatusPreconditionFailed) }

// IsRetried reports an error answered to a retry of a call whose earlier
// attempt may have been applied.
func IsRetried(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.Retried
}

// IsReadOnlyKey reports the facade refusing a write from an api:config_read key.
func IsReadOnlyKey(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.Status == http.StatusForbidden && apiErr.Code == "read_only_key"
}

// New validates the configuration before any request is sent.
func New(cfg Config) (*Client, error) {
	raw := cfg.BaseURL
	if raw == "" {
		raw = DefaultBaseURL
	}
	base, err := url.Parse(strings.TrimRight(raw, "/"))
	if err != nil || base.Host == "" {
		return nil, fmt.Errorf("base_url %q is not an absolute URL", raw)
	}
	if base.Scheme != "https" && (base.Scheme != "http" || !IsLoopback(base.Hostname())) {
		return nil, fmt.Errorf("base_url must use https; plain http is accepted only for localhost, got %q", raw)
	}
	if !strings.HasPrefix(cfg.APIKey, "sre_ak_") {
		return nil, errors.New("api_key must be an sre_ak_ API key holding the api:admin scope; the configuration API refuses personal tokens")
	}
	if cfg.MaxRetries < 0 || cfg.MaxRetries > 10 {
		return nil, errors.New("max_retries must be between 0 and 10")
	}
	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{
			Timeout: 60 * time.Second,
			// The API never redirects. Following one could replay the key and a
			// secret-bearing body to another scheme or host, or turn a POST into
			// a GET, so a redirect is answered as the error it is.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}
	}
	sleep := cfg.Sleep
	if sleep == nil {
		sleep = sleepContext
	}
	return &Client{base: base, apiKey: cfg.APIKey, org: cfg.Organization, userAgent: cfg.UserAgent, maxRetries: cfg.MaxRetries, http: hc, sleep: sleep}, nil
}

// IsLoopback reports a host that never leaves this machine.
func IsLoopback(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func sleepContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Do sends one request, retrying only where a retry cannot duplicate a write.
func (c *Client) Do(ctx context.Context, r Request) (*Response, error) {
	var payload []byte
	if r.Body != nil {
		b, err := json.Marshal(r.Body)
		if err != nil {
			return nil, fmt.Errorf("encoding the request body: %w", err)
		}
		payload = b
	}
	target := c.base.JoinPath("api", "v1", "config", r.Path)
	if len(r.Query) > 0 {
		target.RawQuery = r.Query.Encode()
	}
	secrets := append([]string{c.apiKey}, r.Secrets...)
	// uncertain: an earlier attempt failed in a way that says nothing about
	// whether it was applied. A 429 is not one: the rate limiter answers
	// before anything runs.
	uncertain := false

	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, r.Method, target.String(), bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", c.userAgent)
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if r.IfMatch != "" {
			req.Header.Set("If-Match", r.IfMatch)
		}

		resp, err := c.http.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if r.Method != http.MethodPost && attempt < c.maxRetries {
				uncertain = true
				if serr := c.sleep(ctx, backoff(attempt)); serr != nil {
					return nil, serr
				}
				continue
			}
			return nil, errors.New(redact(fmt.Sprintf("%s %s: %v", r.Method, r.Path, err), secrets))
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("reading the answer to %s %s: %w", r.Method, r.Path, readErr)
		}
		tflog.Debug(ctx, "sreagent API call", map[string]any{"method": r.Method, "path": r.Path, "status": resp.StatusCode, "attempt": attempt})

		if wait, retry := c.retryWait(r.Method, resp, attempt); retry {
			if resp.StatusCode != http.StatusTooManyRequests {
				uncertain = true
			}
			if serr := c.sleep(ctx, wait); serr != nil {
				return nil, serr
			}
			continue
		}
		out, err := c.decode(r, resp, body, secrets)
		var apiErr *APIError
		if errors.As(err, &apiErr) {
			apiErr.Retried = uncertain
		}
		return out, err
	}
}

func (c *Client) retryWait(method string, resp *http.Response, attempt int) (time.Duration, bool) {
	if attempt >= c.maxRetries {
		return 0, false
	}
	switch resp.StatusCode {
	case http.StatusTooManyRequests:
		// The rate limiter answers before the controller runs, so nothing was written.
		if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && s >= 0 {
			return min(time.Duration(s)*time.Second, maxRetryAfter), true
		}
		return backoff(attempt), true
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return backoff(attempt), method != http.MethodPost
	}
	return 0, false
}

func backoff(attempt int) time.Duration {
	base := min(time.Second<<attempt, 8*time.Second)
	// Jitter only spreads retries apart; it protects nothing, so a weak source is fine.
	return base + time.Duration(rand.Int64N(int64(250*time.Millisecond))) //nolint:gosec
}

type envelope struct {
	Organization *Organization   `json:"organization"`
	Data         json.RawMessage `json:"data"`
	Truncated    bool            `json:"truncated"`
	Error        string          `json:"error"`
	Message      string          `json:"message"`
	Existing     map[string]any  `json:"existing"`
}

func (c *Client) decode(r Request, resp *http.Response, body []byte, secrets []string) (*Response, error) {
	var env envelope
	if len(body) > 0 {
		if err := json.Unmarshal(body, &env); err != nil {
			return nil, &APIError{Status: resp.StatusCode, Code: "invalid_response", Message: fmt.Sprintf("%s %s answered a body that is not JSON", r.Method, r.Path)}
		}
	}
	if env.Organization != nil && c.org != "" && env.Organization.Slug != c.org {
		return nil, fmt.Errorf("the API key belongs to organization %q but the provider is pinned to %q; nothing was recorded in state", env.Organization.Slug, c.org)
	}
	success := resp.StatusCode >= 200 && resp.StatusCode < 300
	// Every success names its organization; one that does not cannot prove
	// the pin, so it is refused rather than trusted.
	if success && c.org != "" && env.Organization == nil {
		return nil, fmt.Errorf("%s %s answered without naming its organization, so the pin to %q cannot be checked; nothing was recorded in state", r.Method, r.Path, c.org)
	}
	if success {
		out := &Response{Status: resp.StatusCode, ETag: resp.Header.Get("ETag"), Data: env.Data, Truncated: env.Truncated}
		if env.Organization != nil {
			out.Organization = *env.Organization
		}
		return out, nil
	}
	apiErr := &APIError{Status: resp.StatusCode, Code: redact(env.Error, secrets), Message: redact(env.Message, secrets), Existing: env.Existing}
	if apiErr.Message == "" {
		apiErr.Message = http.StatusText(resp.StatusCode)
	}
	if resp.StatusCode == http.StatusPreconditionFailed && len(env.Data) > 0 {
		var current map[string]any
		if json.Unmarshal(env.Data, &current) == nil {
			apiErr.Current = current
		}
	}
	return nil, apiErr
}

func redact(s string, secrets []string) string {
	for _, secret := range secrets {
		if secret != "" {
			s = strings.ReplaceAll(s, secret, "[redacted]")
		}
	}
	return s
}
