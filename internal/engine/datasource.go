package engine

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/client"
)

type facadeDataSource struct {
	spec   Spec
	list   bool
	client *client.Client
}

var (
	_ datasource.DataSource              = (*facadeDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*facadeDataSource)(nil)
)

func (d *facadeDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	if d.list {
		resp.TypeName = req.ProviderTypeName + "_" + d.spec.ListName
		return
	}
	resp.TypeName = req.ProviderTypeName + "_" + d.spec.TypeName
}

func (d *facadeDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("got %T", req.ProviderData))
		return
	}
	d.client = c
}

func (d *facadeDataSource) attrs() []Attr {
	if d.list && d.spec.ListAttrs != nil {
		return d.spec.ListAttrs
	}
	return d.spec.Attrs
}

func kindType(k Kind) attr.Type {
	switch k {
	case Int:
		return types.Int64Type
	case Float:
		return types.Float64Type
	case Bool:
		return types.BoolType
	case StringList:
		return types.ListType{ElemType: types.StringType}
	case JSON:
		return jsontypes.NormalizedType{}
	}
	return types.StringType
}

func computedAttribute(a Attr) schema.Attribute {
	switch a.Kind {
	case Int:
		return schema.Int64Attribute{Computed: true, Description: a.Description}
	case Float:
		return schema.Float64Attribute{Computed: true, Description: a.Description}
	case Bool:
		return schema.BoolAttribute{Computed: true, Description: a.Description}
	case StringList:
		return schema.ListAttribute{Computed: true, ElementType: types.StringType, Description: a.Description}
	case JSON:
		return schema.StringAttribute{Computed: true, CustomType: jsontypes.NormalizedType{}, Description: a.Description}
	}
	return schema.StringAttribute{Computed: true, Description: a.Description}
}

// readable is every attribute a read answers, keyed by its Terraform name,
// with Name set to the answer's own key. A secret is answered only as
// <name>_set, never the secret; a NotRead argument is not answered at all.
func readable(attrs []Attr) map[string]Attr {
	out := map[string]Attr{}
	for _, a := range attrs {
		switch {
		case a.NotRead:
		case a.Secret:
			out[a.Attribute()+"_set"] = Attr{Name: a.Name + "_set", Kind: Bool, Description: fmt.Sprintf("Whether the platform holds a value for %s.", a.Attribute())}
		default:
			out[a.Attribute()] = Attr{Name: a.Name, Kind: a.Kind, Description: a.Description}
		}
	}
	return out
}

func (d *facadeDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	fields := map[string]schema.Attribute{"id": schema.StringAttribute{Computed: true, Description: "The row's identifier on the platform."}}
	for name, a := range readable(d.attrs()) {
		fields[name] = computedAttribute(a)
	}
	if !d.list {
		if _, ok := fields["configured"]; !ok {
			fields["configured"] = schema.BoolAttribute{Computed: true, Description: "False when the organization has no row of its own yet (or reads its parent's); every other attribute is then null."}
		}
		resp.Schema = schema.Schema{Description: d.spec.Description, Attributes: fields}
		return
	}
	resp.Schema = schema.Schema{
		Description: "Every " + d.spec.TypeName + " row, read-only. " + d.spec.Description,
		Attributes: map[string]schema.Attribute{
			"id":        schema.StringAttribute{Computed: true, Description: "The resource key the list was read from."},
			"truncated": schema.BoolAttribute{Computed: true, Description: "True when the API returned as many rows as its cap and may hold more."},
			"items":     schema.ListNestedAttribute{Computed: true, Description: "The rows.", NestedObject: schema.NestedAttributeObject{Attributes: fields}},
		},
	}
}

func (d *facadeDataSource) values(row map[string]any) (map[string]attr.Value, map[string]attr.Type, error) {
	vals := map[string]attr.Value{"id": types.StringValue(RowID(d.spec, row))}
	typs := map[string]attr.Type{"id": types.StringType}
	for name, a := range readable(d.attrs()) {
		v, err := fromJSON(a.Kind, row[a.Name])
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", name, err)
		}
		vals[name] = v
		typs[name] = kindType(a.Kind)
	}
	return vals, typs, nil
}

func (d *facadeDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	name := "sreagent_" + d.spec.TypeName
	if d.list {
		name = "sreagent_" + d.spec.ListName
	}
	out, err := d.client.Do(ctx, client.Request{Method: http.MethodGet, Path: d.spec.Key})
	if !d.list && client.IsNotFound(err) {
		// No row of its own (Slack not connected, or settings inherited from a
		// parent): a fact to branch on in HCL, not an error.
		vals, _, _ := d.values(map[string]any{})
		vals["configured"] = types.BoolValue(false)
		for n, v := range vals {
			resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(n), v)...)
		}
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not read "+name, err.Error())
		return
	}
	if !d.list {
		row, err := decodeRow(out.Data)
		if err != nil {
			resp.Diagnostics.AddError("Unexpected API answer", err.Error())
			return
		}
		vals, _, err := d.values(row)
		if err != nil {
			resp.Diagnostics.AddError("Unexpected API answer", err.Error())
			return
		}
		if _, answered := row["configured"]; !answered {
			vals["configured"] = types.BoolValue(true)
		}
		for n, v := range vals {
			resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(n), v)...)
		}
		return
	}
	rows, err := decodeRows(out.Data)
	if err != nil {
		resp.Diagnostics.AddError("Unexpected API answer", err.Error())
		return
	}
	_, typs, _ := d.values(map[string]any{})
	objs := make([]attr.Value, 0, len(rows))
	for _, row := range rows {
		vals, _, err := d.values(row)
		if err != nil {
			resp.Diagnostics.AddError("Unexpected API answer", err.Error())
			return
		}
		obj, diags := types.ObjectValue(typs, vals)
		resp.Diagnostics.Append(diags...)
		objs = append(objs, obj)
	}
	list, diags := types.ListValue(types.ObjectType{AttrTypes: typs}, objs)
	resp.Diagnostics.Append(diags...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), types.StringValue(d.spec.Key))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("truncated"), types.BoolValue(out.Truncated))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("items"), list)...)
}
