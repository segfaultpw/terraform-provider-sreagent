package engine

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
)

// sameSetAsState plans the list state holds when the configuration names the
// same entries in another order. Terraform accepts a plan that returns the prior
// value for a configuration it is equivalent to, and the platform stores these
// lists sorted, so a reordered configuration is no change.
type sameSetAsState struct{}

var _ planmodifier.List = sameSetAsState{}

func (sameSetAsState) Description(context.Context) string {
	return "A list that holds the same entries as the stored one, in any order, is no change."
}

func (m sameSetAsState) MarkdownDescription(ctx context.Context) string { return m.Description(ctx) }

func (sameSetAsState) PlanModifyList(_ context.Context, req planmodifier.ListRequest, resp *planmodifier.ListResponse) {
	if sameSet(req.PlanValue, req.StateValue) {
		resp.PlanValue = req.StateValue
	}
}

// sameInstantAsState plans the time state holds when the configuration names the
// same instant in another spelling (another zone, with or without fractions).
type sameInstantAsState struct{}

var _ planmodifier.String = sameInstantAsState{}

func (sameInstantAsState) Description(context.Context) string {
	return "A time that is the same instant as the stored one is no change."
}

func (m sameInstantAsState) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (sameInstantAsState) PlanModifyString(_ context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.PlanValue.IsNull() || req.PlanValue.IsUnknown() || req.StateValue.IsNull() || req.StateValue.IsUnknown() {
		return
	}
	planned, err := time.Parse(time.RFC3339, req.PlanValue.ValueString())
	if err != nil {
		return
	}
	held, err := time.Parse(time.RFC3339, req.StateValue.ValueString())
	if err != nil {
		return
	}
	if planned.Equal(held) {
		resp.PlanValue = req.StateValue
	}
}

// sameNormalizedAsState plans the list state holds when the configuration names
// the same entries in the same order, spelled as the platform's normalization
// reads them equal (an emoji with or without its presentation selector). The
// platform answers its own spelling, and Terraform accepts a plan that returns
// the prior value for a configuration it is equivalent to.
type sameNormalizedAsState struct{ normalize func(string) string }

var _ planmodifier.List = sameNormalizedAsState{}

func (sameNormalizedAsState) Description(context.Context) string {
	return "A list whose entries equal the stored ones after the platform's normalization is no change."
}

func (m sameNormalizedAsState) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m sameNormalizedAsState) PlanModifyList(_ context.Context, req planmodifier.ListRequest, resp *planmodifier.ListResponse) {
	if sameAfter(m.normalize, req.PlanValue, req.StateValue) {
		resp.PlanValue = req.StateValue
	}
}

// sameNormalizedMapAsState plans the map state holds when the configuration names
// the same keys with values the platform's normalization reads equal (an emoji
// with or without its presentation selector). The platform answers its own
// spelling, and Terraform accepts a plan that returns the prior value for a
// configuration it is equivalent to.
type sameNormalizedMapAsState struct{ normalize func(string) string }

var _ planmodifier.Map = sameNormalizedMapAsState{}

func (sameNormalizedMapAsState) Description(context.Context) string {
	return "A map whose values equal the stored ones after the platform's normalization is no change."
}

func (m sameNormalizedMapAsState) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m sameNormalizedMapAsState) PlanModifyMap(_ context.Context, req planmodifier.MapRequest, resp *planmodifier.MapResponse) {
	if sameMapAfter(m.normalize, req.PlanValue, req.StateValue) {
		resp.PlanValue = req.StateValue
	}
}
