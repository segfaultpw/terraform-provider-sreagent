package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/defaults"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/client"
)

func ptr(n int64) *int64 { return &n }

// slaSpec is a scratch singleton shaped like the security SLA policy: an Int
// whose null is the real setting "no SLA", read as 0 and sent as null.
var slaSpec = Spec{
	Key: "sla", TypeName: "sla", Shape: Singleton, Lifecycle: true,
	Attrs: []Attr{
		{Name: "high_days", Kind: Int, NullAs: "0", Min: ptr(1), Max: ptr(365)},
		{Name: "channel", Kind: String, NullAs: `""`, Pattern: `^#\w+(-\w+)*$`, MaxLength: 80},
	},
}

func fixtureOf(t *testing.T, spec Spec) (*facadeResource, rschema.Schema) {
	t.Helper()
	r := &facadeResource{spec: spec}
	var resp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	return r, resp.Schema
}

// rawObject is an object of s with the named fields set and every other null.
func rawObject(t *testing.T, s rschema.Schema, fields map[string]tftypes.Value) tftypes.Value {
	t.Helper()
	typ := s.Type().TerraformType(context.Background()).(tftypes.Object)
	values := map[string]tftypes.Value{}
	for name, ty := range typ.AttributeTypes {
		if v, ok := fields[name]; ok {
			values[name] = v
		} else {
			values[name] = tftypes.NewValue(ty, nil)
		}
	}
	return tftypes.NewValue(typ, values)
}

func TestANullAsValueIsSentAsNullAndReadBack(t *testing.T) {
	r, sch := fixtureOf(t, slaSpec)
	ctx := context.Background()
	mk := func(days tftypes.Value) tftypes.Value {
		return rawObject(t, sch, map[string]tftypes.Value{"id": text("sla"), "high_days": days})
	}
	cases := []struct {
		name        string
		state, plan tftypes.Value
		op          op
		want        any
		sent        bool
	}{
		{"0 over a stated 30 sends null", mk(some(30)), mk(some(0)), opUpdate, nil, true},
		{"0 on a create sends null", tftypes.Value{}, mk(some(0)), opCreate, nil, true},
		{"a real value is sent as it is", mk(some(0)), mk(some(7)), opUpdate, int64(7), true},
		{"0 over 0 is no change", mk(some(0)), mk(some(0)), opUpdate, nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			plan := tfsdk.Plan{Schema: sch, Raw: c.plan}
			config := tfsdk.Config{Schema: sch, Raw: c.plan}
			var state valueSource
			if c.op == opUpdate {
				state = tfsdk.State{Schema: sch, Raw: c.state}
			}
			body, _, diags := r.body(ctx, plan, config, state, c.op)
			if diags.HasError() {
				t.Fatal(diags)
			}
			got, present := body["high_days"]
			if present != c.sent || got != c.want {
				t.Fatalf("body = %v, want present %v value %v", body, c.sent, c.want)
			}
		})
	}

	// A null answer reads as the literal; an answer that holds a number reads as it is.
	for answer, want := range map[string]int64{`{"high_days": null}`: 0, `{"high_days": 14}`: 14} {
		state := tfsdk.State{Schema: sch, Raw: rawObject(t, sch, nil)}
		diags := r.writeState(ctx, &client.Response{Data: []byte(answer)}, nil, &state, nil, nil)
		if diags.HasError() {
			t.Fatal(diags)
		}
		var got types.Int64
		diags.Append(state.GetAttribute(ctx, path.Root("high_days"), &got)...)
		if diags.HasError() || got.IsNull() || got.ValueInt64() != want {
			t.Fatalf("answer %s read as %v, want %d (%v)", answer, got, want, diags)
		}
	}
}

func TestTheDataSourceReadsNullForANullAsAttribute(t *testing.T) {
	d := &facadeDataSource{spec: slaSpec}
	values, _, err := d.values(map[string]any{"high_days": nil, "channel": nil})
	if err != nil {
		t.Fatal(err)
	}
	if !values["high_days"].IsNull() || !values["channel"].IsNull() {
		t.Fatalf("a data source keeps the platform's null: %v", values)
	}
}

func attributeNamed(t *testing.T, spec Spec, name string) rschema.Attribute {
	t.Helper()
	_, sch := fixtureOf(t, spec)
	a, ok := sch.Attributes[name]
	if !ok {
		t.Fatalf("no attribute %s", name)
	}
	return a
}

// The validate helpers run the validators an attribute carries the way the
// framework does, so a test sees the diagnostics a plan would.
func validateString(t *testing.T, spec Spec, name, value string) diag.Diagnostics {
	t.Helper()
	var all diag.Diagnostics
	for _, v := range attributeNamed(t, spec, name).(rschema.StringAttribute).Validators {
		resp := validator.StringResponse{}
		v.ValidateString(context.Background(), validator.StringRequest{Path: path.Root(name), ConfigValue: types.StringValue(value)}, &resp)
		all.Append(resp.Diagnostics...)
	}
	return all
}

func validateInt(t *testing.T, spec Spec, name string, value int64) diag.Diagnostics {
	t.Helper()
	var all diag.Diagnostics
	for _, v := range attributeNamed(t, spec, name).(rschema.Int64Attribute).Validators {
		resp := validator.Int64Response{}
		v.ValidateInt64(context.Background(), validator.Int64Request{Path: path.Root(name), ConfigValue: types.Int64Value(value)}, &resp)
		all.Append(resp.Diagnostics...)
	}
	return all
}

func validateList(t *testing.T, spec Spec, name string, values ...string) diag.Diagnostics {
	t.Helper()
	var all diag.Diagnostics
	for _, v := range attributeNamed(t, spec, name).(rschema.ListAttribute).Validators {
		resp := validator.ListResponse{}
		v.ValidateList(context.Background(), validator.ListRequest{Path: path.Root(name), ConfigValue: listOf(values...)}, &resp)
		all.Append(resp.Diagnostics...)
	}
	return all
}

func TestABoundAcceptsItsNullAs(t *testing.T) {
	for value, bad := range map[int64]bool{0: false, 1: false, 365: false, -1: true, 366: true} {
		d := validateInt(t, slaSpec, "high_days", value)
		if d.HasError() != bad {
			t.Errorf("%d: errors = %v, want error %v", value, d, bad)
		}
		if bad {
			text := d.Errors()[0].Summary() + " " + d.Errors()[0].Detail()
			if !strings.Contains(text, "high_days") || !strings.Contains(text, "1") || !strings.Contains(text, "365") {
				t.Errorf("%d: the refusal must name the attribute and the bounds, got %q", value, text)
			}
		}
	}
}

func TestAPatternAcceptsItsNullAs(t *testing.T) {
	for value, bad := range map[string]bool{"": false, "#security": false, "#a-b": false, "security": true, "#a--b": true, "#": true, "#security\n": true} {
		d := validateString(t, slaSpec, "channel", value)
		if d.HasError() != bad {
			t.Errorf("%q: errors = %v, want error %v", value, d, bad)
		}
		if bad {
			if text := d.Errors()[0].Detail(); !strings.Contains(text, "channel") || !strings.Contains(text, `^#\w+(-\w+)*$`) {
				t.Errorf("%q: the refusal must name the attribute and the pattern, got %q", value, text)
			}
		}
	}
}

func TestCodePointLengthsCountCharactersNotBytes(t *testing.T) {
	spec := Spec{Key: "k", TypeName: "k", Shape: Generated, Lifecycle: true, Attrs: []Attr{{Name: "name", Kind: String, Required: true, MinLength: 1, MaxLength: 80}}}
	if d := validateString(t, spec, "name", strings.Repeat("é", 80)); d.HasError() {
		t.Errorf("80 code points of 2 bytes each must pass a bound of 80: %v", d)
	}
	d := validateString(t, spec, "name", strings.Repeat("é", 81))
	if !d.HasError() || !strings.Contains(d.Errors()[0].Detail(), "80") || !strings.Contains(d.Errors()[0].Detail(), "81") {
		t.Errorf("81 code points must fail naming the bound and the count: %v", d)
	}
	if d := validateString(t, spec, "name", ""); !d.HasError() {
		t.Error("an empty name must fail MinLength 1")
	}
	if d := validateString(t, spec, "name", "\U0001F600"); d.HasError() {
		t.Errorf("one code point of 4 bytes is one character: %v", d)
	}
}

func TestAListOneOfBoundsItsItems(t *testing.T) {
	spec := Spec{Key: "k", TypeName: "k", Shape: Generated, Lifecycle: true, Attrs: []Attr{{Name: "areas", Kind: StringList, OneOf: []string{"secrets", "iam"}, MinItems: 1}}}
	if d := validateList(t, spec, "areas", "secrets", "iam"); d.HasError() {
		t.Errorf("known items pass: %v", d)
	}
	d := validateList(t, spec, "areas", "secrets", "bogus")
	if !d.HasError() || !strings.Contains(d.Errors()[0].Detail(), "bogus") {
		t.Errorf("an unknown item must be refused naming it: %v", d)
	}
	d = validateList(t, spec, "areas")
	if !d.HasError() || !strings.Contains(strings.ToLower(d.Errors()[0].Summary()+d.Errors()[0].Detail()), "at least 1") {
		t.Errorf("an empty list must fail MinItems 1 naming the bound: %v", d)
	}
}

func TestADefaultIsPlannedWhenLeftOut(t *testing.T) {
	spec := Spec{Key: "rules", TypeName: "rule", Shape: Generated, Lifecycle: true, Attrs: []Attr{
		{Name: "name", Kind: String, Required: true},
		{Name: "enabled", Kind: Bool, Default: "true"},
		{Name: "min_severity", Kind: String, Default: `"high"`},
		{Name: "weight", Kind: Int, Default: "3"},
		{Name: "events", Kind: StringList, Default: `["new_finding"]`},
		{Name: "areas", Kind: StringList, Default: `[]`},
	}}
	_, sch := fixtureOf(t, spec)
	ctx := context.Background()
	for name, want := range map[string]attr.Value{
		"enabled":      types.BoolValue(true),
		"min_severity": types.StringValue("high"),
		"weight":       types.Int64Value(3),
		"events":       listOf("new_finding"),
		"areas":        listOf(),
	} {
		a := sch.Attributes[name]
		if !a.IsComputed() || !a.IsOptional() {
			t.Errorf("%s: a defaulted attribute is Optional and Computed", name)
		}
		var got attr.Value
		switch x := a.(type) {
		case rschema.BoolAttribute:
			resp := defaults.BoolResponse{}
			x.Default.DefaultBool(ctx, defaults.BoolRequest{}, &resp)
			got = resp.PlanValue
		case rschema.StringAttribute:
			resp := defaults.StringResponse{}
			x.Default.DefaultString(ctx, defaults.StringRequest{}, &resp)
			got = resp.PlanValue
		case rschema.Int64Attribute:
			resp := defaults.Int64Response{}
			x.Default.DefaultInt64(ctx, defaults.Int64Request{}, &resp)
			got = resp.PlanValue
		case rschema.ListAttribute:
			resp := defaults.ListResponse{}
			x.Default.DefaultList(ctx, defaults.ListRequest{}, &resp)
			got = resp.PlanValue
		}
		if got == nil || !got.Equal(want) {
			t.Errorf("%s: default %v, want %v", name, got, want)
		}
	}
}

// A defaulted attribute keeps no UseStateForUnknown: removing a stated value
// must plan the default, not the value state still holds.
func TestADefaultedAttributeHasNoStateCarryingModifier(t *testing.T) {
	spec := Spec{Key: "rules", TypeName: "rule", Shape: Generated, Lifecycle: true, Attrs: []Attr{{Name: "enabled", Kind: Bool, Default: "true"}}}
	if mods := attributeNamed(t, spec, "enabled").(rschema.BoolAttribute).PlanModifiers; len(mods) != 0 {
		t.Fatalf("modifiers on a defaulted attribute: %v", mods)
	}
}

func TestTheDataSourceCarriesNoValidators(t *testing.T) {
	d := &facadeDataSource{spec: slaSpec}
	var resp datasource.SchemaResponse
	d.Schema(context.Background(), datasource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	for name, a := range resp.Schema.Attributes {
		if sa, ok := a.(dschema.StringAttribute); ok && len(sa.Validators) > 0 {
			t.Errorf("%s carries validators on the data source", name)
		}
	}
}
