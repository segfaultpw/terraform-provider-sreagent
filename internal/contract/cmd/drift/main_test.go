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

func readFile(path string) ([]byte, error) {
	return os.ReadFile(path) //nolint:gosec // a scratch file the test created
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
	if code, msg := run(s.Client(), s.URL, vendored(t, `{"a": 1}`), "k", false); code != 0 {
		t.Fatalf("code %d: %s", code, msg)
	}
}

func TestRunReportsDifference(t *testing.T) {
	s := server(t, "k", `{"a":2}`)
	if code, _ := run(s.Client(), s.URL, vendored(t, `{"a":1}`), "k", false); code != 1 {
		t.Fatalf("code %d, want 1", code)
	}
}

func TestRunWithoutAKeyNamesTheKeyNotADrift(t *testing.T) {
	s := server(t, "k", `{"a":1}`)
	code, msg := run(s.Client(), s.URL, vendored(t, `{"a":1}`), "", false)
	if code != 2 || !strings.Contains(msg, "SRE_AGENT_API_KEY") {
		t.Fatalf("code %d msg %q: a refused request must not read as a drift", code, msg)
	}
}

func TestRunTrimsAPastedKey(t *testing.T) {
	s := server(t, "sre_ak_abcdefXYZ", `{"a":1}`)
	if code, msg := run(s.Client(), s.URL, vendored(t, `{"a":1}`), " sre_ak_abcdefXYZ\n", false); code != 0 {
		t.Fatalf("code %d: %s: a key pasted with surrounding whitespace must still be sent as the key", code, msg)
	}
}

func TestRunNamesTheRefusedKeysPublicPrefixOnly(t *testing.T) {
	s := server(t, "the-real-key", `{"a":1}`)
	code, msg := run(s.Client(), s.URL, vendored(t, `{"a":1}`), "sre_ak_mor68SECRETPART", false)
	if code != 2 || !strings.Contains(msg, "sre_ak_mor68") {
		t.Fatalf("code %d msg %q: a refusal names the key's prefix so the operator can find it in Settings", code, msg)
	}
	if strings.Contains(msg, "SECRETPART") {
		t.Fatalf("msg %q prints more of the key than its public prefix", msg)
	}
}

const base = `{
  "openapi": "3.1.0",
  "info": {"title": "API", "description": "text", "version": "1.0.0"},
  "paths": {
    "/things": {
      "post": {
        "summary": "Create",
        "description": "Creates a thing.",
        "parameters": [{"name": "q", "in": "query", "description": "filter", "schema": {"type": "string", "example": "a"}}],
        "requestBody": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/thing"}}}},
        "responses": {"201": {"description": "created"}}
      }
    }
  },
  "components": {"schemas": {"thing": {
    "type": "object",
    "title": "Thing",
    "description": "A thing.",
    "required": ["name"],
    "properties": {
      "name": {"type": "string", "description": "The name.", "examples": ["x"]},
      "kind": {"type": "string", "enum": ["a", "b"], "default": {"description": "a"}},
      "description": {"type": "string", "description": "The description property."},
      "title": {"type": "string"}
    }
  }}},
  "x-sreagent-resources": {"things": {"fields": ["name"], "computed_fields": {"kind": "a"}}}
}`

func TestRunComparesShapeNotProse(t *testing.T) {
	tests := []struct {
		name     string
		live     string
		wantCode int
		wantMsg  string
	}{
		{"identical", base, 0, "contract current"},
		{
			"a description, summary, title and example reworded",
			strings.NewReplacer(
				`"Creates a thing."`, `"Makes a new thing."`,
				`"Create"`, `"Make"`,
				`"Thing"`, `"The thing"`,
				`"The name."`, `"What it is called."`,
				`"examples": ["x"]`, `"examples": ["y", "z"]`,
				`"example": "a"`, `"example": "b"`,
				`"filter"`, `"narrow the list"`,
			).Replace(base),
			0, "contract current (descriptions differ)",
		},
		{
			"a documentation key added where there was none",
			strings.Replace(base, `"title": {"type": "string"}`, `"title": {"type": "string", "description": "now documented"}`, 1),
			0, "contract current (descriptions differ)",
		},
		{
			"a new property",
			strings.Replace(base, `"title": {"type": "string"}`, `"title": {"type": "string"}, "extra": {"type": "string"}`, 1),
			1, "differs",
		},
		{
			"a property named description is shape, not prose",
			strings.Replace(base, `"description": {"type": "string", "description": "The description property."},`, ``, 1),
			1, "differs",
		},
		{
			"a property named title is shape, not prose",
			strings.Replace(base, `,
      "title": {"type": "string"}`, ``, 1),
			1, "differs",
		},
		{
			"an enum value added",
			strings.Replace(base, `["a", "b"]`, `["a", "b", "c"]`, 1),
			1, "differs",
		},
		{
			"an enum value renamed",
			strings.Replace(base, `["a", "b"]`, `["a", "z"]`, 1),
			1, "differs",
		},
		{
			"a required list changed",
			strings.Replace(base, `"required": ["name"]`, `"required": ["name", "kind"]`, 1),
			1, "differs",
		},
		{
			"a type changed",
			strings.Replace(base, `"name": {"type": "string"`, `"name": {"type": ["string", "null"]`, 1),
			1, "differs",
		},
		{
			"a default changed, including inside a default object",
			strings.Replace(base, `"default": {"description": "a"}`, `"default": {"description": "b"}`, 1),
			1, "differs",
		},
		{
			"a path removed",
			strings.Replace(base, `"/things"`, `"/other"`, 1),
			1, "differs",
		},
		{
			"a parameter location changed",
			strings.Replace(base, `"in": "query"`, `"in": "header"`, 1),
			1, "differs",
		},
		{
			"a response status added",
			strings.Replace(base, `"201": {"description": "created"}`, `"201": {"description": "created"}, "409": {"description": "conflict"}`, 1),
			1, "differs",
		},
		{
			"the provider's resource table changed",
			strings.Replace(base, `"fields": ["name"]`, `"fields": ["name", "kind"]`, 1),
			1, "differs",
		},
		{
			"the API version changed",
			strings.Replace(base, `"version": "1.0.0"`, `"version": "1.1.0"`, 1),
			1, "differs",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := server(t, "k", tt.live)
			path := vendored(t, base)
			code, msg := run(s.Client(), s.URL, path, "k", false)
			if code != tt.wantCode || !strings.Contains(msg, tt.wantMsg) {
				t.Fatalf("code %d msg %q, want code %d containing %q", code, msg, tt.wantCode, tt.wantMsg)
			}
			got, err := readFile(path)
			if err != nil || string(got) != base {
				t.Fatalf("without -write the vendored copy must not change (err %v)", err)
			}
		})
	}
}

func TestRunWriteReplacesTheVendoredCopyOnRealAndProseChanges(t *testing.T) {
	prose := strings.Replace(base, `"Creates a thing."`, `"Makes a new thing."`, 1)
	shaped := strings.Replace(base, `["a", "b"]`, `["a", "b", "c"]`, 1)
	tests := []struct {
		name     string
		live     string
		wantCode int
		wantMsg  string
	}{
		{"a prose change is written and still current", prose, 0, "contract current (descriptions differ); wrote the live document"},
		{"a shape change is written and reported", shaped, 1, "wrote the live document"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := server(t, "k", tt.live)
			path := vendored(t, base)
			code, msg := run(s.Client(), s.URL, path, "k", true)
			if code != tt.wantCode || !strings.Contains(msg, tt.wantMsg) {
				t.Fatalf("code %d msg %q, want code %d containing %q", code, msg, tt.wantCode, tt.wantMsg)
			}
			got, err := readFile(path)
			if err != nil || string(got) != tt.live {
				t.Fatalf("the vendored copy is not the live bytes (err %v)", err)
			}
			if entries, _ := os.ReadDir(filepath.Dir(path)); len(entries) != 1 {
				t.Fatalf("the write left a temporary file behind: %v", entries)
			}
			if code, msg := run(s.Client(), s.URL, path, "k", true); code != 0 || msg != "contract current" {
				t.Fatalf("a second run after the write: code %d msg %q, want current with nothing to write", code, msg)
			}
		})
	}
}

func TestRunWriteLeavesTheCopyAloneWhenTheComparisonCannotBeMade(t *testing.T) {
	s := server(t, "the-real-key", base)
	path := vendored(t, `{"a":1}`)
	if code, _ := run(s.Client(), s.URL, path, "wrong", true); code != 2 {
		t.Fatalf("code %d, want 2", code)
	}
	if got, _ := readFile(path); string(got) != `{"a":1}` {
		t.Fatalf("a refused request must not rewrite the vendored copy: %s", got)
	}
}

func TestRunWriteFailsWhenTheTargetDirectoryIsNotWritable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes into a read-only directory")
	}
	s := server(t, "k", `{"a":2}`)
	path := vendored(t, `{"a":1}`)
	dir := filepath.Dir(path)
	if err := os.Chmod(dir, 0o500); err != nil { //nolint:gosec // a scratch test directory
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) }) //nolint:gosec // restoring the scratch directory so TempDir can remove it
	if code, _ := run(s.Client(), s.URL, path, "k", true); code != 2 {
		t.Fatalf("code %d, want 2 when the copy cannot be written", code)
	}
}
