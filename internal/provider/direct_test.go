package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/client"
	"github.com/segfaultpw/terraform-provider-sreagent/internal/engine"
	"github.com/segfaultpw/terraform-provider-sreagent/internal/fakefacade"
)

// direct drives one resource's plan and destroy without the Terraform CLI, for
// what the CLI hides from a test: a warning, which the plugin testing framework
// never shows, and the plan the resource makes for a prior state it is handed.
type direct struct {
	t      *testing.T
	res    resource.Resource
	schema resource.SchemaResponse
	typ    tftypes.Object
}

func newDirect(t *testing.T, spec engine.Spec, f *fakefacade.Fake) *direct {
	t.Helper()
	res := engine.NewResource(spec)()
	d := &direct{t: t, res: res}
	res.Schema(context.Background(), resource.SchemaRequest{}, &d.schema)
	d.typ = d.schema.Schema.Type().TerraformType(context.Background()).(tftypes.Object)
	c, err := client.New(client.Config{BaseURL: f.URL, APIKey: "sre_ak_unit"}) //nolint:gosec // the fake facade accepts any sre_ak_ key
	if err != nil {
		t.Fatal(err)
	}
	var cfg resource.ConfigureResponse
	res.(resource.ResourceWithConfigure).Configure(context.Background(), resource.ConfigureRequest{ProviderData: c}, &cfg)
	if cfg.Diagnostics.HasError() {
		t.Fatal(cfg.Diagnostics)
	}
	return d
}

// value is an object of the resource's schema: the fields named, null for the rest.
func (d *direct) value(fields map[string]tftypes.Value) tftypes.Value {
	all := map[string]tftypes.Value{}
	for name, typ := range d.typ.AttributeTypes {
		all[name] = tftypes.NewValue(typ, nil)
	}
	for name, v := range fields {
		if _, ok := d.typ.AttributeTypes[name]; !ok {
			d.t.Fatalf("the schema has no attribute %s", name)
		}
		all[name] = v
	}
	return tftypes.NewValue(d.typ, all)
}

// plan runs the resource's ModifyPlan over a prior state and the plan Terraform
// would propose.
func (d *direct) plan(state, plan tftypes.Value) resource.ModifyPlanResponse {
	d.t.Helper()
	req := resource.ModifyPlanRequest{
		Config: tfsdk.Config{Schema: d.schema.Schema, Raw: plan},
		Plan:   tfsdk.Plan{Schema: d.schema.Schema, Raw: plan},
		State:  tfsdk.State{Schema: d.schema.Schema, Raw: state},
	}
	resp := resource.ModifyPlanResponse{Plan: req.Plan}
	d.res.(resource.ResourceWithModifyPlan).ModifyPlan(context.Background(), req, &resp)
	return resp
}

func (d *direct) destroy(state tftypes.Value) resource.DeleteResponse {
	d.t.Helper()
	resp := resource.DeleteResponse{State: tfsdk.State{Schema: d.schema.Schema, Raw: state}}
	d.res.Delete(context.Background(), resource.DeleteRequest{State: tfsdk.State{Schema: d.schema.Schema, Raw: state}}, &resp)
	return resp
}

// warned reports a warning whose summary and detail together hold every part.
func warned(diags diag.Diagnostics, parts ...string) bool {
	for _, w := range diags.Warnings() {
		text := w.Summary() + " " + w.Detail()
		ok := true
		for _, p := range parts {
			ok = ok && strings.Contains(text, p)
		}
		if ok {
			return true
		}
	}
	return false
}

func tfNum(n int64) tftypes.Value { return tftypes.NewValue(tftypes.Number, n) }
func tfBool(b bool) tftypes.Value { return tftypes.NewValue(tftypes.Bool, b) }
