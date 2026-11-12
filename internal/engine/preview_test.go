package engine

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/client"
)

// fullPreview is a preview.data with every key the platform emits.
func fullPreview(over map[string]any) map[string]any {
	m := map[string]any{
		"fingerprint": "f", "kept": float64(0), "moved": []any{}, "changes": map[string]any{},
		"differs_by_branch": []any{}, "children": []any{}, "needs_confirm": []any{}, "refusals": []any{},
		"leaves_parent": false,
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

const dropKey = "\x00drop"

func mustParse(t *testing.T, m map[string]any) preview {
	t.Helper()
	p, err := parsePreview(m)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPreviewWarningNamesTenRepositoriesAndCountsTheRest(t *testing.T) {
	moved := make([]any, 0, 13)
	for i := 1; i <= 13; i++ {
		moved = append(moved, fmt.Sprintf("acme/r%02d", i))
	}
	p := mustParse(t, fullPreview(map[string]any{
		"fingerprint": "f", "kept": float64(2), "moved": moved,
		"changes":           map[string]any{"fix_runner": map[string]any{"from": "platform", "to": "self_hosted"}, "draft_prs": map[string]any{"from": nil, "to": true}},
		"differs_by_branch": []any{"acme/r01"},
		"children":          []any{map[string]any{"organization_name": "Acme EU", "moved": float64(3), "ci_pinned": float64(1), "no_runner": true}},
	}))
	got := p.warning()
	for _, want := range []string{
		"draft_prs from unset to true; fix_runner from platform to self_hosted",
		"13 repositories will read a different value: acme/r01, acme/r02",
		"acme/r10 and 3 more",
		"2 repositories keep a value of their own",
		"Branches differ from the repository's own value for: acme/r01",
		"Child organization Acme EU follows these defaults: 3 repositories will read a different value, 1 repository pinned to keep CI configuration changes, and it has no fix runner registered",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the warning lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "acme/r11") {
		t.Errorf("the warning names an eleventh repository:\n%s", got)
	}
	if strings.Contains(got, "follows its parent") {
		t.Errorf("a preview that does not leave a parent says so:\n%s", got)
	}
}

func TestPreviewWarningSaysWhenNothingMoves(t *testing.T) {
	got := mustParse(t, fullPreview(nil)).warning()
	if !strings.Contains(got, "moves no setting") || !strings.Contains(got, "No repository moves") {
		t.Fatalf("the warning is %q", got)
	}
}

func TestPreviewProblemsNameAMissingConfirmation(t *testing.T) {
	r := &facadeResource{spec: Spec{TypeName: "repository_defaults", Confirmations: map[string]Confirmation{
		"ci_config": {Attr: "allow_ci_config", Sentence: "Turning on CI needs confirmation: pass confirm ci_config."},
	}}}
	p := mustParse(t, fullPreview(map[string]any{"needs_confirm": []any{"ci_config"}, "refusals": []any{"No runner."}}))
	probs := r.problems(p, nil)
	if len(probs) != 2 {
		t.Fatalf("want a refusal and a missing confirmation, got %v", probs)
	}
	if probs[0].attr != "" || !strings.Contains(probs[0].detail, "No runner.") {
		t.Errorf("the refusal is %+v", probs[0])
	}
	if probs[1].attr != "confirm" || !strings.Contains(probs[1].detail, "pass confirm ci_config") || !strings.Contains(probs[1].detail, `confirm = ["ci_config"]`) {
		t.Errorf("the confirmation problem is %+v", probs[1])
	}
	if got := r.problems(mustParse(t, fullPreview(map[string]any{"needs_confirm": []any{"ci_config"}})), []string{"ci_config"}); len(got) != 0 {
		t.Errorf("a confirmed change still has problems: %v", got)
	}
}

// A preview that lacks a key the provider reads is an error, never "nothing
// moves": the warning, the fingerprint and the checks all come from these keys.
func TestAPreviewWithoutAKeyTheProviderReadsIsAnError(t *testing.T) {
	for _, key := range PreviewKeys() {
		_, err := parsePreview(fullPreview(map[string]any{key: dropKey}))
		if err == nil || !strings.Contains(err.Error(), key+" is missing") || !strings.Contains(err.Error(), "cannot read") {
			t.Errorf("a preview without %s parsed: %v", key, err)
		}
	}
	for _, m := range []map[string]any{nil, {}} {
		if _, err := parsePreview(m); err == nil || !strings.Contains(err.Error(), "no preview") {
			t.Errorf("an empty preview parsed: %v", err)
		}
	}
}

func TestAPreviewWithAKeyOfTheWrongTypeIsAnError(t *testing.T) {
	cases := map[string]map[string]any{
		"moved is string, not array":       {"moved": "acme/web"},
		"changes is array, not object":     {"changes": []any{}},
		"fingerprint is empty":             {"fingerprint": ""},
		"fingerprint is number":            {"fingerprint": float64(3)},
		"leaves_parent is string":          {"leaves_parent": "yes"},
		"children[0] is string":            {"children": []any{"Acme EU"}},
		"children[0].moved is missing":     {"children": []any{map[string]any{"organization_name": "Acme EU", "ci_pinned": float64(0), "no_runner": false}}},
		"changes.fix_runner.to is missing": {"changes": map[string]any{"fix_runner": map[string]any{"from": "platform"}}},
	}
	for want, over := range cases {
		if _, err := parsePreview(fullPreview(over)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("want an error naming %q, got %v", want, err)
		}
	}
}

func TestAPreviewKeepsANullFromAndTo(t *testing.T) {
	p := mustParse(t, fullPreview(map[string]any{"changes": map[string]any{"draft_prs": map[string]any{"from": nil, "to": true}}}))
	if got := p.warning(); !strings.Contains(got, "draft_prs from unset to true") {
		t.Fatalf("%s", got)
	}
}

// A 409 answered to a retry may follow a first attempt the platform applied, so
// the error does not claim that nothing was written.
func TestAStaleAnswerToARetriedRequestSaysTheFirstAttemptMayHaveApplied(t *testing.T) {
	r := &facadeResource{spec: Spec{TypeName: "repository_defaults"}}
	var d diag.Diagnostics
	r.staleError(&d, "update", &client.APIError{Status: 409, Retried: true, Preview: fullPreview(nil)})
	if len(d.Errors()) != 1 {
		t.Fatalf("%v", d)
	}
	detail := d.Errors()[0].Detail()
	if !strings.Contains(detail, "first attempt may have been applied") || strings.Contains(detail, "nothing was written") {
		t.Fatalf("%s", detail)
	}
	d = nil
	r.staleError(&d, "update", &client.APIError{Status: 409})
	if got := d.Errors()[0].Detail(); !strings.Contains(got, "nothing was written") || !strings.Contains(got, "terraform plan again") {
		t.Fatalf("%s", got)
	}
}
