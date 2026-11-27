package engine

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/client"
)

// A PathAndQuery row (what a parent organization gives one child of one GitHub
// account) has no door that creates or deletes it: something in the app makes
// the row, and Terraform adopts it (AdoptOnly) and leaves it in place when the
// resource is destroyed (LeaveOnDestroy). Its writes are applied against a
// preview when they take something away (Previewed with a ProbeFingerprint).

// addressedID is the id of the row a configuration (or a plan) addresses: the
// path value and the query value of a PathAndQuery row, the natural key of any
// other. ok is false when a part of the address is not known yet.
func (r *facadeResource) addressedID(ctx context.Context, src valueSource) (id string, ok bool, diags diag.Diagnostics) {
	var segment, value types.String
	for _, a := range r.spec.Attrs {
		if !a.Address {
			continue
		}
		var v types.String
		diags.Append(src.GetAttribute(ctx, path.Root(a.Attribute()), &v)...)
		switch a.Name {
		case r.spec.IDAttr:
			segment = v
		case r.spec.QueryAttr:
			value = v
		}
	}
	if diags.HasError() || segment.IsNull() || segment.IsUnknown() || value.IsUnknown() {
		return "", false, diags
	}
	if r.spec.Shape != PathAndQuery {
		return segment.ValueString(), true, diags
	}
	return pathQueryID(r.spec.QueryAttr, segment.ValueString(), value.ValueString()), true, diags
}

// adoptMissing is the sentence for an adoption whose row the platform does not
// hold.
func (r *facadeResource) adoptMissing(id string, err error) string {
	hint := ""
	if r.spec.AdoptHint != "" {
		hint = " " + r.spec.AdoptHint
	}
	return fmt.Sprintf("%s\n\nThe platform has no sreagent_%s for %s, and it has no door that creates one, so there is nothing to adopt. Connect or link it in the app first, then declare this resource for it.%s",
		err.Error(), r.spec.TypeName, id, hint)
}

// createAdopted adopts the row the platform holds for the configured address:
// it reads the row, judges the block against it as an update is judged against
// state, and writes the difference through the update door.
func (r *facadeResource) createAdopted(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	id, ok, d := r.addressedID(ctx, req.Plan)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !ok {
		resp.Diagnostics.AddError(r.title("create"), "The row this resource addresses is not known yet, so there is nothing to adopt.")
		return
	}
	p, q := r.rowPath(id)
	current, err := r.client.Do(ctx, client.Request{Method: http.MethodGet, Path: p, Query: q})
	switch {
	case client.IsNotFound(err):
		resp.Diagnostics.AddError(r.title("create"), r.adoptMissing(id, err))
		return
	case err != nil:
		summary, detail := r.failure("create", err)
		resp.Diagnostics.AddError(summary, detail)
		return
	}
	row, err := decodeRow(current.Data)
	if err != nil {
		resp.Diagnostics.AddError("Unexpected API answer", err.Error())
		return
	}
	body, secrets, d := r.body(ctx, req.Plan, req.Config, rowSource{row: row, attrs: r.spec.Attrs}, opUpdate)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	var planned types.String
	if r.spec.Previewed {
		resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root(fingerprintAttr), &planned)...)
	}
	out, used := current, ""
	if len(body) > 0 {
		if current.ETag == "" {
			resp.Diagnostics.AddError(r.title("create"), fmt.Sprintf("the platform answered the current sreagent_%s without a row version, so adopting it could overwrite a change made in the app; nothing was written", r.spec.TypeName))
			return
		}
		if r.spec.Previewed {
			confirm, _, d := r.configuredConfirm(ctx, req.Config)
			resp.Diagnostics.Append(d...)
			out, used, err = r.writeRow(ctx, &resp.Diagnostics, "create", id, body, current.ETag, confirm, planned)
			if errors.Is(err, errReported) {
				return
			}
		} else {
			out, err = r.client.Do(ctx, client.Request{Method: http.MethodPut, Path: p, Query: q, Body: body, IfMatch: current.ETag, Secrets: secrets})
		}
		if r.writeFailed(&resp.Diagnostics, "create", err) {
			return
		}
	}
	resp.Diagnostics.Append(r.writeState(ctx, out, req.Plan, &resp.State, resp.Private, resp.Identity)...)
	if r.spec.Previewed {
		r.recordFingerprint(ctx, &resp.State, &resp.Diagnostics, planned, used)
	}
}

// writeFailed adds the diagnostic for a failed write of a row and reports it.
func (r *facadeResource) writeFailed(diags *diag.Diagnostics, verb string, err error) bool {
	switch {
	case err == nil:
		return false
	case client.IsReadOnlyKey(err):
		summary, detail := r.readOnly(verb)
		diags.AddError(summary, detail)
	case client.IsPreconditionFailed(err):
		summary, detail := r.precondition(verb, err)
		diags.AddError(summary, detail)
	case client.IsNotFound(err) && verb == "update":
		diags.AddError(r.title(verb), "The row was deleted outside Terraform. Run terraform plan to see what is left.")
	default:
		summary, detail := r.failure(verb, err)
		diags.AddError(summary, detail)
	}
	return true
}

// writeRow writes a change of a Previewed collection row. A plan that took a
// preview sends its fingerprint, and a 409 means what would move changed since.
// A plan that found nothing to preview sends none, and fails without writing if
// the platform now answers a preview, because the plan never showed it. A plan
// that could not preview (a value known only at apply) is previewed here,
// judged as the plan would have been, and applied against its own fingerprint.
// It answers the fingerprint it applied.
func (r *facadeResource) writeRow(ctx context.Context, diags *diag.Diagnostics, verb, id string, body map[string]any, ifMatch string, confirm []string, planned types.String) (*client.Response, string, error) {
	switch {
	case !planned.IsNull() && !planned.IsUnknown() && planned.ValueString() != "":
		out, err := r.client.Do(ctx, r.previewedRequest(id, body, ifMatch, confirm, planned.ValueString()))
		if client.IsConflict(err) {
			r.staleError(diags, verb, err)
			return nil, "", errReported
		}
		return out, planned.ValueString(), err
	case planned.IsNull():
		out, err := r.client.Do(ctx, r.previewedRequest(id, body, ifMatch, confirm, ""))
		if client.IsPreconditionRequired(err) {
			var apiErr *client.APIError
			detail := ""
			if errors.As(err, &apiErr) {
				if fresh, perr := r.parsePreviewOf(apiErr.Preview); perr == nil {
					detail = " " + fresh.warning()
				}
			}
			diags.AddError(r.title(verb), "The platform now says this change takes something away, which the plan did not show, so nothing was written. Run terraform plan to review it."+detail)
			return nil, "", errReported
		}
		return out, "", err
	}
	warned := false
	for attempt := 0; ; attempt++ {
		p, written, err := r.probe(ctx, id, body, ifMatch, confirm)
		if err != nil {
			return nil, "", err
		}
		if written != nil {
			return written, "", nil
		}
		if probs := r.problems(p, confirm); len(probs) > 0 {
			r.addProblems(diags, probs)
			return nil, "", errReported
		}
		if !warned {
			diags.AddWarning("Applying against a preview taken now", "The plan could not preview this change, so the apply did. "+p.warning())
			warned = true
		}
		out, err := r.client.Do(ctx, r.previewedRequest(id, body, ifMatch, confirm, p.Fingerprint))
		if client.IsConflict(err) {
			if attempt == 0 {
				continue
			}
			r.staleError(diags, verb, err)
			return nil, "", errReported
		}
		return out, p.Fingerprint, err
	}
}

// planRowPreview takes the plan-time preview of a Previewed collection row and
// carries its fingerprint to the apply in the Local attribute. On a create (an
// adoption) current is the row the platform holds, read for the plan; the plan
// is judged against it as an update is judged against state.
func (r *facadeResource) planRowPreview(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse, current *rowSource) {
	creating := req.State.Raw.IsNull()
	setFingerprint := func(v types.String) {
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root(fingerprintAttr), v)...)
	}
	var (
		id      string
		prior   valueSource
		ifMatch string
	)
	if creating {
		if current == nil {
			if r.client != nil {
				resp.Diagnostics.AddWarning("This change is not previewed",
					"The row this resource adopts is not known yet, so the plan cannot ask the platform what the change moves; the apply previews it and applies what it finds.")
			}
			setFingerprint(types.StringUnknown())
			return
		}
		id, prior, ifMatch = current.id, *current, current.etag
	} else {
		var held types.String
		resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("id"), &held)...)
		id, prior = held.ValueString(), req.State
		if req.Private != nil {
			tag, d := r.etag(ctx, req.Private)
			resp.Diagnostics.Append(d...)
			ifMatch = tag
		}
	}
	body, _, d := r.body(ctx, req.Plan, req.Config, prior, opUpdate)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	confirm, known, d := r.configuredConfirm(ctx, req.Config)
	resp.Diagnostics.Append(d...)
	settings := r.settingsKnown(ctx, req.Config)
	if settings && len(body) == 0 {
		held := types.StringNull()
		if !creating {
			resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root(fingerprintAttr), &held)...)
		}
		setFingerprint(held)
		return
	}
	if !known || !settings {
		if r.client != nil {
			resp.Diagnostics.AddWarning("This change is not previewed",
				"A value in it is known only at apply, so the plan cannot ask the platform what it moves; the apply previews it and applies what it finds.")
		}
		setFingerprint(types.StringUnknown())
		return
	}
	if r.client == nil {
		setFingerprint(types.StringUnknown())
		return
	}
	p, written, err := r.probe(ctx, id, body, ifMatch, confirm)
	switch {
	case err == nil && written != nil:
		setFingerprint(types.StringNull())
		return
	case client.IsReadOnlyKey(err):
		resp.Diagnostics.AddWarning("This change is not previewed",
			"The preview of what this change moves needs an api:admin key; the apply previews it and applies what it finds.")
		setFingerprint(types.StringUnknown())
		return
	case client.IsPreconditionFailed(err):
		summary, detail := r.precondition("plan", err)
		resp.Diagnostics.AddError(summary, detail)
		return
	case client.IsNotFound(err):
		resp.Diagnostics.AddError(r.title("plan"), r.adoptMissing(id, err))
		return
	case err != nil:
		resp.Diagnostics.AddError(r.title("plan"), err.Error())
		return
	}
	if probs := r.problems(p, confirm); len(probs) > 0 {
		r.addProblems(&resp.Diagnostics, probs)
		return
	}
	resp.Diagnostics.AddWarning("What this change moves", p.warning())
	setFingerprint(types.StringValue(p.Fingerprint))
}

// leftInPlace is what a destroy of a LeaveOnDestroy row says.
func (r *facadeResource) leftInPlace() string {
	return fmt.Sprintf("sreagent_%s cannot be deleted: the platform has no door that removes it. Terraform stopped managing it and the platform keeps what it holds. %s", r.spec.TypeName, r.spec.LeaveOnDestroy)
}
