package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const testKey = "sre_ak_test0123456789"

func noSleep(context.Context, time.Duration) error { return nil }

func newTest(t *testing.T, h http.HandlerFunc, org string) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c, err := New(Config{BaseURL: srv.URL, APIKey: testKey, Organization: org, UserAgent: "test", MaxRetries: 3, Sleep: noSleep})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func write(w http.ResponseWriter, status int, body any, headers map[string]string) {
	for k, v := range headers {
		w.Header().Set(k, v)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

var org = map[string]any{"id": "o1", "name": "Acme", "slug": "acme"}

func TestNewRefusesPlainHTTPExceptLoopback(t *testing.T) {
	if _, err := New(Config{BaseURL: "http://sreagent.app", APIKey: testKey}); err == nil {
		t.Fatal("plain http to a remote host must be refused")
	}
	if _, err := New(Config{BaseURL: "http://127.0.0.1:4000", APIKey: testKey}); err != nil {
		t.Fatalf("loopback http must be allowed: %v", err)
	}
}

func TestNewRefusesAPersonalToken(t *testing.T) {
	if _, err := New(Config{APIKey: "sre_pt_abc"}); err == nil { //nolint:gosec // a made-up token of the wrong kind
		t.Fatal("a non sre_ak_ credential must be refused before any request")
	}
}

func TestDoSendsAuthIfMatchAndUnwrapsData(t *testing.T) {
	c := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+testKey {
			t.Errorf("authorization header %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("If-Match") != `"c1-abc"` {
			t.Errorf("if-match %q", r.Header.Get("If-Match"))
		}
		if r.URL.Path != "/api/v1/config/deploy_policies/p1" {
			t.Errorf("path %q", r.URL.Path)
		}
		write(w, 200, map[string]any{"organization": org, "data": map[string]any{"id": "p1"}}, map[string]string{"ETag": `"c1-def"`})
	}, "")

	resp, err := c.Do(context.Background(), Request{Method: http.MethodPut, Path: "deploy_policies/p1", Body: map[string]any{"enabled": true}, IfMatch: `"c1-abc"`})
	if err != nil {
		t.Fatal(err)
	}
	if resp.ETag != `"c1-def"` || resp.Organization.Slug != "acme" || !strings.Contains(string(resp.Data), `"p1"`) {
		t.Fatalf("unexpected response %+v", resp)
	}
}

func TestPreconditionFailedCarriesCurrentRow(t *testing.T) {
	c := newTest(t, func(w http.ResponseWriter, _ *http.Request) {
		write(w, 412, map[string]any{"organization": org, "error": "precondition_failed", "message": "The row changed", "data": map[string]any{"id": "p1", "enabled": false}}, nil)
	}, "")

	_, err := c.Do(context.Background(), Request{Method: http.MethodPut, Path: "deploy_policies/p1", IfMatch: `"x"`})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || !IsPreconditionFailed(err) || apiErr.Current["enabled"] != false {
		t.Fatalf("want a 412 APIError with the current row, got %v", err)
	}
}

func TestRetries429OnPostHonoringRetryAfter(t *testing.T) {
	var calls atomic.Int32
	var slept []time.Duration
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			write(w, 429, map[string]any{"error": "rate_limited", "message": "slow down"}, map[string]string{"Retry-After": "7"})
			return
		}
		write(w, 201, map[string]any{"organization": org, "data": map[string]any{"id": "n"}}, nil)
	}))
	defer srv.Close()
	c, _ := New(Config{BaseURL: srv.URL, APIKey: testKey, MaxRetries: 3, Sleep: func(_ context.Context, d time.Duration) error { slept = append(slept, d); return nil }})

	if _, err := c.Do(context.Background(), Request{Method: http.MethodPost, Path: "teams", Body: map[string]any{"name": "x"}}); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 || len(slept) != 1 || slept[0] != 7*time.Second {
		t.Fatalf("calls=%d slept=%v", calls.Load(), slept)
	}
}

func TestNeverRetriesPostOn5xx(t *testing.T) {
	var calls atomic.Int32
	c := newTest(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		write(w, 503, map[string]any{"error": "unavailable", "message": "down"}, nil)
	}, "")
	if _, err := c.Do(context.Background(), Request{Method: http.MethodPost, Path: "teams"}); err == nil {
		t.Fatal("want an error")
	}
	if calls.Load() != 1 {
		t.Fatalf("POST retried %d times", calls.Load()-1)
	}
}

func TestRetriesGetOn5xxUpToTheLimit(t *testing.T) {
	var calls atomic.Int32
	c := newTest(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		write(w, 502, map[string]any{"error": "bad_gateway", "message": "x"}, nil)
	}, "")
	_, _ = c.Do(context.Background(), Request{Method: http.MethodGet, Path: "teams"})
	if calls.Load() != 4 {
		t.Fatalf("want 1 try + 3 retries, got %d calls", calls.Load())
	}
}

func TestOrganizationPinRefusesAnotherTenant(t *testing.T) {
	c := newTest(t, func(w http.ResponseWriter, _ *http.Request) {
		write(w, 200, map[string]any{"organization": map[string]any{"slug": "someone-else"}, "data": map[string]any{}}, nil)
	}, "acme")
	_, err := c.Do(context.Background(), Request{Method: http.MethodGet, Path: "organization_settings"})
	if err == nil || !strings.Contains(err.Error(), "someone-else") {
		t.Fatalf("want a pin refusal naming the real organization, got %v", err)
	}
}

func TestReadOnlyKeyRefusalIsRecognized(t *testing.T) {
	c := newTest(t, func(w http.ResponseWriter, _ *http.Request) {
		write(w, 403, map[string]any{"error": "read_only_key", "message": "This API key holds api:config_read"}, nil)
	}, "")
	_, err := c.Do(context.Background(), Request{Method: http.MethodPost, Path: "teams", Body: map[string]any{"name": "x"}})
	if !IsReadOnlyKey(err) {
		t.Fatalf("want a read-only refusal, got %v", err)
	}
}

func TestErrorsNeverCarryTheKeyOrASecret(t *testing.T) {
	c := newTest(t, func(w http.ResponseWriter, _ *http.Request) {
		write(w, 422, map[string]any{"error": "unprocessable_entity", "message": "bad token pd-secret-123 for " + testKey}, nil)
	}, "")
	_, err := c.Do(context.Background(), Request{Method: http.MethodPost, Path: "outbound_configs", Body: map[string]any{"routing_key": "pd-secret-123"}, Secrets: []string{"pd-secret-123"}})
	if err == nil || strings.Contains(err.Error(), "pd-secret-123") || strings.Contains(err.Error(), testKey) {
		t.Fatalf("secret or key leaked: %v", err)
	}
}
