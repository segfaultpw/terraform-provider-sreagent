package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func vendored(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "openapi.json")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func server(t *testing.T, key, body string) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+key {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(s.Close)
	return s
}

func TestRunMatchesWithTheKey(t *testing.T) {
	s := server(t, "k", `{"a":1}`)
	if code, msg := run(s.Client(), s.URL, vendored(t, `{"a": 1}`), "k"); code != 0 {
		t.Fatalf("code %d: %s", code, msg)
	}
}

func TestRunReportsDifference(t *testing.T) {
	s := server(t, "k", `{"a":2}`)
	if code, _ := run(s.Client(), s.URL, vendored(t, `{"a":1}`), "k"); code != 1 {
		t.Fatalf("code %d, want 1", code)
	}
}

func TestRunWithoutAKeyNamesTheKeyNotADrift(t *testing.T) {
	s := server(t, "k", `{"a":1}`)
	code, msg := run(s.Client(), s.URL, vendored(t, `{"a":1}`), "")
	if code != 2 || !strings.Contains(msg, "SRE_AGENT_API_KEY") {
		t.Fatalf("code %d msg %q: a refused request must not read as a drift", code, msg)
	}
}
