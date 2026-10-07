package engine

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestDeleteWarningForDiscoveredRows(t *testing.T) {
	spec := Spec{TypeName: "image_target", DiscoveredAttr: "discovered"}
	if w := discoveredWarning(spec, true); !strings.Contains(w, "the next sweep files it again") {
		t.Fatalf("a discovered row must warn, got %q", w)
	}
	if w := discoveredWarning(spec, false); w != "" {
		t.Fatalf("a row created here must not warn, got %q", w)
	}
	if w := discoveredWarning(Spec{TypeName: "team"}, true); w != "" {
		t.Fatalf("a spec without discovered rows must not warn, got %q", w)
	}
}

func TestEveryLongStringOfAJSONSecretIsRedactable(t *testing.T) {
	got := stringLeaves(map[string]any{"host": "10.0.0.1", "auth": map[string]any{"password": "hunter22", "keys": []any{"k-123456"}}, "port": 22, "u": "sre"})
	for _, want := range []string{"10.0.0.1", "hunter22", "k-123456"} {
		if !slices.Contains(got, want) {
			t.Errorf("%s is missing from %v", want, got)
		}
	}
	if slices.Contains(got, "sre") {
		t.Errorf("a string too short to redact safely was kept: %v", got)
	}
}

func TestADroppedKeyOfAMergedObjectIsCleared(t *testing.T) {
	prior := jsontypes.NewNormalizedValue(`{"investigation":"a","suggestion":"b"}`)
	got := clearRemovedKeys(map[string]any{"investigation": "a"}, prior).(map[string]any)
	if got["suggestion"] != "" || got["investigation"] != "a" {
		t.Fatalf("got %v", got)
	}
}

// A secret that is Clearable (base_url) is removed when its value and version
// both leave the configuration; one that is not (api_key) is kept. The body
// and the plan decide through one helper, and these cases hold them together.
var clearSpec = Spec{
	Key: "targets", TypeName: "target", Shape: Generated, Lifecycle: true,
	Attrs: []Attr{
		{Name: "name", Kind: String, Required: true},
		{Name: "base_url", Kind: String, Secret: true, Clearable: true},
		{Name: "api_key", Kind: String, Secret: true},
	},
}

type secretState struct {
	wo      tftypes.Value
	version tftypes.Value
	set     tftypes.Value
}

func some(v int64) tftypes.Value  { return tftypes.NewValue(tftypes.Number, v) }
func text(s string) tftypes.Value { return tftypes.NewValue(tftypes.String, s) }

var (
	nullNumber  = tftypes.NewValue(tftypes.Number, nil)
	nullString  = tftypes.NewValue(tftypes.String, nil)
	boolTrue    = tftypes.NewValue(tftypes.Bool, true)
	boolFalse   = tftypes.NewValue(tftypes.Bool, false)
	unknownText = tftypes.NewValue(tftypes.String, tftypes.UnknownValue)
)

func clearFixture(t *testing.T) (*facadeResource, schema.Schema) {
	t.Helper()
	r := &facadeResource{spec: clearSpec}
	var resp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	return r, resp.Schema
}

func objectOf(t *testing.T, s schema.Schema, base, key secretState) tftypes.Value {
	t.Helper()
	typ := s.Type().TerraformType(context.Background()).(tftypes.Object)
	return tftypes.NewValue(typ, map[string]tftypes.Value{
		"id":                  text("row-1"),
		"name":                text("hook"),
		"base_url_wo":         base.wo,
		"base_url_wo_version": base.version,
		"base_url_set":        base.set,
		"api_key_wo":          key.wo,
		"api_key_wo_version":  key.version,
		"api_key_set":         key.set,
	})
}

type clearCase struct {
	name string
	// state is nil for a create.
	state, config, plan *[2]secretState
	wantBaseURLClear    bool
}

// pair is {base_url, api_key}.
func pair(base, key secretState) *[2]secretState { return &[2]secretState{base, key} }

func sent() secretState {
	return secretState{wo: nullString, version: some(1), set: boolTrue}
}

func gone() secretState { return secretState{wo: nullString, version: nullNumber, set: boolTrue} }

func clearCases() []clearCase {
	value := func(version int64) secretState {
		return secretState{wo: text("https://hooks.example.com/x"), version: some(version), set: boolTrue}
	}
	return []clearCase{
		{name: "both removed while state holds a version clears", state: pair(sent(), sent()), config: pair(gone(), gone()), plan: pair(gone(), gone()), wantBaseURLClear: true},
		{name: "a create never clears", state: nil, config: pair(gone(), gone()), plan: pair(gone(), gone())},
		{name: "an import (no recorded version) never clears", state: pair(gone(), gone()), config: pair(gone(), gone()), plan: pair(gone(), gone())},
		{name: "an unknown value never clears", state: pair(sent(), sent()), config: pair(secretState{wo: unknownText, version: nullNumber, set: boolTrue}, gone()), plan: pair(gone(), gone())},
		{name: "an unchanged version keeps the value", state: pair(sent(), sent()), config: pair(secretState{wo: nullString, version: some(1), set: boolTrue}, gone()), plan: pair(sent(), gone())},
		{name: "a value the platform lost has nothing to clear", state: pair(secretState{wo: nullString, version: some(1), set: boolFalse}, sent()), config: pair(gone(), gone()), plan: pair(secretState{wo: nullString, version: nullNumber, set: boolFalse}, gone())},
		{name: "a new value and version sends and does not clear", state: pair(sent(), sent()), config: pair(value(2), gone()), plan: pair(secretState{wo: nullString, version: some(2), set: boolTrue}, gone())},
	}
}

func fillCase(t *testing.T, s schema.Schema, c clearCase) (state *tfsdk.State, config tfsdk.Config, plan tfsdk.Plan) {
	t.Helper()
	if c.state != nil {
		st := tfsdk.State{Schema: s, Raw: objectOf(t, s, c.state[0], c.state[1])}
		state = &st
	}
	config = tfsdk.Config{Schema: s, Raw: objectOf(t, s, c.config[0], c.config[1])}
	plan = tfsdk.Plan{Schema: s, Raw: objectOf(t, s, c.plan[0], c.plan[1])}
	return state, config, plan
}

func TestAClearableSecretIsClearedOnlyWhenValueAndVersionBothGo(t *testing.T) {
	r, s := clearFixture(t)
	ctx := context.Background()
	for _, c := range clearCases() {
		t.Run(c.name, func(t *testing.T) {
			state, config, plan := fillCase(t, s, c)
			var body map[string]any
			var diags diag.Diagnostics
			if state == nil {
				body, _, diags = r.body(ctx, plan, config, nil, opCreate)
			} else {
				body, _, diags = r.body(ctx, plan, config, *state, opUpdate)
			}
			if diags.HasError() {
				t.Fatal(diags)
			}
			got, present := body["base_url"]
			if c.wantBaseURLClear {
				if !present || got != nil {
					t.Fatalf("want base_url: null in the body, got %v (present %v)", got, present)
				}
			} else if present && got == nil {
				t.Fatalf("base_url: null was sent: %v", body)
			}
			if _, present := body["api_key"]; present {
				t.Fatalf("api_key must be kept when it leaves the configuration, body %v", body)
			}
		})
	}
}

func TestThePlanClearsBaseURLSetExactlyWhenTheBodyClearsIt(t *testing.T) {
	r, s := clearFixture(t)
	ctx := context.Background()
	for _, c := range clearCases() {
		if c.state == nil {
			continue
		}
		t.Run(c.name, func(t *testing.T) {
			state, config, plan := fillCase(t, s, c)
			resp := resource.ModifyPlanResponse{Plan: plan}
			r.ModifyPlan(ctx, resource.ModifyPlanRequest{State: *state, Config: config, Plan: plan}, &resp)
			if resp.Diagnostics.HasError() {
				t.Fatal(resp.Diagnostics)
			}
			var baseSet, keySet types.Bool
			resp.Diagnostics.Append(resp.Plan.GetAttribute(ctx, path.Root("base_url_set"), &baseSet)...)
			resp.Diagnostics.Append(resp.Plan.GetAttribute(ctx, path.Root("api_key_set"), &keySet)...)
			if c.wantBaseURLClear {
				if baseSet.IsNull() || baseSet.IsUnknown() || baseSet.ValueBool() {
					t.Fatalf("a clear plans base_url_set = false, got %v", baseSet)
				}
			} else if !baseSet.IsUnknown() && !baseSet.IsNull() && !baseSet.ValueBool() && c.state[0].set.Equal(boolTrue) {
				t.Fatalf("base_url_set was planned false without a clear")
			}
			if keySet.IsNull() || (!keySet.IsUnknown() && !keySet.ValueBool()) {
				t.Fatalf("api_key_set must stay as it was when api_key leaves the configuration, got %v", keySet)
			}
		})
	}
}

func TestOnlyAClearableSecretCanClear(t *testing.T) {
	r, s := clearFixture(t)
	ctx := context.Background()
	c := clearCases()[0]
	state, config, plan := fillCase(t, s, c)
	for _, a := range clearSpec.Attrs {
		got, diags := r.clearing(ctx, a, config, plan, *state)
		if diags.HasError() {
			t.Fatal(diags)
		}
		if want := a.Name == "base_url"; got != want {
			t.Errorf("%s: clearing = %v, want %v", a.Name, got, want)
		}
	}
	singleton := &facadeResource{spec: Spec{Key: "k", TypeName: "k", Shape: Singleton, Attrs: clearSpec.Attrs}}
	if got, _ := singleton.clearing(ctx, clearSpec.Attrs[1], config, plan, *state); got {
		t.Error("a singleton has no null to send, so it never clears")
	}
}
