package engine

import (
	"fmt"
	"strings"
	"testing"
)

func TestPreviewWarningNamesTenRepositoriesAndCountsTheRest(t *testing.T) {
	moved := make([]any, 0, 13)
	for i := 1; i <= 13; i++ {
		moved = append(moved, fmt.Sprintf("acme/r%02d", i))
	}
	p := parsePreview(map[string]any{
		"fingerprint": "f", "kept": float64(2), "moved": moved,
		"changes":           map[string]any{"fix_runner": map[string]any{"from": "platform", "to": "self_hosted"}, "draft_prs": map[string]any{"from": nil, "to": true}},
		"differs_by_branch": []any{"acme/r01"},
		"children":          []any{map[string]any{"organization_name": "Acme EU", "moved": float64(3), "ci_pinned": float64(1), "no_runner": true}},
	})
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
	got := parsePreview(map[string]any{"changes": map[string]any{}, "moved": []any{}}).warning()
	if !strings.Contains(got, "moves no setting") || !strings.Contains(got, "No repository moves") {
		t.Fatalf("the warning is %q", got)
	}
}

func TestPreviewProblemsNameAMissingConfirmation(t *testing.T) {
	r := &facadeResource{spec: Spec{TypeName: "repository_defaults", Confirmations: map[string]Confirmation{
		"ci_config": {Attr: "allow_ci_config", Sentence: "Turning on CI needs confirmation: pass confirm ci_config."},
	}}}
	p := parsePreview(map[string]any{"needs_confirm": []any{"ci_config"}, "refusals": []any{"No runner."}})
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
	if got := r.problems(parsePreview(map[string]any{"needs_confirm": []any{"ci_config"}}), []string{"ci_config"}); len(got) != 0 {
		t.Errorf("a confirmed change still has problems: %v", got)
	}
}

func TestAPreviewWithNoFieldsParsesToNothing(t *testing.T) {
	p := parsePreview(nil)
	if p.Fingerprint != "" || len(p.Moved) != 0 || len(p.Changes) != 0 || p.LeavesParent {
		t.Fatalf("%+v", p)
	}
}
