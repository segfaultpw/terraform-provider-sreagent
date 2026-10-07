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

func TestRunTrimsAPastedKey(t *testing.T) {
	s := server(t, "sre_ak_abcdefXYZ", `{"a":1}`)
	if code, msg := run(s.Client(), s.URL, vendored(t, `{"a":1}`), " sre_ak_abcdefXYZ\n"); code != 0 {
		t.Fatalf("code %d: %s: a key pasted with surrounding whitespace must still be sent as the key", code, msg)
	}
}

func TestRunNamesTheRefusedKeysPublicPrefixOnly(t *testing.T) {
	s := server(t, "the-real-key", `{"a":1}`)
	code, msg := run(s.Client(), s.URL, vendored(t, `{"a":1}`), "sre_ak_mor68SECRETPART")
	if code != 2 || !strings.Contains(msg, "sre_ak_mor68") {
		t.Fatalf("code %d msg %q: a refusal names the key's prefix so the operator can find it in Settings", code, msg)
	}
	if strings.Contains(msg, "SECRETPART") {
		t.Fatalf("msg %q prints more of the key than its public prefix", msg)
	}
}
