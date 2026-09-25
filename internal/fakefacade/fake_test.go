package fakefacade

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/engine"
)

var testSpec = engine.Spec{Key: "things", TypeName: "thing", Shape: engine.Generated, Lifecycle: true, Attrs: []engine.Attr{
	{Name: "name", Kind: engine.String, Required: true},
	{Name: "token", Kind: engine.String, Secret: true},
}}

func call(t *testing.T, method, url string, body any, ifMatch string) (*http.Response, map[string]any) {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(method, url, bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer sre_ak_test")
	if ifMatch != "" {
		req.Header.Set("If-Match", ifMatch)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp, out
}

func TestFakeRefusesUndeclaredFields(t *testing.T) {
	f := New(t, []engine.Spec{testSpec})
	resp, _ := call(t, "POST", f.URL+"/api/v1/config/things", map[string]any{"name": "a", "nope": 1}, "")
	if resp.StatusCode != 422 {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

func TestFakeStoresSecretsOnlyAsFlags(t *testing.T) {
	f := New(t, []engine.Spec{testSpec})
	_, out := call(t, "POST", f.URL+"/api/v1/config/things", map[string]any{"name": "a", "token": "s3cret"}, "")
	row := out["data"].(map[string]any)
	if _, leaked := row["token"]; leaked || row["token_set"] != true {
		t.Fatalf("row %v", row)
	}
}

func TestFakeRefusesAStaleIfMatch(t *testing.T) {
	f := New(t, []engine.Spec{testSpec})
	created, out := call(t, "POST", f.URL+"/api/v1/config/things", map[string]any{"name": "a"}, "")
	id := out["data"].(map[string]any)["id"].(string)
	stale := created.Header.Get("ETag")
	f.Mutate("things", id, func(r map[string]any) { r["name"] = "b" })
	resp, body := call(t, "PUT", f.URL+"/api/v1/config/things/"+id, map[string]any{"name": "c"}, stale)
	if resp.StatusCode != 412 || body["data"] == nil {
		t.Fatalf("status %d body %v", resp.StatusCode, body)
	}
}
