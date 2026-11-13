package engine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/client"
)

func TestARowSourceReadsAnAnsweredRowAsAStateWould(t *testing.T) {
	src := rowSource{
		attrs: []Attr{
			{Name: "days", Kind: Int, NullAs: "0"},
			{Name: "channel", Kind: String, NullAs: `""`},
			{Name: "on", Kind: Bool},
			{Name: "classes", Kind: StringList},
			{Name: "available", Kind: JSON, Computed: true},
		},
		row: map[string]any{"days": nil, "channel": nil, "on": true, "classes": []any{"a", "b"}, "available": []any{"x"}},
	}
	ctx := context.Background()
	var days types.Int64
	var channel types.String
	var on types.Bool
	var classes types.List
	var available jsontypes.Normalized
	for _, d := range []interface{ HasError() bool }{
		src.GetAttribute(ctx, path.Root("days"), &days),
		src.GetAttribute(ctx, path.Root("channel"), &channel),
		src.GetAttribute(ctx, path.Root("on"), &on),
		src.GetAttribute(ctx, path.Root("classes"), &classes),
		src.GetAttribute(ctx, path.Root("available"), &available),
	} {
		if d.HasError() {
			t.Fatal("a row source must read every kind")
		}
	}
	if days.ValueInt64() != 0 || days.IsNull() || channel.ValueString() != "" || channel.IsNull() {
		t.Errorf("a null answer reads as its NullAs, got %v %v", days, channel)
	}
	if !on.ValueBool() || strings.Join(stringsOf(classes), ",") != "a,b" || available.ValueString() != `["x"]` {
		t.Errorf("got %v %v %v", on, classes, available)
	}
	var unknown types.String
	if d := src.GetAttribute(ctx, path.Root("nothing"), &unknown); d.HasError() || !unknown.IsNull() {
		t.Errorf("an attribute the row does not describe reads as null, got %v %v", unknown, d)
	}
}

// A platform that predates a singleton every organization has answers 404: the create and the
// refresh name the release, and the refresh never reads it as a deleted row.
func TestA404ForAnAlwaysPresentSingletonNamesTheRelease(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not_found","message":"No such configuration resource: tiny."}`))
	}))
	defer srv.Close()
	c, err := client.New(client.Config{BaseURL: srv.URL, APIKey: "sre_ak_unit"}) //nolint:gosec // a test server accepts any sre_ak_ key
	if err != nil {
		t.Fatal(err)
	}
	spec := Spec{Key: "tiny", TypeName: "tiny", Shape: Singleton, Lifecycle: true, AlwaysPresent: true, MinPlatform: "v9.9.9 or later",
		Attrs: []Attr{{Name: "days", Kind: Int, Description: "Days."}}}
	r := &facadeResource{spec: spec, client: c}
	var sch resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &sch)
	typ := sch.Schema.Type().TerraformType(context.Background()).(tftypes.Object)
	obj := func(fields map[string]tftypes.Value) tftypes.Value {
		all := map[string]tftypes.Value{}
		for name, ty := range typ.AttributeTypes {
			all[name] = tftypes.NewValue(ty, nil)
		}
		for name, v := range fields {
			all[name] = v
		}
		return tftypes.NewValue(typ, all)
	}

	plan := tfsdk.Plan{Schema: sch.Schema, Raw: obj(map[string]tftypes.Value{"days": tftypes.NewValue(tftypes.Number, 3)})}
	created := resource.CreateResponse{State: tfsdk.State{Schema: sch.Schema}}
	r.Create(context.Background(), resource.CreateRequest{Plan: plan, Config: tfsdk.Config{Schema: sch.Schema, Raw: plan.Raw}}, &created)
	if !created.Diagnostics.HasError() || !strings.Contains(created.Diagnostics.Errors()[0].Detail(), "sreagent_tiny needs platform v9.9.9 or later") {
		t.Errorf("a create over a platform that predates the resource must name the release: %v", created.Diagnostics)
	}

	state := tfsdk.State{Schema: sch.Schema, Raw: obj(map[string]tftypes.Value{"id": tftypes.NewValue(tftypes.String, "tiny"), "days": tftypes.NewValue(tftypes.Number, 3)})}
	read := resource.ReadResponse{State: state}
	r.Read(context.Background(), resource.ReadRequest{State: state}, &read)
	if !read.Diagnostics.HasError() || !strings.Contains(read.Diagnostics.Errors()[0].Detail(), "needs platform v9.9.9 or later") {
		t.Errorf("a refresh that meets the 404 must name the release: %v", read.Diagnostics)
	}
	if read.State.Raw.IsNull() {
		t.Error("the 404 must not remove the resource from state: the next plan would propose a create")
	}

	// A singleton that is not always there keeps reading a 404 as a deleted row.
	other := &facadeResource{spec: Spec{Key: "tiny", TypeName: "tiny", Shape: Singleton, Lifecycle: true, Attrs: spec.Attrs}, client: c}
	read = resource.ReadResponse{State: state}
	other.Read(context.Background(), resource.ReadRequest{State: state}, &read)
	if read.Diagnostics.HasError() || !read.State.Raw.IsNull() {
		t.Errorf("a released singleton keeps its behaviour: %v", read.Diagnostics)
	}
}
