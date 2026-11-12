package engine

import (
	"context"
	"fmt"
	"slices"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// attrNamed is the attribute the API calls name. A Complement attribute reuses
// the name of the list it is derived from, so the list itself wins.
func (r *facadeResource) attrNamed(name string) (Attr, bool) {
	var found Attr
	ok := false
	for _, a := range r.spec.Attrs {
		if a.Name == name && (!ok || len(found.Complement) > 0) {
			found, ok = a, true
		}
	}
	return found, ok
}

func stringsOf(l types.List) []string {
	out := make([]string, 0, len(l.Elements()))
	for _, e := range l.Elements() {
		if s, ok := e.(types.String); ok && !s.IsNull() && !s.IsUnknown() {
			out = append(out, s.ValueString())
		}
	}
	return out
}

// answered lets followUp ask resets about the row a create answered: it holds
// the answered list under the set-here attribute's name.
type answered struct{ row map[string]any }

func (a answered) GetAttribute(_ context.Context, p path.Path, target any) diag.Diagnostics {
	var diags diag.Diagnostics
	dst, ok := target.(*types.List)
	if !ok {
		diags.AddError("Provider bug", "answered reads lists only")
		return diags
	}
	*dst = types.ListNull(types.StringType)
	for key, raw := range a.row {
		if p.Equal(path.Root(key)) {
			if items, ok := raw.([]any); ok {
				v, err := fromJSON(StringList, items)
				if err != nil {
					diags.AddError("Unexpected API answer", err.Error())
					return diags
				}
				*dst = v.(types.List)
			}
		}
	}
	return diags
}

// resets are the settings the configuration puts back on the organization
// default that the row holds itself, in the order the API lists them. A
// Clearable setting is left out: removing it from the configuration already
// sends null, which is how the platform returns it to the default. held is read
// from state on an update and from the answered row after a create.
func (r *facadeResource) resets(ctx context.Context, config, held valueSource) ([]string, diag.Diagnostics) {
	var diags diag.Diagnostics
	if r.spec.FollowsDefault == "" || held == nil {
		return nil, diags
	}
	fd, ok := r.attrNamed(r.spec.FollowsDefault)
	if !ok {
		return nil, diags
	}
	var named, holds types.List
	diags.Append(config.GetAttribute(ctx, path.Root(fd.Attribute()), &named)...)
	setHere := r.spec.SetHere
	if sh, ok := r.attrNamed(r.spec.SetHere); ok {
		setHere = sh.Attribute()
	}
	diags.Append(held.GetAttribute(ctx, path.Root(setHere), &holds)...)
	if diags.HasError() || named.IsNull() || named.IsUnknown() || holds.IsNull() || holds.IsUnknown() {
		return nil, diags
	}
	want, have := stringsOf(named), stringsOf(holds)
	var out []string
	for _, name := range fd.OneOf {
		a, known := r.attrNamed(name)
		if known && a.Clearable {
			continue
		}
		if slices.Contains(want, name) && slices.Contains(have, name) {
			out = append(out, name)
		}
	}
	return out, diags
}

// planResets plans the change a reset is: each setting the configuration puts
// back on the default and the row holds becomes unknown until the answer, with
// the lists that say which settings the row holds. Until then the plan equals
// state, and a row that already follows plans nothing, so the write-only list
// is never a diff by itself.
func (r *facadeResource) planResets(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	names, d := r.resets(ctx, req.Config, req.State)
	resp.Diagnostics.Append(d...)
	if len(names) == 0 {
		return
	}
	for _, name := range names {
		a, _ := r.attrNamed(name)
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root(a.Attribute()), unknownOf(a))...)
		resp.Diagnostics.AddAttributeWarning(path.Root(a.Attribute()), "Setting put back on the organization default",
			fmt.Sprintf("%s is set on this row; this apply puts it back on the organization default.", a.Attribute()))
	}
	for _, a := range r.spec.Attrs {
		if a.Name == r.spec.SetHere {
			resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root(a.Attribute()), types.ListUnknown(types.StringType))...)
		}
	}
}

// validateFollows refuses a setting that is both stated and put back on the
// default, which the platform refuses in the same words.
func (r *facadeResource) validateFollows(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	if r.spec.FollowsDefault == "" {
		return
	}
	fd, ok := r.attrNamed(r.spec.FollowsDefault)
	if !ok {
		return
	}
	var named types.List
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root(fd.Attribute()), &named)...)
	if named.IsNull() || named.IsUnknown() {
		return
	}
	for _, name := range stringsOf(named) {
		a, known := r.attrNamed(name)
		if !known {
			continue
		}
		v, d := getValue(ctx, req.Config, a)
		resp.Diagnostics.Append(d...)
		if v == nil || v.IsNull() {
			continue
		}
		resp.Diagnostics.AddAttributeError(path.Root(fd.Attribute()), "Set and reset to follow the default",
			fmt.Sprintf("%s is both set and reset to follow the default; send one.", a.Attribute()))
	}
}

func unknownOf(a Attr) attr.Value {
	switch a.Kind {
	case Int:
		return types.Int64Unknown()
	case Float:
		return types.Float64Unknown()
	case Bool:
		return types.BoolUnknown()
	case StringList:
		return types.ListUnknown(types.StringType)
	}
	return types.StringUnknown()
}

func nullOf(k Kind) attr.Value {
	v, _ := fromJSON(k, nil)
	return v
}

// follows reports that the answered list of held settings does not name one: the
// row follows the default for it. An answer with no such list says nothing.
func follows(heldList any, name string) bool {
	items, ok := heldList.([]any)
	if !ok {
		return false
	}
	for _, it := range items {
		if it == name {
			return false
		}
	}
	return true
}

// complement is the members of a.Complement that the answered list does not hold.
func complement(a Attr, heldList any) attr.Value {
	items, ok := heldList.([]any)
	if !ok {
		return types.ListNull(types.StringType)
	}
	elems := []attr.Value{}
	for _, name := range a.Complement {
		if follows(items, name) {
			elems = append(elems, types.StringValue(name))
		}
	}
	return types.ListValueMust(types.StringType, elems)
}

// withoutValues drops entries from an answered list.
func withoutValues(v attr.Value, drop []string) attr.Value {
	l, ok := v.(types.List)
	if !ok || l.IsNull() || l.IsUnknown() {
		return v
	}
	kept := []attr.Value{}
	for _, e := range l.Elements() {
		if s, ok := e.(types.String); ok && slices.Contains(drop, s.ValueString()) {
			continue
		}
		kept = append(kept, e)
	}
	return types.ListValueMust(types.StringType, kept)
}
