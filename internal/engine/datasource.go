package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

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
		case a.NotRead, a.WriteOnly, a.Local, a.Kept, len(a.Complement) > 0:
		case a.Secret:
			out[a.Attribute()+"_set"] = Attr{Name: a.Name + "_set", Kind: Bool, Description: fmt.Sprintf("Whether the platform holds a value for %s.", a.Attribute())}
		default:
			desc := a.Description
			if a.ReadDescription != "" {
				desc = a.ReadDescription
			}
			out[a.Attribute()] = Attr{Name: a.Name, Kind: a.Kind, Description: desc, IgnoredValues: a.IgnoredValues}
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
			fields["configured"] = schema.BoolAttribute{Computed: true, Description: ConfiguredDescription(d.spec)}
		}
		resp.Schema = schema.Schema{Description: d.description(), Attributes: fields}
		return
	}
	truncated := "True when the API returned as many rows as its cap and may hold more."
	if d.spec.Cursor != "" {
		truncated = "Always false: every page was read, or the read failed."
	}
	attrs := map[string]schema.Attribute{
		"id":        schema.StringAttribute{Computed: true, Description: "The resource key the list was read from."},
		"truncated": schema.BoolAttribute{Computed: true, Description: truncated},
		"items":     schema.ListNestedAttribute{Computed: true, Description: "The rows.", NestedObject: schema.NestedAttributeObject{Attributes: fields}},
	}
	for _, q := range d.spec.QueryAttrs {
		switch q.Kind {
		case Bool:
			attrs[q.Attribute()] = schema.BoolAttribute{Optional: true, Description: q.Description}
		default:
			attrs[q.Attribute()] = schema.StringAttribute{Optional: true, Description: q.Description}
		}
	}
	for _, x := range d.spec.ExtraAttrs {
		attrs[x.Attribute()] = computedAttribute(x)
	}
	resp.Schema = schema.Schema{Description: "Every " + d.spec.TypeName + " row, read-only. " + d.description(), Attributes: attrs}
}

// description is what the data source says of the resource.
func (d *facadeDataSource) description() string {
	if d.spec.ReadDescription != "" {
		return d.spec.ReadDescription
	}
	return d.spec.Description
}

// query is the query string a list read sends: the arguments the configuration
// states, and the page size of a paged list.
func (d *facadeDataSource) query(ctx context.Context, cfg valueSource) (url.Values, error) {
	q := url.Values{}
	for _, a := range d.spec.QueryAttrs {
		v, diags := getValue(ctx, cfg, a)
		if diags.HasError() {
			return nil, fmt.Errorf("reading %s: %s", a.Attribute(), diags.Errors()[0].Detail())
		}
		if v == nil || v.IsNull() || v.IsUnknown() {
			continue
		}
		switch t := v.(type) {
		case types.String:
			q.Set(a.Name, t.ValueString())
		case types.Bool:
			q.Set(a.Name, strconv.FormatBool(t.ValueBool()))
		}
	}
	if d.spec.Cursor != "" && d.spec.PageSize > 0 {
		q.Set("limit", strconv.Itoa(d.spec.PageSize))
	}
	return q, nil
}

// readPages reads a list: one request for a plain list, every page for a paged
// one. It answers the rows, the keys beside them in the first page and whether
// the API says it cut the list.
func (d *facadeDataSource) readPages(ctx context.Context, cfg valueSource) ([]map[string]any, map[string]json.RawMessage, bool, error) {
	q, err := d.query(ctx, cfg)
	if err != nil {
		return nil, nil, false, err
	}
	if d.spec.Cursor == "" {
		out, err := d.client.Do(ctx, client.Request{Method: http.MethodGet, Path: d.spec.Key, Query: q})
		if err != nil {
			return nil, nil, false, err
		}
		rows, err := decodeRows(out.Data)
		return rows, out.Extras, out.Truncated, err
	}
	maxPages := d.spec.MaxPages
	if maxPages == 0 {
		maxPages = 200
	}
	var all []map[string]any
	var first map[string]json.RawMessage
	for page := 1; ; page++ {
		out, err := d.client.Do(ctx, client.Request{Method: http.MethodGet, Path: d.spec.Key, Query: q})
		if err != nil {
			return nil, nil, false, err
		}
		rows, err := decodeRows(out.Data)
		if err != nil {
			return nil, nil, false, err
		}
		all = append(all, rows...)
		if first == nil {
			first = out.Extras
		}
		var next *string
		if raw, ok := out.Extras[d.spec.Cursor]; ok {
			if err := json.Unmarshal(raw, &next); err != nil {
				return nil, nil, false, fmt.Errorf("the API answered a %s that is not text or null: %w", d.spec.Cursor, err)
			}
		}
		if next == nil || *next == "" {
			return all, first, false, nil
		}
		if page >= maxPages {
			return nil, nil, false, fmt.Errorf("the list has more than %d pages of %d rows (%d rows read); narrow it with the query arguments instead of reading it whole", maxPages, d.spec.PageSize, len(all))
		}
		q.Set("cursor", *next)
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
		if len(a.IgnoredValues) > 0 {
			v = withoutValues(v, a.IgnoredValues)
		}
		vals[name] = v
		typs[name] = kindType(a.Kind)
	}
	return vals, typs, nil
}

func (d *facadeDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.list {
		d.readList(ctx, req, resp)
		return
	}
	name := "sreagent_" + d.spec.TypeName
	out, err := d.client.Do(ctx, client.Request{Method: http.MethodGet, Path: d.spec.Key})
	if d.spec.AlwaysPresent && client.IsNotFound(err) {
		resp.Diagnostics.AddError("The platform does not serve "+d.spec.Key, notServedDetail(d.spec, name, err))
		return
	}
	if client.IsNotFound(err) {
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
}

func (d *facadeDataSource) readList(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	name := "sreagent_" + d.spec.ListName
	rows, extras, truncated, err := d.readPages(ctx, req.Config)
	if err != nil {
		resp.Diagnostics.AddError("Could not read "+name, err.Error())
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
	for _, x := range d.spec.ExtraAttrs {
		var raw any
		if blob, ok := extras[x.Name]; ok {
			dec := json.NewDecoder(bytes.NewReader(blob))
			dec.UseNumber()
			if err := dec.Decode(&raw); err != nil {
				resp.Diagnostics.AddError("Unexpected API answer", fmt.Sprintf("%s: %s", x.Name, err))
				return
			}
		}
		v, err := fromJSON(x.Kind, raw)
		if err != nil {
			resp.Diagnostics.AddError("Unexpected API answer", fmt.Sprintf("%s: %s", x.Name, err))
			return
		}
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(x.Attribute()), v)...)
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), types.StringValue(d.spec.Key))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("truncated"), types.BoolValue(truncated))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("items"), list)...)
}

// ConfiguredDescription is the configured attribute of a singleton the platform
// holds for every organization it offers the resource to: true whenever the read
// succeeds, an error otherwise, and the description says which errors.
func ConfiguredDescription(spec Spec) string {
	if !spec.AlwaysPresent {
		return "False when the organization has no row of its own yet (or reads its parent's); every other attribute is then null."
	}
	if spec.AbsentWhen == "" && spec.RefusedWhen == "" {
		return "Always true: every organization has this resource. A platform that does not serve it fails the read with an error instead."
	}
	d := "Always true when the read succeeds; the read never answers false with null attributes. It fails with an error instead when the platform predates the resource"
	if spec.AbsentWhen != "" {
		d += ", when " + spec.AbsentWhen
	}
	if spec.RefusedWhen != "" {
		d += ", and when " + spec.RefusedWhen
	}
	return d + "."
}

// notServedDetail is the error of a 404 on an AlwaysPresent singleton. A resource every
// organization has can only be missing because the platform is older; one the platform
// offers conditionally names that condition too, because the platform's own sentence
// for it ("Unknown tool") does not say which cause it is.
func notServedDetail(spec Spec, name string, err error) string {
	said := "\n\nThe platform said: " + err.Error()
	if spec.AbsentWhen == "" {
		return "Every organization has " + spec.Key + ", so the platform answered 404 because it predates the resource. " +
			"Upgrade the platform to " + spec.MinPlatform + " before reading " + name + "." + said
	}
	return "The platform answered 404 for " + spec.Key + ". Either the platform predates the resource " +
		"(upgrade it to " + spec.MinPlatform + " before reading " + name + "), or this organization is not offered it: " +
		spec.AbsentWhen + ". Not every organization has " + spec.Key + "." + said
}
