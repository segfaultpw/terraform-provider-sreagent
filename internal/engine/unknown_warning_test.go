package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type listConfig map[string]types.List

func (c listConfig) GetAttribute(_ context.Context, p path.Path, target any) diag.Diagnostics {
	l, ok := c[p.String()]
	if !ok {
		l = types.ListNull(types.StringType)
	}
	*(target.(*types.List)) = l
	return nil
}

type mapPrivate map[string][]byte

func (m mapPrivate) GetKey(_ context.Context, key string) ([]byte, diag.Diagnostics) {
	return m[key], nil
}

func (m mapPrivate) SetKey(_ context.Context, key string, value []byte) diag.Diagnostics {
	m[key] = value
	return nil
}

func keepUnknownResource() *facadeResource {
	return &facadeResource{spec: Spec{Key: "tiny", TypeName: "tiny", Shape: Singleton, Attrs: []Attr{
		{Name: "meanings_off", Kind: StringList, OneOf: []string{"looking", "take_it"}, KeepUnknown: true},
		{Name: "plain", Kind: StringList, OneOf: []string{"a", "b"}},
	}}}
}

func stringsList(vals ...string) types.List {
	elems := []types.String{}
	for _, v := range vals {
		elems = append(elems, types.StringValue(v))
	}
	l, _ := types.ListValueFrom(context.Background(), types.StringType, elems)
	return l
}

func TestTheUnknownMeaningWarningNamesTheValueAndTheUpgradeAdviceOncePerPlan(t *testing.T) {
	ctx := context.Background()
	r := keepUnknownResource()
	priv := mapPrivate{}
	if d := priv.SetKey(ctx, "unknown_meanings_off", []byte(`["future_meaning"]`)); d.HasError() {
		t.Fatal(d)
	}
	held := r.unknownHeld(ctx, priv)

	var diags diag.Diagnostics
	r.warnUnknown(ctx, listConfig{"meanings_off": stringsList("looking")}, held, &diags)
	if len(diags) != 1 {
		t.Fatalf("a plan that names the attribute warns once, got %d: %v", len(diags), diags)
	}
	d := diags[0]
	if d.Severity() != diag.SeverityWarning {
		t.Errorf("severity = %v, want a warning", d.Severity())
	}
	for _, want := range []string{
		"future_meaning",
		"meanings_off",
		"upgrade the provider",
		"shows it as removed",
		"add it to the configuration when upgrading",
		"switches it back on",
	} {
		if !strings.Contains(d.Detail(), want) {
			t.Errorf("the warning must say %q, got: %s", want, d.Detail())
		}
	}
}

func TestTheUnknownMeaningWarningIsSilentWhenThereIsNothingToSay(t *testing.T) {
	ctx := context.Background()
	r := keepUnknownResource()

	held := map[string][]string{"meanings_off": {"future_meaning"}}
	var diags diag.Diagnostics
	r.warnUnknown(ctx, listConfig{}, held, &diags)
	if len(diags) != 0 {
		t.Errorf("an attribute the configuration leaves out must not warn: %v", diags)
	}

	diags = nil
	r.warnUnknown(ctx, listConfig{"meanings_off": types.ListUnknown(types.StringType)}, held, &diags)
	if len(diags) != 0 {
		t.Errorf("an unknown configuration value must not warn: %v", diags)
	}

	for name, priv := range map[string]privateData{
		"no private state":      nil,
		"nothing held":          mapPrivate{"unknown_meanings_off": []byte(`[]`)},
		"a key that is not set": mapPrivate{},
		"garbage":               mapPrivate{"unknown_meanings_off": []byte(`not json`)},
	} {
		diags = nil
		r.warnUnknown(ctx, listConfig{"meanings_off": stringsList("looking")}, r.unknownHeld(ctx, priv), &diags)
		if len(diags) != 0 {
			t.Errorf("%s: no unknown value is held, so there is nothing to warn about: %v", name, diags)
		}
	}

	diags = nil
	r.warnUnknown(ctx, listConfig{"plain": stringsList("a")}, map[string][]string{"plain": {"zzz"}}, &diags)
	if len(diags) != 0 {
		t.Errorf("an attribute without KeepUnknown never warns: %v", diags)
	}
}

func TestTheUnknownEntriesAreReadFromAnAnsweredRow(t *testing.T) {
	r := keepUnknownResource()
	held := r.unknownIn(map[string]any{"meanings_off": []any{"future_meaning", "looking"}, "plain": []any{"zzz"}})
	if got := held["meanings_off"]; len(got) != 1 || got[0] != "future_meaning" {
		t.Errorf("held = %v, want only future_meaning", held)
	}
	if _, ok := held["plain"]; ok {
		t.Errorf("an attribute without KeepUnknown holds nothing: %v", held)
	}
}

// A state written by an earlier provider release recorded a value as unknown before a later
// release declared it. Once the attribute's OneOf lists it, it is not held any more: it neither
// warns nor is sent again against the configuration, and a value that is still unknown stays held.
func TestAValueThisReleaseDeclaresIsNotHeldAsUnknownFromAnEarlierState(t *testing.T) {
	ctx := context.Background()
	r := keepUnknownResource()
	priv := mapPrivate{"unknown_meanings_off": []byte(`["take_it"]`)}
	if held := r.unknownHeld(ctx, priv); len(held) != 0 {
		t.Fatalf("take_it is declared, so nothing is held, got %v", held)
	}
	var diags diag.Diagnostics
	r.warnUnknown(ctx, listConfig{"meanings_off": stringsList("looking")}, r.unknownHeld(ctx, priv), &diags)
	if len(diags) != 0 {
		t.Errorf("a declared value must not warn: %v", diags)
	}
	body := map[string]any{"meanings_off": []string{"looking"}}
	r.withUnknown(body, r.unknownHeld(ctx, priv))
	if got := body["meanings_off"].([]string); len(got) != 1 || got[0] != "looking" {
		t.Errorf("a declared value was sent again: %v", got)
	}
	// One declared and one still unknown: only the unknown one is held.
	priv = mapPrivate{"unknown_meanings_off": []byte(`["take_it","future_meaning"]`)}
	held := r.unknownHeld(ctx, priv)
	if got := held["meanings_off"]; len(got) != 1 || got[0] != "future_meaning" {
		t.Errorf("held = %v, want only future_meaning", held)
	}
}
