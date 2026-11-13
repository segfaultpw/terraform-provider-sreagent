package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func strList(values ...string) tftypes.Value {
	elems := make([]tftypes.Value, 0, len(values))
	for _, v := range values {
		elems = append(elems, tftypes.NewValue(tftypes.String, v))
	}
	return tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, elems)
}

var unknownBool = tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue)

// pagingSpec refuses paging on a rule that is not critical: the platform judges
// the merged row, so a value the configuration leaves to state must be judged
// from the plan.
func pagingSpec(seen *[]bool) Spec {
	return Spec{
		Key: "rules", TypeName: "rule", Shape: Generated, Lifecycle: true,
		Attrs: []Attr{
			{Name: "name", Kind: String, Required: true},
			{Name: "page", Kind: Bool},
			{Name: "min_severity", Kind: String},
		},
		PlanChecks: []func(plan, prior Config) []Issue{func(plan, prior Config) []Issue {
			*seen = append(*seen, prior != nil)
			page, pageSeen := plan.Bool("page")
			severity, severitySeen := plan.String("min_severity")
			if pageSeen == Present && page && severitySeen == Present && severity != "critical" {
				return []Issue{{Attr: "page", Summary: "Paging needs a critical rule", Detail: "Set min_severity to critical."}}
			}
			return nil
		}},
	}
}

func planOf(t *testing.T, r *facadeResource, s rschema.Schema, state, config, plan tftypes.Value) resource.ModifyPlanResponse {
	t.Helper()
	req := resource.ModifyPlanRequest{
		Config: tfsdk.Config{Schema: s, Raw: config},
		Plan:   tfsdk.Plan{Schema: s, Raw: plan},
		State:  tfsdk.State{Schema: s, Raw: state},
	}
	resp := resource.ModifyPlanResponse{Plan: req.Plan}
	r.ModifyPlan(context.Background(), req, &resp)
	return resp
}

func TestAPlanCheckSeesAValueCarriedFromState(t *testing.T) {
	var seen []bool
	r, s := fixtureOf(t, pagingSpec(&seen))
	row := func(page tftypes.Value, severity string) tftypes.Value {
		return rawObject(t, s, map[string]tftypes.Value{"id": text("r1"), "name": text("a"), "page": page, "min_severity": text(severity)})
	}
	// The configuration says nothing about page or min_severity; both are carried from state.
	config := rawObject(t, s, map[string]tftypes.Value{"name": text("a")})
	resp := planOf(t, r, s, row(boolTrue, "high"), config, row(boolTrue, "high"))
	if !resp.Diagnostics.HasError() {
		t.Fatal("a plan that keeps paging on a high rule must be refused")
	}
	d := resp.Diagnostics.Errors()[0]
	if !strings.Contains(d.Summary(), "Paging needs a critical rule") {
		t.Errorf("the platform's rule must be named: %v", d)
	}
	var onPage bool
	for _, e := range resp.Diagnostics.Errors() {
		if wa, ok := e.(diag.DiagnosticWithPath); ok && wa.Path().Equal(path.Root("page")) {
			onPage = true
		}
	}
	if !onPage {
		t.Errorf("the refusal must sit on the attribute: %v", resp.Diagnostics)
	}
	if len(seen) != 1 || !seen[0] {
		t.Errorf("an update must hand the check the prior state, got %v", seen)
	}

	if resp := planOf(t, r, s, row(boolTrue, "critical"), config, row(boolTrue, "critical")); resp.Diagnostics.HasError() {
		t.Errorf("a critical rule may page: %v", resp.Diagnostics)
	}

	// A create: no prior state, and a value that is not known yet is never judged.
	seen = nil
	create := rawObject(t, s, map[string]tftypes.Value{"name": text("a"), "page": unknownBool, "min_severity": text("high")})
	resp = planOf(t, r, s, tftypes.NewValue(s.Type().TerraformType(context.Background()), nil), config, create)
	if resp.Diagnostics.HasError() {
		t.Errorf("an unknown value is never refused: %v", resp.Diagnostics)
	}
	if len(seen) != 1 || seen[0] {
		t.Errorf("a create must hand the check no prior state, got %v", seen)
	}
	known := rawObject(t, s, map[string]tftypes.Value{"name": text("a"), "page": boolTrue, "min_severity": text("high")})
	if resp := planOf(t, r, s, tftypes.NewValue(s.Type().TerraformType(context.Background()), nil), config, known); !resp.Diagnostics.HasError() {
		t.Error("a create that pages below critical must be refused at plan")
	}
}

func TestAPlanCheckWarningDoesNotFailThePlan(t *testing.T) {
	spec := Spec{
		Key: "rules", TypeName: "rule", Shape: Generated, Lifecycle: true,
		Attrs: []Attr{{Name: "name", Kind: String, Required: true}},
		PlanChecks: []func(plan, prior Config) []Issue{func(Config, Config) []Issue {
			return []Issue{{Attr: "name", Summary: "Heads up", Detail: "This rule ended.", Warning: true}}
		}},
	}
	r, s := fixtureOf(t, spec)
	row := rawObject(t, s, map[string]tftypes.Value{"id": text("r1"), "name": text("a")})
	resp := planOf(t, r, s, row, row, row)
	if resp.Diagnostics.HasError() {
		t.Fatalf("a warning must not fail the plan: %v", resp.Diagnostics)
	}
	if resp.Diagnostics.WarningsCount() != 1 || !strings.Contains(resp.Diagnostics.Warnings()[0].Summary(), "Heads up") {
		t.Fatalf("the warning must be said: %v", resp.Diagnostics)
	}
}

func TestTheConfigReadersSayWhatTheyAreGiven(t *testing.T) {
	spec := Spec{Key: "k", TypeName: "k", Shape: Generated, Lifecycle: true, Attrs: []Attr{
		{Name: "on", Kind: Bool}, {Name: "n", Kind: Int}, {Name: "items", Kind: StringList}, {Name: "name", Kind: String},
	}}
	_, s := fixtureOf(t, spec)
	var diags diag.Diagnostics
	view := func(fields map[string]tftypes.Value) Config {
		return configView{ctx: context.Background(), src: tfsdk.Plan{Schema: s, Raw: rawObject(t, s, fields)}, attrs: spec.Attrs, diags: &diags}
	}
	known := view(map[string]tftypes.Value{"on": boolTrue, "n": some(7), "items": strList("a", "b")})
	if v, p := known.Bool("on"); !v || p != Present {
		t.Errorf("Bool = %v %v", v, p)
	}
	if v, p := known.Int("n"); v != 7 || p != Present {
		t.Errorf("Int = %v %v", v, p)
	}
	if v, p := known.Strings("items"); len(v) != 2 || v[1] != "b" || p != Present {
		t.Errorf("Strings = %v %v", v, p)
	}
	if v, p := view(map[string]tftypes.Value{"items": strList()}).Strings("items"); len(v) != 0 || p != Present {
		t.Errorf("an empty list is stated: %v %v", v, p)
	}
	absent := view(nil)
	if _, p := absent.Bool("on"); p != Absent {
		t.Error("a null boolean is Absent")
	}
	if _, p := absent.Int("n"); p != Absent {
		t.Error("a null integer is Absent")
	}
	if _, p := absent.Strings("items"); p != Absent {
		t.Error("a null list is Absent")
	}
	unknown := view(map[string]tftypes.Value{"on": unknownBool, "n": tftypes.NewValue(tftypes.Number, tftypes.UnknownValue), "items": tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, tftypes.UnknownValue)})
	if _, p := unknown.Bool("on"); p != Unknown {
		t.Error("an unknown boolean is Unknown")
	}
	if _, p := unknown.Int("n"); p != Unknown {
		t.Error("an unknown integer is Unknown")
	}
	if _, p := unknown.Strings("items"); p != Unknown {
		t.Error("an unknown list is Unknown")
	}
	partly := view(map[string]tftypes.Value{"items": tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{text("a"), unknownText})})
	if _, p := partly.Strings("items"); p != Unknown {
		t.Error("a list with an unknown entry is Unknown")
	}
	if diags.HasError() {
		t.Fatal(diags)
	}
	known.Bool("name")
	if !diags.HasError() {
		t.Error("asking for a string as a boolean is a provider bug and must say so")
	}
}

func modifyList(t *testing.T, spec Spec, name string, state, plan types.List) types.List {
	t.Helper()
	resp := planmodifier.ListResponse{PlanValue: plan}
	for _, m := range attributeNamed(t, spec, name).(rschema.ListAttribute).PlanModifiers {
		m.PlanModifyList(context.Background(), planmodifier.ListRequest{Path: path.Root(name), StateValue: state, PlanValue: plan, ConfigValue: plan}, &resp)
		plan = resp.PlanValue
	}
	return resp.PlanValue
}

func TestASetEqualListPlansNothing(t *testing.T) {
	spec := Spec{Key: "k", TypeName: "k", Shape: Singleton, Lifecycle: true, Attrs: []Attr{{Name: "classes", Kind: StringList, Unordered: true}}}
	state := listOf("other:a", "other:b")
	if got := modifyList(t, spec, "classes", state, listOf("other:b", "other:a")); !got.Equal(state) {
		t.Errorf("the same entries in another order must plan the state's list, got %v", got)
	}
	if got := modifyList(t, spec, "classes", state, listOf("other:a", "other:c")); !got.Equal(listOf("other:a", "other:c")) {
		t.Errorf("a different entry is a change, got %v", got)
	}
	if got := modifyList(t, spec, "classes", state, listOf("other:a")); !got.Equal(listOf("other:a")) {
		t.Errorf("a dropped entry is a change, got %v", got)
	}
	if got := modifyList(t, spec, "classes", types.ListNull(types.StringType), listOf("other:a")); !got.Equal(listOf("other:a")) {
		t.Errorf("with no state there is nothing to keep, got %v", got)
	}
	ordered := Spec{Key: "k", TypeName: "k", Shape: Singleton, Lifecycle: true, Attrs: []Attr{{Name: "events", Kind: StringList}}}
	if got := modifyList(t, ordered, "events", state, listOf("other:b", "other:a")); !got.Equal(listOf("other:b", "other:a")) {
		t.Errorf("a list the platform keeps in order is a change when reordered, got %v", got)
	}

	a := Attr{Kind: StringList, Unordered: true}
	if !keep(a, listOf("b", "a"), listOf("a", "b")) {
		t.Error("a refresh must keep the order the configuration wrote")
	}
	if keep(Attr{Kind: StringList}, listOf("b", "a"), listOf("a", "b")) {
		t.Error("an ordered list keeps the answer")
	}
	if keep(a, listOf("b", "a"), listOf("a", "c")) {
		t.Error("different entries are not the same set")
	}
}

func TestRFC3339NormalizesToTheSameInstant(t *testing.T) {
	forms := []string{"2026-12-31T00:00:00Z", "2026-12-31T00:00:00.000000Z", "2026-12-31T01:00:00+01:00"}
	for _, f := range forms {
		if got := RFC3339(f); got != "2026-12-31T00:00:00Z" {
			t.Errorf("%s normalized to %s", f, got)
		}
	}
	if got := RFC3339("not a time"); got != "not a time" {
		t.Errorf("a value that is not a time is returned as it is, got %q", got)
	}
	a := Attr{Name: "expires_at", Kind: String, Timestamp: true}
	if !keep(a, types.StringValue(forms[2]), types.StringValue(forms[1])) {
		t.Error("equal instants are equal whatever their spelling")
	}
	if keep(a, types.StringValue(forms[0]), types.StringValue("2027-01-01T00:00:00.000000Z")) {
		t.Error("another instant is a change")
	}

	spec := Spec{Key: "k", TypeName: "k", Shape: Generated, Lifecycle: true, Attrs: []Attr{{Name: "expires_at", Kind: String, Timestamp: true, CreateOnly: true, Clearable: true}}}
	attr := attributeNamed(t, spec, "expires_at").(rschema.StringAttribute)
	for _, c := range []struct {
		value string
		bad   bool
	}{{"2027-01-01T00:00:00Z", false}, {"2027-01-01T00:00:00.5+02:00", false}, {"2027-01-01T00:00:00", true}, {"2027-01-01", true}, {"tomorrow", true}} {
		var all diag.Diagnostics
		for _, v := range attr.Validators {
			resp := validator.StringResponse{}
			v.ValidateString(context.Background(), validator.StringRequest{Path: path.Root("expires_at"), ConfigValue: types.StringValue(c.value)}, &resp)
			all.Append(resp.Diagnostics...)
		}
		if all.HasError() != c.bad {
			t.Errorf("%q: errors %v, want error %v", c.value, all, c.bad)
		}
		if c.bad && !strings.Contains(all.Errors()[0].Detail(), "time zone") {
			t.Errorf("%q: the refusal must name the zone: %v", c.value, all)
		}
	}
	// The same instant in another spelling plans the state's value, so it neither
	// shows a change nor replaces a create-only row.
	state := types.StringValue("2027-01-01T00:00:00Z")
	plan := types.StringValue("2027-01-01T01:00:00+01:00")
	resp := planmodifier.StringResponse{PlanValue: plan}
	for _, m := range attr.PlanModifiers {
		m.PlanModifyString(context.Background(), planmodifier.StringRequest{Path: path.Root("expires_at"), StateValue: state, PlanValue: resp.PlanValue, ConfigValue: plan}, &resp)
	}
	if !resp.PlanValue.Equal(state) || resp.RequiresReplace {
		t.Errorf("planned %v, replace %v", resp.PlanValue, resp.RequiresReplace)
	}
}

func TestATrimmedStringRefusesSurroundingSpace(t *testing.T) {
	spec := Spec{Key: "k", TypeName: "k", Shape: Generated, Lifecycle: true, Attrs: []Attr{{Name: "name", Kind: String, Required: true, Trim: true}}}
	if d := validateString(t, spec, "name", "ok"); d.HasError() {
		t.Errorf("a trimmed name passes: %v", d)
	}
	d := validateString(t, spec, "name", " padded")
	if !d.HasError() || !strings.Contains(d.Errors()[0].Detail(), "whitespace") {
		t.Errorf("surrounding space must be refused: %v", d)
	}
	loose := Spec{Key: "k", TypeName: "k", Shape: Generated, Lifecycle: true, Attrs: []Attr{{Name: "name", Kind: String, Required: true}}}
	if d := validateString(t, loose, "name", " padded"); d.HasError() {
		t.Errorf("an attribute that does not ask for it is not trimmed: %v", d)
	}
}

func TestAChangeAndARemovedEntryAreWarnedAtPlan(t *testing.T) {
	spec := Spec{
		Key: "k", TypeName: "k", Shape: Singleton, Lifecycle: true,
		Attrs: []Attr{
			{Name: "days", Kind: Int, ChangeWarning: "Changing the SLA moves due dates."},
			{Name: "classes", Kind: StringList, Unordered: true, RemovedWarning: "Unmarking %s closes its findings."},
		},
	}
	r, s := fixtureOf(t, spec)
	row := func(days tftypes.Value, classes tftypes.Value) tftypes.Value {
		return rawObject(t, s, map[string]tftypes.Value{"id": text("k"), "days": days, "classes": classes})
	}
	warnings := func(state, plan tftypes.Value) []string {
		var out []string
		resp := planOf(t, r, s, state, plan, plan)
		if resp.Diagnostics.HasError() {
			t.Fatal(resp.Diagnostics)
		}
		for _, w := range resp.Diagnostics.Warnings() {
			out = append(out, w.Detail())
		}
		return out
	}
	if got := warnings(row(some(7), strList("a", "b")), row(some(7), strList("a", "b"))); len(got) != 0 {
		t.Errorf("no change, no warning: %v", got)
	}
	got := warnings(row(some(7), strList("a", "b")), row(some(30), strList("a", "b")))
	if len(got) != 1 || got[0] != "Changing the SLA moves due dates." {
		t.Errorf("a changed value warns: %v", got)
	}
	got = warnings(row(some(7), strList("a", "b", "c")), row(some(7), strList("a")))
	if len(got) != 2 || got[0] != "Unmarking b closes its findings." || got[1] != "Unmarking c closes its findings." {
		t.Errorf("each dropped entry warns once, in state's order: %v", got)
	}
	if got := warnings(row(some(7), strList("a", "b")), row(some(7), strList("b", "a", "c"))); len(got) != 0 {
		t.Errorf("an added entry drops nothing: %v", got)
	}
	// A create has no state to compare with.
	create := planOf(t, r, s, tftypes.NewValue(s.Type().TerraformType(context.Background()), nil), row(some(30), strList("a")), row(some(30), strList("a")))
	if create.Diagnostics.WarningsCount() != 0 {
		t.Errorf("a create has nothing to warn about: %v", create.Diagnostics)
	}
}
