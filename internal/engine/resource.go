package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/float64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/client"
)

var (
	_ resource.Resource                = (*facadeResource)(nil)
	_ resource.ResourceWithConfigure   = (*facadeResource)(nil)
	_ resource.ResourceWithImportState = (*facadeResource)(nil)
	_ resource.ResourceWithIdentity    = (*facadeResource)(nil)
)

type facadeResource struct {
	spec   Spec
	client *client.Client
}

// privateData is what the framework's private state accessors share.
type privateData interface {
	GetKey(ctx context.Context, key string) ([]byte, diag.Diagnostics)
	SetKey(ctx context.Context, key string, value []byte) diag.Diagnostics
}

type op int

const (
	opCreate op = iota
	opUpdate
)

// NewResource returns a factory for spec.
func NewResource(spec Spec) func() resource.Resource {
	return func() resource.Resource { return &facadeResource{spec: spec} }
}

func (r *facadeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + r.spec.TypeName
}

func (r *facadeResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("got %T", req.ProviderData))
		return
	}
	r.client = c
}

func (r *facadeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := map[string]schema.Attribute{
		"id": schema.StringAttribute{Computed: true, Description: "The row's identifier on the platform.", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
	}
	for _, a := range r.spec.Attrs {
		for name, sa := range resourceAttributes(a, r.spec.Shape == Singleton) {
			attrs[name] = sa
		}
	}
	resp.Schema = schema.Schema{Description: r.spec.Description, Attributes: attrs}
}

func (r *facadeResource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	if r.spec.Shape == ServiceBinding {
		resp.IdentitySchema = identityschema.Schema{Attributes: map[string]identityschema.Attribute{
			"service":     identityschema.StringAttribute{RequiredForImport: true},
			"environment": identityschema.StringAttribute{OptionalForImport: true},
		}}
		return
	}
	resp.IdentitySchema = identityschema.Schema{Attributes: map[string]identityschema.Attribute{
		"id": identityschema.StringAttribute{RequiredForImport: true},
	}}
}

func resourceAttributes(a Attr, singleton bool) map[string]schema.Attribute {
	if a.Secret {
		n := a.Attribute()
		version := schema.Int64Attribute{Optional: true, Description: fmt.Sprintf("Change this number to send %s_wo again. Set it whenever %s_wo is set.", n, n)}
		if a.CreateOnly {
			version.PlanModifiers = []planmodifier.Int64{int64planmodifier.RequiresReplace()}
		}
		return map[string]schema.Attribute{
			// Optional even when the API requires the secret on create: an imported
			// row already holds it, and a required argument would force every import
			// to restate a value nobody can read back. Create checks it instead.
			n + "_wo":         schema.StringAttribute{Optional: true, Sensitive: true, WriteOnly: true, Description: a.Description + " Write-only: never stored in state or plan."},
			n + "_wo_version": version,
			n + "_set":        schema.BoolAttribute{Computed: true, Description: fmt.Sprintf("Whether the platform holds a value for %s.", n)},
		}
	}
	required := a.Required && !a.Computed
	optional := !required && !a.Computed
	clearable := a.Clearable && !singleton
	// A NotRead argument is never answered, so the platform can never fill it:
	// left out of the configuration it stays null rather than unknown.
	computed := a.Computed || (!required && !clearable && !a.NotRead)
	stable := computed && !a.Computed
	switch a.Kind {
	case String, JSON:
		mods := []planmodifier.String{}
		if stable {
			mods = append(mods, stringplanmodifier.UseStateForUnknown())
		}
		if a.CreateOnly || a.NotRead {
			mods = append(mods, replaceString(a))
		}
		sa := schema.StringAttribute{Description: describe(a), Required: required, Optional: optional, Computed: computed, PlanModifiers: mods}
		if a.Kind == JSON {
			sa.CustomType = jsontypes.NormalizedType{}
		}
		if len(a.OneOf) > 0 {
			sa.Validators = append(sa.Validators, stringvalidator.OneOf(a.OneOf...))
		}
		if len(a.HiddenKeys) > 0 {
			sa.Validators = append(sa.Validators, refuseKeys{keys: a.HiddenKeys})
		}
		return map[string]schema.Attribute{a.Attribute(): sa}
	case Int:
		mods := []planmodifier.Int64{}
		if stable {
			mods = append(mods, int64planmodifier.UseStateForUnknown())
		}
		if a.CreateOnly || a.NotRead {
			mods = append(mods, int64planmodifier.RequiresReplace())
		}
		return map[string]schema.Attribute{a.Attribute(): schema.Int64Attribute{Description: a.Description, Required: required, Optional: optional, Computed: computed, PlanModifiers: mods}}
	case Float:
		mods := []planmodifier.Float64{}
		if stable {
			mods = append(mods, float64planmodifier.UseStateForUnknown())
		}
		if a.CreateOnly || a.NotRead {
			mods = append(mods, float64planmodifier.RequiresReplace())
		}
		return map[string]schema.Attribute{a.Attribute(): schema.Float64Attribute{Description: a.Description, Required: required, Optional: optional, Computed: computed, PlanModifiers: mods}}
	case Bool:
		mods := []planmodifier.Bool{}
		if stable {
			mods = append(mods, boolplanmodifier.UseStateForUnknown())
		}
		if a.CreateOnly || a.NotRead {
			mods = append(mods, boolplanmodifier.RequiresReplace())
		}
		return map[string]schema.Attribute{a.Attribute(): schema.BoolAttribute{Description: a.Description, Required: required, Optional: optional, Computed: computed, PlanModifiers: mods}}
	case StringList:
		mods := []planmodifier.List{}
		if stable {
			mods = append(mods, listplanmodifier.UseStateForUnknown())
		}
		if a.CreateOnly || a.NotRead {
			mods = append(mods, listplanmodifier.RequiresReplace())
		}
		return map[string]schema.Attribute{a.Attribute(): schema.ListAttribute{ElementType: types.StringType, Description: a.Description, Required: required, Optional: optional, Computed: computed, PlanModifiers: mods}}
	}
	return nil
}

// replaceString replaces the row when a CreateOnly string changes. For a
// value the platform normalizes, only a change that survives normalization
// counts: an import reads the stored "checkout", and a configured "Checkout"
// is the same service, not a new one.
func replaceString(a Attr) planmodifier.String {
	if a.Normalize == nil {
		return stringplanmodifier.RequiresReplace()
	}
	desc := "Changing it replaces the row; a change of case or surrounding space does not."
	return stringplanmodifier.RequiresReplaceIf(func(_ context.Context, req planmodifier.StringRequest, resp *stringplanmodifier.RequiresReplaceIfFuncResponse) {
		resp.RequiresReplace = req.PlanValue.IsUnknown() || a.Normalize(req.PlanValue.ValueString()) != a.Normalize(req.StateValue.ValueString())
	}, desc, desc)
}

func (r *facadeResource) title(verb string) string {
	return fmt.Sprintf("Could not %s sreagent_%s", verb, r.spec.TypeName)
}

// rowPath addresses one row; id is the resource's own id attribute.
func (r *facadeResource) rowPath(id string) (string, url.Values) {
	switch r.spec.Shape {
	case Singleton:
		return r.spec.Key, nil
	case ServiceBinding:
		service, env := splitBindingID(id)
		q := url.Values{}
		if env != "" {
			q.Set("environment", env)
		}
		return r.spec.Key + "/" + url.PathEscape(service), q
	}
	return r.spec.Key + "/" + url.PathEscape(id), nil
}

func bindingID(service, env string) string {
	id := url.PathEscape(service)
	if env != "" {
		id += "?environment=" + url.QueryEscape(env)
	}
	return id
}

func splitBindingID(id string) (string, string) {
	escaped, query, _ := strings.Cut(id, "?environment=")
	service, err := url.PathUnescape(escaped)
	if err != nil {
		service = escaped
	}
	env, err := url.QueryUnescape(query)
	if err != nil {
		env = query
	}
	return service, env
}

// RowID is the id Terraform holds for a row: the uuid, the natural key, the
// singleton's key, or a service binding's escaped Location form.
func RowID(spec Spec, row map[string]any) string {
	switch spec.Shape {
	case Singleton:
		return spec.Key
	case NaturalKey:
		return fmt.Sprint(row[spec.IDAttr])
	case ServiceBinding:
		env, _ := row["environment"].(string)
		return bindingID(fmt.Sprint(row["service"]), env)
	}
	if id, ok := row["id"].(string); ok {
		return id
	}
	if spec.ListIDAttr != "" {
		return fmt.Sprint(row[spec.ListIDAttr])
	}
	return ""
}

func (r *facadeResource) body(ctx context.Context, plan, config, state valueSource, o op) (map[string]any, []string, diag.Diagnostics) {
	var diags diag.Diagnostics
	body := map[string]any{}
	var secrets []string
	for _, a := range r.spec.Attrs {
		if a.Computed || (o == opUpdate && (a.CreateOnly || a.NotRead)) || (o == opCreate && a.UpdateOnly) {
			continue
		}
		if a.Secret {
			value, sent, d := r.secretBody(ctx, a, plan, config, state, o)
			diags.Append(d...)
			if value != nil {
				body[a.Name] = value
				secrets = append(secrets, sent...)
			}
			continue
		}
		v, d := getValue(ctx, plan, a)
		diags.Append(d...)
		if v == nil || v.IsUnknown() {
			continue
		}
		var prior attr.Value
		if o == opUpdate && state != nil {
			prior, d = getValue(ctx, state, a)
			diags.Append(d...)
		}
		unchanged := prior != nil && !prior.IsUnknown() && (keep(a, prior, v) || (prior.IsNull() && v.IsNull()))
		// An upsert tool describes the row's whole state, so every field goes
		// out; the other tools change only the fields a call names, so a field
		// nobody changed stays out of the call (some, like a setting a person
		// decides, are refused even when resent unchanged).
		if o == opUpdate && !r.spec.Upsert && !a.Required && unchanged {
			continue
		}
		if v.IsNull() {
			if o == opUpdate && a.Clearable && r.spec.Shape != Singleton {
				body[a.Name] = nil
			}
			continue
		}
		if o == opUpdate && len(a.HiddenKeys) > 0 && prior != nil {
			if unchanged {
				continue
			}
			if r.hiddenHeld(ctx, state, a) {
				diags.AddAttributeError(path.Root(a.Attribute()), "Would drop values set in the app",
					fmt.Sprintf("sreagent_%s holds %s set in the app, which the platform never answers, and a change to %s replaces the stored %s as a whole, so applying it would silently drop them. Change %s in the app, or remove %s there first. Nothing was written.",
						r.spec.TypeName, strings.Join(a.HiddenKeys, " or "), a.Attribute(), a.Attribute(), a.Attribute(), strings.Join(a.HiddenKeys, " and ")))
				continue
			}
		}
		j, err := toJSON(v)
		if err != nil {
			diags.AddAttributeError(path.Root(a.Attribute()), "Invalid value", err.Error())
			continue
		}
		if o == opUpdate && a.MergedObject {
			j = clearRemovedKeys(j, prior)
		}
		body[a.Name] = j
	}
	return body, secrets, diags
}

// secretBody is what a write-only argument sends, and the strings an error
// must never repeat: the whole value and, for a JSON secret, every string in
// it, since a refusal may quote one field rather than the whole object.
func (r *facadeResource) secretBody(ctx context.Context, a Attr, plan, config, state valueSource, o op) (any, []string, diag.Diagnostics) {
	var diags diag.Diagnostics
	n := a.Attribute()
	var wo types.String
	diags.Append(config.GetAttribute(ctx, path.Root(n+"_wo"), &wo)...)
	var planned types.Int64
	diags.Append(plan.GetAttribute(ctx, path.Root(n+"_wo_version"), &planned)...)
	changed := true
	if o == opUpdate && state != nil {
		var prior types.Int64
		diags.Append(state.GetAttribute(ctx, path.Root(n+"_wo_version"), &prior)...)
		changed = !prior.Equal(planned)
	}
	if wo.IsNull() || wo.IsUnknown() {
		switch {
		case o == opCreate && a.Required && r.spec.Shape != Singleton:
			diags.AddAttributeError(path.Root(n+"_wo"), "Missing required secret", fmt.Sprintf("%s_wo is required to create sreagent_%s.", n, r.spec.TypeName))
		case o == opUpdate && changed && !planned.IsNull():
			diags.AddAttributeError(path.Root(n+"_wo"), "Missing secret", fmt.Sprintf("%s_wo_version changed but %s_wo is not set, so there is nothing to send. Set %s_wo in the same apply.", n, n, n))
		}
		return nil, nil, diags
	}
	if planned.IsNull() {
		diags.AddAttributeError(path.Root(n+"_wo_version"), "Missing version", fmt.Sprintf("Set %s_wo_version whenever %s_wo is set, and change it to send a new value.", n, n))
		return nil, nil, diags
	}
	if !changed {
		return nil, nil, diags
	}
	sent := []string{wo.ValueString()}
	if a.Kind != JSON {
		return wo.ValueString(), sent, diags
	}
	var parsed any
	if err := json.Unmarshal([]byte(wo.ValueString()), &parsed); err != nil {
		diags.AddAttributeError(path.Root(n+"_wo"), "Invalid JSON", "Use jsonencode(...) for this value.")
		return nil, nil, diags
	}
	return parsed, append(sent, stringLeaves(parsed)...), diags
}

// stringLeaves is every string in a decoded JSON value long enough that
// redacting it cannot mangle an unrelated message.
func stringLeaves(v any) []string {
	switch t := v.(type) {
	case string:
		if len(t) >= 4 {
			return []string{t}
		}
	case map[string]any:
		var out []string
		for _, x := range t {
			out = append(out, stringLeaves(x)...)
		}
		return out
	case []any:
		var out []string
		for _, x := range t {
			out = append(out, stringLeaves(x)...)
		}
		return out
	}
	return nil
}

// clearRemovedKeys names, with an empty string, every key the prior object
// held that the planned one dropped: a merged object keeps what a write
// leaves out, and an empty string is how it clears one.
func clearRemovedKeys(planned any, prior attr.Value) any {
	obj, ok := planned.(map[string]any)
	if !ok || prior == nil || prior.IsNull() || prior.IsUnknown() {
		return planned
	}
	n, ok := prior.(jsontypes.Normalized)
	if !ok {
		return planned
	}
	var before map[string]any
	if json.Unmarshal([]byte(n.ValueString()), &before) != nil {
		return planned
	}
	for k := range before {
		if _, kept := obj[k]; !kept {
			obj[k] = ""
		}
	}
	return obj
}

// hiddenHeld reports whether state says the row stores keys of a that the
// platform never answers.
func (r *facadeResource) hiddenHeld(ctx context.Context, state valueSource, a Attr) bool {
	for _, name := range a.HiddenSetBy {
		for _, c := range r.spec.Attrs {
			if c.Name != name {
				continue
			}
			v, d := getValue(ctx, state, c)
			if d.HasError() || v == nil || v.IsNull() || v.IsUnknown() {
				continue
			}
			switch t := v.(type) {
			case types.Bool:
				if t.ValueBool() {
					return true
				}
			case types.List:
				if len(t.Elements()) > 0 {
					return true
				}
			}
		}
	}
	return false
}

func (r *facadeResource) writeState(ctx context.Context, out *client.Response, prior valueSource, state *tfsdk.State, priv privateData, identity *tfsdk.ResourceIdentity) diag.Diagnostics {
	var diags diag.Diagnostics
	row, err := decodeRow(out.Data)
	if err != nil {
		diags.AddError("Unexpected API answer", err.Error())
		return diags
	}
	id := RowID(r.spec, row)
	diags.Append(state.SetAttribute(ctx, path.Root("id"), types.StringValue(id))...)
	for _, a := range r.spec.Attrs {
		if a.Secret {
			version := types.Int64Null()
			if prior != nil {
				diags.Append(prior.GetAttribute(ctx, path.Root(a.Attribute()+"_wo_version"), &version)...)
			}
			set, _ := row[a.Name+"_set"].(bool)
			diags.Append(state.SetAttribute(ctx, path.Root(a.Attribute()+"_wo"), types.StringNull())...)
			diags.Append(state.SetAttribute(ctx, path.Root(a.Attribute()+"_wo_version"), version)...)
			diags.Append(state.SetAttribute(ctx, path.Root(a.Attribute()+"_set"), types.BoolValue(set))...)
			continue
		}
		if a.NotRead {
			if prior != nil {
				v, d := getValue(ctx, prior, a)
				diags.Append(d...)
				diags.Append(state.SetAttribute(ctx, path.Root(a.Attribute()), v)...)
			}
			continue
		}
		remote, err := fromJSON(a.Kind, row[a.Name])
		if err != nil {
			diags.AddAttributeError(path.Root(a.Attribute()), "Unexpected API answer", err.Error())
			continue
		}
		if prior != nil && !a.Computed {
			before, d := getValue(ctx, prior, a)
			diags.Append(d...)
			if keep(a, before, remote) {
				remote = before
			}
		}
		diags.Append(state.SetAttribute(ctx, path.Root(a.Attribute()), remote)...)
	}
	if priv != nil {
		// An answer without an ETag clears the stored one: a stale version
		// would refuse every later write, and a missing one refuses them too.
		b, _ := json.Marshal(out.ETag)
		diags.Append(priv.SetKey(ctx, "etag", b)...)
		if out.ETag == "" {
			diags.AddWarning("No row version", fmt.Sprintf("The platform answered sreagent_%s without an ETag, so Terraform cannot prove a later write starts from this version; updates and deletes are refused until a refresh records one.", r.spec.TypeName))
		}
	}
	diags.Append(r.setIdentity(ctx, identity, id)...)
	return diags
}

// setIdentity records the identity of the row Terraform holds as id.
func (r *facadeResource) setIdentity(ctx context.Context, identity *tfsdk.ResourceIdentity, id string) diag.Diagnostics {
	var diags diag.Diagnostics
	if identity == nil {
		return diags
	}
	if r.spec.Shape == ServiceBinding {
		service, env := splitBindingID(id)
		diags.Append(identity.SetAttribute(ctx, path.Root("service"), types.StringValue(service))...)
		diags.Append(identity.SetAttribute(ctx, path.Root("environment"), types.StringValue(env))...)
		return diags
	}
	diags.Append(identity.SetAttribute(ctx, path.Root("id"), types.StringValue(id))...)
	return diags
}

// matchesState reports whether an answered row holds every configuration
// value state holds.
func (r *facadeResource) matchesState(ctx context.Context, state valueSource, out *client.Response) (bool, diag.Diagnostics) {
	var diags diag.Diagnostics
	row, err := decodeRow(out.Data)
	if err != nil {
		diags.AddError("Unexpected API answer", err.Error())
		return false, diags
	}
	for _, a := range r.spec.Attrs {
		if a.Computed || a.Secret || a.NotRead {
			continue
		}
		held, d := getValue(ctx, state, a)
		diags.Append(d...)
		remote, err := fromJSON(a.Kind, row[a.Name])
		if err != nil || held == nil {
			return false, diags
		}
		if !keep(a, held, remote) && (!held.IsNull() || !remote.IsNull()) {
			return false, diags
		}
	}
	return true, diags
}

// noVersion is the diagnostic for a write with no recorded row version:
// sending it without If-Match would overwrite a browser edit unchecked.
func (r *facadeResource) noVersion(verb string) (string, string) {
	return r.title(verb), fmt.Sprintf("No row version was recorded for sreagent_%s, so the write could overwrite a change made in the app. Nothing was written. Run terraform plan to refresh it, then apply again.", r.spec.TypeName)
}

func (r *facadeResource) etag(ctx context.Context, priv privateData) (string, diag.Diagnostics) {
	b, diags := priv.GetKey(ctx, "etag")
	if len(b) == 0 {
		return "", diags
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return "", diags
	}
	return s, diags
}

// readOnly is the diagnostic for a write refused because the configured key is
// the plan-only api:config_read kind.
func (r *facadeResource) readOnly(verb string) (string, string) {
	return r.title(verb), "The configured API key holds api:config_read, which can plan but not apply. Apply with an api:admin key."
}

func (r *facadeResource) precondition(verb string, err error) (string, string) {
	if client.IsRetried(err) {
		return r.title(verb), fmt.Sprintf("sreagent_%s: the platform answered 412 to a retried request after a gateway error, so the first attempt may have been applied. Run terraform plan to see the row as it is now.", r.spec.TypeName)
	}
	return r.title(verb), fmt.Sprintf("sreagent_%s changed outside Terraform after the last refresh (the platform answered 412). Nothing was written. Run terraform plan again to review the change.", r.spec.TypeName)
}

func (r *facadeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	body, secrets, diags := r.body(ctx, req.Plan, req.Config, nil, opCreate)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	var out *client.Response
	var err error
	if r.spec.Shape == Singleton {
		out, err = r.adoptSingleton(ctx, body, secrets)
	} else {
		out, err = r.client.Do(ctx, client.Request{Method: http.MethodPost, Path: r.spec.Key, Body: body, Secrets: secrets})
	}
	if client.IsReadOnlyKey(err) {
		summary, detail := r.readOnly("create")
		resp.Diagnostics.AddError(summary, detail)
		return
	}
	if client.IsConflict(err) {
		resp.Diagnostics.AddError(r.title("create"), fmt.Sprintf("A row with this key already exists on the platform. Import it instead of creating it: %s", err))
		return
	}
	if err != nil {
		resp.Diagnostics.AddError(r.title("create"), err.Error())
		return
	}
	// The row exists from here on: state records it before the follow-up, so a
	// failed follow-up leaves it tainted in state rather than orphaned.
	resp.Diagnostics.Append(r.writeState(ctx, out, req.Plan, &resp.State, resp.Private, resp.Identity)...)
	if resp.Diagnostics.HasError() {
		return
	}
	followed, err := r.followUp(ctx, req.Plan, out)
	if err != nil {
		resp.Diagnostics.AddError(r.title("create"), "The row was created, but setting the fields its create does not take failed; it is marked tainted and the next apply replaces it. "+err.Error())
		return
	}
	if followed != out {
		resp.Diagnostics.Append(r.writeState(ctx, followed, req.Plan, &resp.State, resp.Private, resp.Identity)...)
	}
}

// followUp sends UpdateOnly fields whose planned value differs from the created row.
func (r *facadeResource) followUp(ctx context.Context, plan valueSource, created *client.Response) (*client.Response, error) {
	row, err := decodeRow(created.Data)
	if err != nil {
		return nil, err
	}
	body := map[string]any{}
	for _, a := range r.spec.Attrs {
		if !a.UpdateOnly {
			continue
		}
		v, d := getValue(ctx, plan, a)
		if d.HasError() || v == nil || v.IsNull() || v.IsUnknown() {
			continue
		}
		remote, err := fromJSON(a.Kind, row[a.Name])
		if err == nil && keep(a, v, remote) {
			continue
		}
		j, err := toJSON(v)
		if err != nil {
			return nil, err
		}
		body[a.Name] = j
	}
	if len(body) == 0 {
		return created, nil
	}
	if created.ETag == "" {
		return nil, fmt.Errorf("the create answered no row version, so the fields it does not take were not sent")
	}
	p, q := r.rowPath(RowID(r.spec, row))
	return r.client.Do(ctx, client.Request{Method: http.MethodPut, Path: p, Query: q, Body: body, IfMatch: created.ETag})
}

func (r *facadeResource) adoptSingleton(ctx context.Context, body map[string]any, secrets []string) (*client.Response, error) {
	current, err := r.client.Do(ctx, client.Request{Method: http.MethodGet, Path: r.spec.Key})
	ifMatch := ""
	switch {
	case err == nil:
		ifMatch = current.ETag
		if len(body) == 0 {
			return current, nil
		}
		if ifMatch == "" {
			return nil, fmt.Errorf("the platform answered the current sreagent_%s without a row version, so adopting it could overwrite a change made in the app; nothing was written", r.spec.TypeName)
		}
	case client.IsNotFound(err):
		if len(body) == 0 {
			return nil, fmt.Errorf("sreagent_%s has no row of its own yet; set at least one attribute to create it", r.spec.TypeName)
		}
	default:
		return nil, err
	}
	return r.client.Do(ctx, client.Request{Method: http.MethodPut, Path: r.spec.Key, Body: body, IfMatch: ifMatch, Secrets: secrets})
}

func (r *facadeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var id types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("id"), &id)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// The framework refuses a read that answers no identity, even one that
	// removes the resource, and a row can reach here without one stored
	// (after a failed apply on Terraform 1.12, for one).
	resp.Diagnostics.Append(r.setIdentity(ctx, resp.Identity, id.ValueString())...)
	p, q := r.rowPath(id.ValueString())
	out, err := r.client.Do(ctx, client.Request{Method: http.MethodGet, Path: p, Query: q})
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError(r.title("read"), err.Error())
		return
	}
	resp.Diagnostics.Append(r.writeState(ctx, out, req.State, &resp.State, resp.Private, resp.Identity)...)
}

func (r *facadeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var id types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("id"), &id)...)
	// A failed update keeps the prior state, and with it this identity: some
	// Terraform versions send no planned identity, and a row stored without one
	// fails the next refresh that finds it deleted.
	resp.Diagnostics.Append(r.setIdentity(ctx, resp.Identity, id.ValueString())...)
	body, secrets, diags := r.body(ctx, req.Plan, req.Config, req.State, opUpdate)
	resp.Diagnostics.Append(diags...)
	etag, d := r.etag(ctx, req.Private)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	p, q := r.rowPath(id.ValueString())
	request := client.Request{Method: http.MethodPut, Path: p, Query: q, Body: body, IfMatch: etag, Secrets: secrets}
	if len(body) == 0 {
		// Nothing to write: read the row for state instead.
		request = client.Request{Method: http.MethodGet, Path: p, Query: q}
	} else if etag == "" {
		summary, detail := r.noVersion("update")
		resp.Diagnostics.AddError(summary, detail)
		return
	}
	out, err := r.client.Do(ctx, request)
	switch {
	case client.IsReadOnlyKey(err):
		summary, detail := r.readOnly("update")
		resp.Diagnostics.AddError(summary, detail)
		return
	case client.IsPreconditionFailed(err):
		summary, detail := r.precondition("update", err)
		resp.Diagnostics.AddError(summary, detail)
		return
	case client.IsNotFound(err):
		resp.Diagnostics.AddError(r.title("update"), "The row was deleted outside Terraform. Run terraform plan to recreate it.")
		return
	case err != nil:
		resp.Diagnostics.AddError(r.title("update"), err.Error())
		return
	}
	resp.Diagnostics.Append(r.writeState(ctx, out, req.Plan, &resp.State, resp.Private, resp.Identity)...)
}

func (r *facadeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	if r.spec.Shape == Singleton {
		resp.Diagnostics.AddWarning("Settings left in place", fmt.Sprintf("sreagent_%s is one row per organization and cannot be deleted; Terraform stopped managing it and the platform keeps its current values.", r.spec.TypeName))
		return
	}
	var id types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("id"), &id)...)
	discovered := types.BoolNull()
	if r.spec.DiscoveredAttr != "" {
		resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root(r.spec.DiscoveredAttr), &discovered)...)
	}
	etag, d := r.etag(ctx, req.Private)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	p, q := r.rowPath(id.ValueString())
	if etag == "" {
		// Terraform keeps a tainted row's state but not its private data, so a
		// replacement can arrive here with no version. The row is deleted only
		// when it still holds what state says, under the version just read.
		current, err := r.client.Do(ctx, client.Request{Method: http.MethodGet, Path: p, Query: q})
		if client.IsNotFound(err) {
			return
		}
		if err != nil {
			resp.Diagnostics.AddError(r.title("delete"), err.Error())
			return
		}
		same, d := r.matchesState(ctx, req.State, current)
		resp.Diagnostics.Append(d...)
		if !same || current.ETag == "" {
			summary, detail := r.noVersion("delete")
			resp.Diagnostics.AddError(summary, detail)
			return
		}
		etag = current.ETag
	}
	_, err := r.client.Do(ctx, client.Request{Method: http.MethodDelete, Path: p, Query: q, IfMatch: etag})
	switch {
	case err == nil, client.IsNotFound(err):
		if w := discoveredWarning(r.spec, discovered.ValueBool()); w != "" && err == nil {
			resp.Diagnostics.AddWarning("Discovered row", w)
		}
		return
	case client.IsReadOnlyKey(err):
		summary, detail := r.readOnly("delete")
		resp.Diagnostics.AddError(summary, detail)
	case client.IsPreconditionFailed(err):
		summary, detail := r.precondition("delete", err)
		resp.Diagnostics.AddError(summary, detail)
	default:
		resp.Diagnostics.AddError(r.title("delete"), err.Error())
	}
}

// describe is an attribute's description, naming its allowed values.
func describe(a Attr) string {
	if len(a.OneOf) == 0 {
		return a.Description
	}
	return fmt.Sprintf("%s One of: %s.", a.Description, strings.Join(a.OneOf, ", "))
}

// discoveredWarning is said when Terraform destroys a row a discovery sweep
// filed: the platform deletes it, and the next sweep files it again.
func discoveredWarning(spec Spec, discovered bool) string {
	if !discovered || spec.DiscoveredAttr == "" {
		return ""
	}
	return fmt.Sprintf("A discovery sweep filed this sreagent_%s. It was deleted, and the next sweep files it again while the estate still runs it; stop it at the source (the connector) to keep it gone.", spec.TypeName)
}

func (r *facadeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id := req.ID
	if id == "" && req.Identity != nil {
		if r.spec.Shape == ServiceBinding {
			var service, env types.String
			resp.Diagnostics.Append(req.Identity.GetAttribute(ctx, path.Root("service"), &service)...)
			resp.Diagnostics.Append(req.Identity.GetAttribute(ctx, path.Root("environment"), &env)...)
			id = bindingID(service.ValueString(), env.ValueString())
		} else {
			var v types.String
			resp.Diagnostics.Append(req.Identity.GetAttribute(ctx, path.Root("id"), &v)...)
			id = v.ValueString()
		}
	}
	if r.spec.Shape == Singleton && id != r.spec.Key {
		resp.Diagnostics.AddError("Import a singleton by its name", fmt.Sprintf("Import sreagent_%s with the ID %q.", r.spec.TypeName, r.spec.Key))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), types.StringValue(id))...)
}
