package engine

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/client"
)

var childSpec = Spec{
	Key: "github_child_access", TypeName: "github_child_access", ListName: "github_child_access_rows",
	Shape: PathAndQuery, IDAttr: "child_organization", QueryAttr: "account", Lifecycle: true,
	AdoptOnly: true, Previewed: true, ProbeFingerprint: "preview", AccessPreview: true,
	LeaveOnDestroy: "The child keeps what it was given.",
	Attrs: []Attr{
		{Name: "child_organization", Kind: String, Address: true, Required: true, CreateOnly: true},
		{Name: "account", Kind: String, Address: true, Required: true, CreateOnly: true},
		{Name: "mode", Kind: String, UpdateOnly: true},
	},
}

func TestAPathAndQueryIdRoundTripsAndNamesItsQuery(t *testing.T) {
	for _, c := range []struct{ segment, value, id string }{
		{"child-1", "acme", "child-1?account=acme"},
		{"child-1", "", "child-1"},
		{"acme labs", "GitHub account 42", "acme%20labs?account=GitHub+account+42"},
		{"c", "a&b=c?d", "c?account=a%26b%3Dc%3Fd"},
	} {
		if got := pathQueryID("account", c.segment, c.value); got != c.id {
			t.Errorf("pathQueryID(%q, %q) = %q, want %q", c.segment, c.value, got, c.id)
		}
		segment, value, ok := splitPathQueryID("account", c.id)
		if !ok || segment != c.segment || value != c.value {
			t.Errorf("splitPathQueryID(%q) = %q, %q, %v; want %q, %q", c.id, segment, value, ok, c.segment, c.value)
		}
	}
	for _, id := range []string{"child-1?environment=acme", "child-1?account", "child-1?account=a&b=c", "child-1?account=a?b"} {
		if _, _, ok := splitPathQueryID("account", id); ok {
			t.Errorf("%q must be refused: it carries more than the account", id)
		}
	}
}

func TestAPathAndQueryRowIsAddressedByItsChildInThePathAndItsAccountInTheQuery(t *testing.T) {
	r := &facadeResource{spec: childSpec}
	p, q := r.rowPath("child-1?account=acme")
	if p != "github_child_access/child-1" || q.Get("account") != "acme" || len(q) != 1 {
		t.Fatalf("got %q %v", p, q)
	}
	if p, q := r.rowPath("child-1"); p != "github_child_access/child-1" || len(q) != 0 {
		t.Fatalf("an id without an account sends no query: %q %v", p, q)
	}
	row := map[string]any{"child_organization": "child-1", "account": "acme"}
	if id := RowID(childSpec, row); id != "child-1?account=acme" {
		t.Fatalf("RowID = %q", id)
	}
	if id := RowID(childSpec, map[string]any{"child_organization": "child-1"}); id != "child-1" {
		t.Fatalf("a row that names no account is addressed by its child: %q", id)
	}
}

// The service binding keeps its own handling: the generalization must not move it.
func TestAServiceBindingIsStillAddressedByItsServiceAndEnvironment(t *testing.T) {
	spec := Spec{Key: "service_bindings", Shape: ServiceBinding}
	r := &facadeResource{spec: spec}
	p, q := r.rowPath("checkout?environment=prod+eu")
	if p != "service_bindings/checkout" || q.Get("environment") != "prod eu" {
		t.Fatalf("got %q %v", p, q)
	}
	if p, q := r.rowPath("checkout"); p != "service_bindings/checkout" || len(q) != 0 {
		t.Fatalf("got %q %v", p, q)
	}
	if id := RowID(spec, map[string]any{"service": "checkout", "environment": "prod eu"}); id != "checkout?environment=prod+eu" {
		t.Fatalf("RowID = %q", id)
	}
	if id := RowID(spec, map[string]any{"service": "checkout", "environment": ""}); id != "checkout" {
		t.Fatalf("RowID = %q", id)
	}
}

func accessPreviewFixture(over map[string]any) map[string]any {
	view := func(mode string, shares bool, repos ...string) map[string]any {
		items := []any{}
		for _, r := range repos {
			items = append(items, map[string]any{"full_name": r, "repository_id": float64(1), "moved": false})
		}
		return map[string]any{"mode": mode, "repositories": items, "repository_count": float64(len(repos)), "context_repo_shared": shares}
	}
	reach := func(names ...string) map[string]any {
		items := []any{}
		for _, n := range names {
			items = append(items, map[string]any{"full_name": n, "repository_id": nil})
		}
		return map[string]any{"count": float64(len(names)), "repositories": items, "more": float64(0)}
	}
	m := map[string]any{
		"fingerprint": "fp-1", "narrows": true, "widens": false, "changes": true,
		"account": "acme", "account_id": float64(42), "installation_id": float64(7),
		"child":  map[string]any{"id": "child-1", "name": "acme-labs"},
		"before": view("all", true), "after": view("selected", true, "acme/web"),
		"stops_reaching": reach("acme/api", "acme/infra"), "starts_reaching": reach(),
		"auto_merge_off": float64(1), "open_fix_prs": float64(3), "waiting": float64(2),
		"waiting_detail": map[string]any{"held": float64(1), "queued_for_runner": float64(1), "in_flight": float64(0)},
		"cards":          float64(4), "by_repository": []any{}, "counts_complete": true,
		"later_repositories_not_given": true, "refusals": []any{},
	}
	for k, v := range over {
		if v == dropKey {
			delete(m, k)
			continue
		}
		m[k] = v
	}
	return m
}

func TestAnAccessPreviewIsReadAndSaysWhatTheChildStopsReaching(t *testing.T) {
	p, err := parseAccessPreview(accessPreviewFixture(nil))
	if err != nil {
		t.Fatal(err)
	}
	if p.Fingerprint != "fp-1" || !p.Access {
		t.Fatalf("got %+v", p)
	}
	got := p.warning()
	for _, want := range []string{
		"child organization acme-labs and the GitHub account acme",
		"It may use every repository of the account now and will use only acme/web",
		"It stops reaching 2 repositories: acme/api, acme/infra",
		"3 open fix pull requests of the child's stay open on GitHub",
		"auto-merge is turned off on 1 of them",
		"2 fix requests still waiting or running stop at their next step (1 held, 1 queued for the runner, 0 running)",
		"4 cards keep their links and history. Nothing is closed or deleted",
		"Repositories the account gets later are not given to the child until they are named here",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the warning lacks %q:\n%s", want, got)
		}
	}
}

func TestAnAccessPreviewOfSharingAloneNamesNoAccount(t *testing.T) {
	m := accessPreviewFixture(map[string]any{
		"account": nil, "account_id": nil, "installation_id": nil, "narrows": true, "widens": false,
		"after":          map[string]any{"mode": nil, "repositories": []any{}, "repository_count": float64(0), "context_repo_shared": false},
		"before":         map[string]any{"mode": nil, "repositories": []any{}, "repository_count": float64(0), "context_repo_shared": true},
		"stops_reaching": map[string]any{"count": float64(0), "repositories": []any{}, "more": float64(0)},
		"open_fix_prs":   float64(0), "waiting": float64(0), "cards": float64(0), "auto_merge_off": float64(0),
		"waiting_detail":               map[string]any{"held": float64(0), "queued_for_runner": float64(0), "in_flight": float64(0)},
		"later_repositories_not_given": false,
	})
	p, err := parseAccessPreview(m)
	if err != nil {
		t.Fatal(err)
	}
	got := p.warning()
	if strings.Contains(got, "GitHub account") || !strings.Contains(got, "It will stop using this organization's context repository") || !strings.Contains(got, "Nothing of the child is held on those repositories") {
		t.Fatalf("got %s", got)
	}
}

func TestAnAccessPreviewThatChangesNothingSaysSo(t *testing.T) {
	p, err := parseAccessPreview(accessPreviewFixture(map[string]any{"changes": false, "narrows": false}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(p.warning(), "The platform has nothing to write") {
		t.Fatalf("got %s", p.warning())
	}
}

func TestAnAccessPreviewMissingAKeyTheProviderReadsIsNotAPreview(t *testing.T) {
	for _, key := range AccessPreviewKeys() {
		_, err := parseAccessPreview(accessPreviewFixture(map[string]any{key: dropKey}))
		if err == nil || !strings.Contains(err.Error(), key+" is missing") {
			t.Errorf("a preview without %s must be refused naming it, got %v", key, err)
		}
	}
	for _, c := range []struct {
		over map[string]any
		want string
	}{
		{map[string]any{"narrows": "yes"}, "narrows is string, not boolean"},
		{map[string]any{"account_id": "42"}, "account_id is string, not number|null"},
		{map[string]any{"fingerprint": ""}, "fingerprint is empty"},
		{map[string]any{"before": map[string]any{"mode": "all"}}, "before.repositories is missing"},
		{map[string]any{"stops_reaching": map[string]any{"count": "2", "repositories": []any{}, "more": float64(0)}}, "stops_reaching.count is string, not number"},
		{map[string]any{"refusals": []any{map[string]any{"repository": "x"}}}, "refusals[0].code is missing"},
		{map[string]any{"stale": "no"}, "stale is string, not boolean"},
	} {
		_, err := parseAccessPreview(accessPreviewFixture(c.over))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("want an error naming %q, got %v", c.want, err)
		}
	}
	if _, err := parseAccessPreview(nil); err == nil {
		t.Error("an answer with no preview is not one")
	}
}

func TestARefusalOfAnAccessPreviewIsAProblemOfTheChange(t *testing.T) {
	m := accessPreviewFixture(map[string]any{"refusals": []any{map[string]any{"repository": "acme/nope", "code": "not_in_installation", "reason": "acme/nope is not a repository of acme."}}})
	p, err := parseAccessPreview(m)
	if err != nil {
		t.Fatal(err)
	}
	r := &facadeResource{spec: childSpec}
	probs := r.problems(p, nil)
	if len(probs) != 1 || !strings.Contains(probs[0].detail, "acme/nope: acme/nope is not a repository of acme.") {
		t.Fatalf("got %+v", probs)
	}
}

// A fake platform that answers what the child access PUT answers, and records
// what it was sent.
type wire struct {
	t        *testing.T
	requests []wireRequest
	answer   func(body map[string]any) (int, map[string]any, string)
}

type wireRequest struct {
	method, path, account, ifMatch string
	body                           map[string]any
}

func (w *wire) client() *client.Client {
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.requests = append(w.requests, wireRequest{method: r.Method, path: r.URL.Path, account: r.URL.Query().Get("account"), ifMatch: r.Header.Get("If-Match"), body: body})
		status, out, etag := w.answer(body)
		if etag != "" {
			rw.Header().Set("ETag", etag)
		}
		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(status)
		_ = json.NewEncoder(rw).Encode(out)
	}))
	w.t.Cleanup(srv.Close)
	c, err := client.New(client.Config{BaseURL: srv.URL, APIKey: "sre_ak_unit", MaxRetries: 0}) //nolint:gosec // a test server
	if err != nil {
		w.t.Fatal(err)
	}
	return c
}

func previewBody(status int, over map[string]any) (int, map[string]any, string) {
	return status, map[string]any{"error": "x", "message": "m", "preview": map[string]any{"data": accessPreviewFixture(over)}}, ""
}

func writtenBody() (int, map[string]any, string) {
	return 200, map[string]any{"organization": map[string]any{"id": "o"}, "data": map[string]any{"child_organization": "child-1", "account": "acme", "mode": "selected"}}, `"v2"`
}

func TestAProbeAsksForThePreviewWithAFingerprintNoPreviewCarries(t *testing.T) {
	w := &wire{t: t, answer: func(body map[string]any) (int, map[string]any, string) {
		if body["expected_fingerprint"] != "preview" {
			t.Errorf("the probe carried %v", body["expected_fingerprint"])
		}
		return previewBody(409, map[string]any{"stale": true, "applied": false})
	}}
	r := &facadeResource{spec: childSpec, client: w.client()}
	p, written, err := r.probe(context.Background(), "child-1?account=acme", map[string]any{"mode": "selected"}, `"v1"`, nil)
	if err != nil || written != nil || p.Fingerprint != "fp-1" {
		t.Fatalf("got %+v %v %v", p, written, err)
	}
	got := w.requests[0]
	if got.method != http.MethodPut || got.path != "/api/v1/config/github_child_access/child-1" || got.account != "acme" || got.ifMatch != `"v1"` {
		t.Fatalf("the probe went to %+v", got)
	}
	if got.body["mode"] != "selected" {
		t.Fatalf("the probe lost the change: %v", got.body)
	}
}

func TestAProbeThatTheWritesAtOnceAnswerIsNothingToWrite(t *testing.T) {
	w := &wire{t: t, answer: func(map[string]any) (int, map[string]any, string) { return writtenBody() }}
	r := &facadeResource{spec: childSpec, client: w.client()}
	_, written, err := r.probe(context.Background(), "child-1?account=acme", map[string]any{"mode": "selected"}, "", nil)
	if err != nil || written == nil {
		t.Fatalf("a 200 to the probe is the platform's answer that it has nothing to write: %v %v", written, err)
	}
}

func TestAStaleFingerprintAtTheApplyFailsWithoutWritingAndShowsTheFreshPreview(t *testing.T) {
	w := &wire{t: t, answer: func(map[string]any) (int, map[string]any, string) {
		return previewBody(409, map[string]any{"stale": true, "applied": false, "open_fix_prs": float64(9)})
	}}
	r := &facadeResource{spec: childSpec, client: w.client()}
	var diags diag.Diagnostics
	_, _, err := r.writeRow(context.Background(), &diags, "update", "child-1?account=acme", map[string]any{"mode": "selected"}, `"v1"`, nil, types.StringValue("fp-old"))
	if !errors.Is(err, errReported) || !diags.HasError() {
		t.Fatalf("got %v %v", err, diags)
	}
	text := diags[0].Detail()
	if !strings.Contains(text, "changed since the plan") || !strings.Contains(text, "nothing was written") || !strings.Contains(text, "9 open fix pull requests") {
		t.Fatalf("got %s", text)
	}
	if w.requests[0].body["expected_fingerprint"] != "fp-old" || len(w.requests) != 1 {
		t.Fatalf("got %+v", w.requests)
	}
}

// A plan that found nothing to preview never showed a narrowing, so an apply that
// meets one refuses it instead of applying it.
func TestAnApplyThatMeetsAPreviewTheNullPlanNeverShowedRefusesIt(t *testing.T) {
	w := &wire{t: t, answer: func(body map[string]any) (int, map[string]any, string) {
		if _, sent := body["expected_fingerprint"]; sent {
			t.Errorf("an apply with no planned fingerprint sent one: %v", body)
		}
		return previewBody(428, map[string]any{"applied": false})
	}}
	r := &facadeResource{spec: childSpec, client: w.client()}
	var diags diag.Diagnostics
	_, _, err := r.writeRow(context.Background(), &diags, "update", "child-1?account=acme", map[string]any{"mode": "selected"}, `"v1"`, nil, types.StringNull())
	if !errors.Is(err, errReported) || !diags.HasError() || !strings.Contains(diags[0].Detail(), "which the plan did not show, so nothing was written") {
		t.Fatalf("got %v %v", err, diags)
	}
	if len(w.requests) != 1 {
		t.Fatalf("the apply went on after the refusal: %d requests", len(w.requests))
	}
}

func TestAnApplyWithNoPlannedPreviewTakesOneItselfAndAppliesItsFingerprint(t *testing.T) {
	w := &wire{t: t, answer: func(body map[string]any) (int, map[string]any, string) {
		switch body["expected_fingerprint"] {
		case "preview":
			return previewBody(409, map[string]any{"stale": true, "applied": false})
		case "fp-1":
			return writtenBody()
		}
		t.Errorf("unexpected body %v", body)
		return 500, map[string]any{}, ""
	}}
	r := &facadeResource{spec: childSpec, client: w.client()}
	var diags diag.Diagnostics
	out, used, err := r.writeRow(context.Background(), &diags, "update", "child-1?account=acme", map[string]any{"mode": "selected"}, `"v1"`, nil, types.StringUnknown())
	if err != nil || out == nil || used != "fp-1" || diags.HasError() {
		t.Fatalf("got %v %q %v %v", out, used, err, diags)
	}
	if diags.WarningsCount() != 1 || !strings.Contains(diags[0].Detail(), "The plan could not preview this change") {
		t.Fatalf("the apply did not say it took the preview itself: %v", diags)
	}
}

func TestAnApplyWithNoPlannedPreviewForAChangeThePlatformWritesAtOnceIsDone(t *testing.T) {
	w := &wire{t: t, answer: func(map[string]any) (int, map[string]any, string) { return writtenBody() }}
	r := &facadeResource{spec: childSpec, client: w.client()}
	var diags diag.Diagnostics
	out, used, err := r.writeRow(context.Background(), &diags, "create", "child-1?account=acme", map[string]any{"mode": "selected"}, `"v1"`, nil, types.StringUnknown())
	if err != nil || out == nil || used != "" || diags.HasError() {
		t.Fatalf("got %v %q %v %v", out, used, err, diags)
	}
	if len(w.requests) != 1 {
		t.Fatalf("a probe the platform answered with a 200 needs no second request: %d", len(w.requests))
	}
}
