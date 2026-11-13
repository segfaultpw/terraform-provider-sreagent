package engine

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/client"
)

// A singleton the organization already has is adopted by a block with no import: the
// create PUTs what the block states over what the platform holds. Terraform shows that
// as a plain create, so a spec with PlanAdoption reads the current row at plan time (a
// GET, never a write) and says what the apply will move, and judges the block against
// those values the way it judges an update against state.

// rowSource reads an answered row as a state: the values the platform holds, with a null
// answer read as the attribute's NullAs, as a refresh reads it.
type rowSource struct {
	row   map[string]any
	attrs []Attr
}

func (s rowSource) GetAttribute(_ context.Context, p path.Path, target any) diag.Diagnostics {
	var diags diag.Diagnostics
	var found *Attr
	for i := range s.attrs {
		if p.Equal(path.Root(s.attrs[i].Attribute())) {
			found = &s.attrs[i]
			break
		}
	}
	var value attr.Value
	if found != nil {
		v, err := fromJSON(found.Kind, s.row[found.Name])
		if err != nil {
			diags.AddAttributeError(p, "Unexpected API answer", err.Error())
			return diags
		}
		if sentinel, ok := found.nullAsValue(); ok && v.IsNull() {
			v = sentinel
		}
		value = v
	}
	switch dst := target.(type) {
	case *types.String:
		*dst = types.StringNull()
		if v, ok := value.(types.String); ok {
			*dst = v
		}
	case *types.Int64:
		*dst = types.Int64Null()
		if v, ok := value.(types.Int64); ok {
			*dst = v
		}
	case *types.Float64:
		*dst = types.Float64Null()
		if v, ok := value.(types.Float64); ok {
			*dst = v
		}
	case *types.Bool:
		*dst = types.BoolNull()
		if v, ok := value.(types.Bool); ok {
			*dst = v
		}
	case *types.List:
		*dst = types.ListNull(types.StringType)
		if v, ok := value.(types.List); ok {
			*dst = v
		}
	case *jsontypes.Normalized:
		*dst = jsontypes.NewNormalizedNull()
		if v, ok := value.(jsontypes.Normalized); ok {
			*dst = v
		}
	default:
		diags.AddError("Provider bug", fmt.Sprintf("rowSource cannot read into %T", target))
	}
	return diags
}

// adoptedConfig is the plan of an adopting create as the apply will write it: a value
// the block leaves out is left as it is, so where the plan has it unknown (an Optional
// and Computed attribute with no value in the block) the platform's current value is the
// one that will hold. A value the block states, or one that is unknown because it comes
// from another resource, is the plan's.
type adoptedConfig struct{ plan, config, current Config }

var _ Config = adoptedConfig{}

func (c adoptedConfig) String(name string) (string, Presence) {
	v, p := c.plan.String(name)
	if _, left := c.config.String(name); p == Unknown && left == Absent {
		return c.current.String(name)
	}
	return v, p
}

func (c adoptedConfig) Bool(name string) (bool, Presence) {
	v, p := c.plan.Bool(name)
	if _, left := c.config.Bool(name); p == Unknown && left == Absent {
		return c.current.Bool(name)
	}
	return v, p
}

func (c adoptedConfig) Int(name string) (int64, Presence) {
	v, p := c.plan.Int(name)
	if _, left := c.config.Int(name); p == Unknown && left == Absent {
		return c.current.Int(name)
	}
	return v, p
}

func (c adoptedConfig) Strings(name string) ([]string, Presence) {
	v, p := c.plan.Strings(name)
	if _, left := c.config.Strings(name); p == Unknown && left == Absent {
		return c.current.Strings(name)
	}
	return v, p
}

func (c adoptedConfig) List(name string) Presence {
	p := c.plan.List(name)
	if left := c.config.List(name); p == Unknown && left == Absent {
		return c.current.List(name)
	}
	return p
}

// adopting reports that the plan is the create of a singleton that adopts the
// organization's settings and wants the plan to say what that moves.
func (r *facadeResource) adopting(req resource.ModifyPlanRequest) bool {
	return r.spec.PlanAdoption && r.spec.Shape == Singleton && req.State.Raw.IsNull() && r.client != nil
}

// importAdvice is how to see an adoption as an ordinary plan.
func (r *facadeResource) importAdvice() string {
	return fmt.Sprintf("Import it first (terraform import sreagent_%s.<name> %s) to review the change as a plan.", r.spec.TypeName, r.spec.Key)
}

// currentRow reads the row an adopting create would overwrite. It never fails the
// plan: a read that cannot be made is said, and the apply answers.
func (r *facadeResource) currentRow(ctx context.Context, resp *resource.ModifyPlanResponse) *rowSource {
	out, err := r.client.Do(ctx, client.Request{Method: http.MethodGet, Path: r.spec.Key})
	switch {
	case err == nil:
		row, derr := decodeRow(out.Data)
		if derr == nil {
			return &rowSource{row: row, attrs: r.spec.Attrs}
		}
		err = derr
	case client.IsNotFound(err):
		if r.alwaysThere() {
			resp.Diagnostics.AddWarning("The platform may be too old for this resource", r.platformTooOld(err))
		}
		return nil
	}
	_, detail := r.failure("read", err)
	resp.Diagnostics.AddWarning("The plan could not read the current settings",
		fmt.Sprintf("sreagent_%s adopts the settings the organization already has, and the plan could not read them, so it cannot say what the apply will change: %s\n\nThe apply overwrites what the block names. %s", r.spec.TypeName, detail, r.importAdvice()))
	return nil
}

// alwaysThere reports a singleton every organization has on a platform new enough to
// serve it: a 404 for it means the platform predates the resource.
func (r *facadeResource) alwaysThere() bool {
	return r.spec.Shape == Singleton && r.spec.AlwaysPresent && r.spec.MinPlatform != ""
}

// planAdoptionWarnings says what an adopting create changes: the effects each attribute
// names, then every stated attribute whose value is not the one the organization holds.
func (r *facadeResource) planAdoptionWarnings(ctx context.Context, plan valueSource, current rowSource, diags *diag.Diagnostics) {
	r.planWarnings(ctx, plan, current, diags)
	var moves []string
	for _, a := range r.spec.Attrs {
		if a.Computed || a.Secret || a.Local || a.WriteOnly || a.Kept || a.NotRead || len(a.Complement) > 0 {
			continue
		}
		planned, d := getValue(ctx, plan, a)
		diags.Append(d...)
		held, d := getValue(ctx, current, a)
		diags.Append(d...)
		if planned == nil || held == nil || planned.IsNull() || planned.IsUnknown() || planned.Equal(held) || keep(a, held, planned) {
			continue
		}
		moves = append(moves, fmt.Sprintf("%s from %s to %s", a.Attribute(), showValue(held), showValue(planned)))
	}
	if len(moves) == 0 {
		return
	}
	diags.AddWarning("This block adopts the organization's current settings",
		fmt.Sprintf("sreagent_%s has no state yet, so this block adopts the settings the organization already has, and the apply overwrites what the block names. It will change: %s. An attribute the block leaves out stays as it is. %s",
			r.spec.TypeName, strings.Join(moves, "; "), r.importAdvice()))
}

// showValue is a value as a sentence says it.
func showValue(v attr.Value) string {
	switch t := v.(type) {
	case types.String:
		if t.IsNull() {
			return "not set"
		}
		return strconv.Quote(t.ValueString())
	case types.Int64:
		if t.IsNull() {
			return "not set"
		}
		return strconv.FormatInt(t.ValueInt64(), 10)
	case types.Bool:
		if t.IsNull() {
			return "not set"
		}
		return strconv.FormatBool(t.ValueBool())
	case types.List:
		if t.IsNull() {
			return "not set"
		}
		return "[" + strings.Join(stringsOf(t), ", ") + "]"
	}
	return v.String()
}
